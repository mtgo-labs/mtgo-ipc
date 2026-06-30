// Package server orchestrates the IPC server lifecycle.
//
// The Server ties four layers together:
//
//	transport     → how bytes arrive  (Unix socket)
//	protocol      → what messages mean (JSON-RPC 2.0 + events)
//	client        → what work gets done (mtgo Telegram calls)
//	subscription  → who gets pushed updates
//
// Each connection runs two goroutines: a reader that decodes JSON-RPC requests
// and dispatches them, and a writer that drains a bounded outbound queue of
// pre-marshaled messages (responses + push events). The subscription manager
// broadcasts events into subscribed clients' outbound queues without blocking.
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"

	"github.com/mtgo-labs/mtgo-ipc/client"
	"github.com/mtgo-labs/mtgo-ipc/protocol"
	"github.com/mtgo-labs/mtgo-ipc/subscription"
	"github.com/mtgo-labs/mtgo-ipc/transport"
)

// sendBufferSize is the per-client outbound queue depth. When full, push events
// are dropped (slow-reader policy). Responses are never dropped — if the buffer
// is full the client is effectively dead.
const sendBufferSize = 256

// Config holds the parameters needed to start the IPC server.
type Config struct {
	SocketPath string
}

// session holds per-connection state.
type session struct {
	conn net.Conn
	send chan json.RawMessage // bounded outbound queue (responses + events)
	sub  *subscription.Client  // nil until updates.subscribe
}

// Server is the IPC bridge server.
type Server struct {
	tr   transport.Transport
	cli  *client.Client
	disp *dispatcher
	subs *subscription.Manager
}

// New creates a Server. The server starts with no Telegram credentials —
// the first "mtgo.connect" IPC call provides them.
func New(cfg Config) (*Server, error) {
	if cfg.SocketPath == "" {
		return nil, fmt.Errorf("server: socket path is required")
	}
	tr, err := transport.NewUnixSocket(cfg.SocketPath)
	if err != nil {
		return nil, err
	}

	cli := client.New()
	subs := subscription.NewManager()

	// Wire raw Telegram updates → subscription manager.
	cli.OnRawUpdate(func(updateJSON json.RawMessage) {
		event := protocol.NewEvent("updates.raw", updateJSON)
		eventBytes, err := json.Marshal(event)
		if err != nil {
			return
		}
		subs.Broadcast("raw", eventBytes)
	})

	return &Server{
		tr:   tr,
		cli:  cli,
		subs: subs,
		disp: newDispatcher(cli, subs),
	}, nil
}

// Addr returns the address the server is listening on.
func (s *Server) Addr() string {
	return s.tr.Addr().String()
}

// Serve runs the accept loop until ctx is cancelled or the listener errors.
func (s *Server) Serve(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.acceptLoop(ctx) }()

	select {
	case <-ctx.Done():
		return s.Close()
	case err := <-errCh:
		return err
	}
}

func (s *Server) acceptLoop(ctx context.Context) error {
	for {
		conn, err := s.tr.Accept()
		if err != nil {
			return fmt.Errorf("accept: %w", err)
		}
		go s.handleConn(ctx, conn)
	}
}

// handleConn manages one client connection: a reader goroutine (this function)
// and a writer goroutine that drains the outbound queue.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	sess := &session{
		conn: conn,
		send: make(chan json.RawMessage, sendBufferSize),
	}

	// Writer goroutine: drains outbound queue, writes to the socket.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for msg := range sess.send {
			if _, err := conn.Write(append(msg, '\n')); err != nil {
				// Write failed — force-close to unblock the reader.
				_ = conn.Close()
				// Drain remaining to prevent senders from blocking.
				for range sess.send {
				}
				return
			}
		}
	}()

	// Cleanup: unsubscribe first (prevents Broadcast to closed channel),
	// then close the queue (stops the writer), then wait for writer exit.
	defer func() {
		if sess.sub != nil {
			s.subs.Unsubscribe(sess.sub)
		}
		close(sess.send)
		<-writerDone
		_ = conn.Close()
	}()

	reader := bufio.NewReader(conn)
	for {
		line, err := transport.ReadMessage(reader)
		if err != nil {
			return // EOF or read error
		}
		s.processMessage(ctx, line, sess)
	}
}

// processMessage decodes one JSON-RPC message and queues the response.
func (s *Server) processMessage(ctx context.Context, raw []byte, sess *session) {
	var req protocol.Request
	if err := json.Unmarshal(raw, &req); err != nil {
		s.queue(sess, protocol.NewError(nil, protocol.ErrParseError))
		return
	}

	// Notifications carry no id — no response is sent.
	if req.IsNotification() {
		s.disp.dispatchAndHandle(ctx, req, sess)
		return
	}

	resp := s.disp.dispatchAndHandle(ctx, req, sess)
	s.queue(sess, resp)
}

// queue marshals a response and non-blocking-sends it to the session's outbound
// queue. If the queue is full (client not reading), the response is logged and
// dropped — the client is effectively dead.
func (s *Server) queue(sess *session, resp protocol.Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("server: marshal response: %v", err)
		return
	}
	select {
	case sess.send <- data:
	default:
		log.Printf("server: outbound queue full, dropping response")
	}
}

// Close stops the server and releases the transport.
func (s *Server) Close() error {
	return s.tr.Close()
}
