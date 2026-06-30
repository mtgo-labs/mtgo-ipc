// Package transport abstracts the network layer for the IPC server.
//
// The initial and primary implementation is a Unix domain socket, which gives
// fast local IPC without TCP overhead or network exposure. Framing is
// newline-delimited JSON: each JSON-RPC message is exactly one line terminated
// by '\n'.
//
// The Transport interface is minimal so additional transports (TCP, WebSocket)
// can be added later without changes to the server or protocol packages.
package transport

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
)

// Transport is the wire abstraction consumed by the server.
type Transport interface {
	// Accept blocks until a client connects, returning the connection.
	Accept() (net.Conn, error)

	// Close stops accepting new connections and releases the listener.
	Close() error

	// Addr returns the address the transport is listening on.
	Addr() net.Addr
}

// Ensure UnixSocket satisfies Transport at compile time.
var _ Transport = (*UnixSocket)(nil)

// UnixSocket is a Unix-domain-socket transport.
type UnixSocket struct {
	listener net.Listener
	path     string
}

// NewUnixSocket creates a Unix socket transport bound to path.
//
// If a stale socket file from a previous run exists at path it is removed so
// the new listener can bind.
func NewUnixSocket(path string) (*UnixSocket, error) {
	// Remove a stale socket file left over from a crashed process.
	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(path)
	}

	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", path, err)
	}
	return &UnixSocket{listener: l, path: path}, nil
}

// Accept blocks until a client connects.
func (u *UnixSocket) Accept() (net.Conn, error) {
	return u.listener.Accept()
}

// Close stops the listener and removes the socket file.
func (u *UnixSocket) Close() error {
	err := u.listener.Close()
	_ = os.Remove(u.path)
	return err
}

// Addr returns the listener address.
func (u *UnixSocket) Addr() net.Addr {
	return u.listener.Addr()
}

// --- framing helpers (newline-delimited JSON) ---

// ErrEmptyLine is returned by ReadMessage when a line contains no data.
var ErrEmptyLine = errors.New("transport: empty line")

// ReadMessage reads one newline-delimited JSON value from r.
//
// Blank lines are skipped. Returns io.EOF when the reader is exhausted.
func ReadMessage(r *bufio.Reader) ([]byte, error) {
	for {
		line, err := r.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			return nil, err
		}
		trimmed := trimSpaceBytes(line)
		if len(trimmed) == 0 {
			if err != nil {
				return nil, err
			}
			continue // skip blank lines
		}
		return trimmed, nil
	}
}

// WriteMessage marshals v as JSON and writes it as a single line to w.
func WriteMessage(w io.Writer, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if _, err := w.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func trimSpaceBytes(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r' || b[start] == '\n') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r' || b[end-1] == '\n') {
		end--
	}
	return b[start:end]
}
