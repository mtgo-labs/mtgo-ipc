package server

import (
	"context"
	"encoding/json"

	"github.com/mtgo-labs/mtgo-ipc/client"
	"github.com/mtgo-labs/mtgo-ipc/protocol"
	"github.com/mtgo-labs/mtgo-ipc/subscription"
)

// handler processes one request in the context of a session. It returns a
// JSON-serialisable result or a JSON-RPC error.
type handler func(ctx context.Context, sess *session, params json.RawMessage) (any, *protocol.ErrorValue)

// dispatcher maps JSON-RPC method names to handlers.
//
// Eight methods in the protocol:
//
//	health             — liveness probe
//	mtgo.info          — build + connection metadata
//	mtgo.connect       — establish the Telegram connection
//	mtgo.close         — tear down the Telegram connection
//	auth.status        — current authentication state
//	telegram.invoke    — generic raw MTProto method call
//	updates.subscribe  — subscribe to push events
//	updates.unsubscribe — stop receiving push events
type dispatcher struct {
	cli      *client.Client
	subs     *subscription.Manager
	handlers map[string]handler
}

func newDispatcher(cli *client.Client, subs *subscription.Manager) *dispatcher {
	d := &dispatcher{cli: cli, subs: subs}
	d.handlers = map[string]handler{
		"health":              d.health,
		"mtgo.info":           d.mtgoInfo,
		"mtgo.connect":        d.mtgoConnect,
		"mtgo.close":          d.mtgoClose,
		"auth.status":         d.authStatus,
		"telegram.invoke":     d.telegramInvoke,
		"updates.subscribe":   d.updatesSubscribe,
		"updates.unsubscribe": d.updatesUnsubscribe,
	}
	return d
}

// dispatchAndHandle looks up the handler, executes it, and builds the response.
func (d *dispatcher) dispatchAndHandle(ctx context.Context, req protocol.Request, sess *session) protocol.Response {
	h, ok := d.handlers[req.Method]
	if !ok {
		return protocol.NewError(req.ID, protocol.ErrMethodNotFound)
	}
	result, rpcErr := h(ctx, sess, req.Params)
	if rpcErr != nil {
		return protocol.NewError(req.ID, rpcErr)
	}
	return protocol.NewResult(req.ID, result)
}

// --- handlers ---

func (d *dispatcher) health(_ context.Context, _ *session, _ json.RawMessage) (any, *protocol.ErrorValue) {
	return map[string]any{"status": "ok"}, nil
}

func (d *dispatcher) mtgoInfo(_ context.Context, _ *session, _ json.RawMessage) (any, *protocol.ErrorValue) {
	info, err := d.cli.Info()
	if err != nil {
		return nil, protocol.InternalError(err.Error())
	}
	return info, nil
}

func (d *dispatcher) mtgoConnect(ctx context.Context, _ *session, params json.RawMessage) (any, *protocol.ErrorValue) {
	var p protocol.ConnectParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, protocol.ErrInvalidParams
		}
	}
	cfg := client.Config{
		APIID:    p.APIID,
		APIHash:  p.APIHash,
		BotToken: p.BotToken,
		Session:  p.Session,
	}
	if err := d.cli.Connect(ctx, cfg); err != nil {
		return nil, protocol.InternalError(err.Error())
	}
	return map[string]any{"connected": true}, nil
}

func (d *dispatcher) mtgoClose(_ context.Context, _ *session, _ json.RawMessage) (any, *protocol.ErrorValue) {
	if err := d.cli.Close(); err != nil {
		return nil, protocol.InternalError(err.Error())
	}
	return map[string]any{"connected": false}, nil
}

func (d *dispatcher) authStatus(_ context.Context, _ *session, _ json.RawMessage) (any, *protocol.ErrorValue) {
	status, err := d.cli.AuthStatus()
	if err != nil {
		return nil, protocol.InternalError(err.Error())
	}
	return status, nil
}

func (d *dispatcher) telegramInvoke(ctx context.Context, _ *session, params json.RawMessage) (any, *protocol.ErrorValue) {
	if !d.cli.Connected() {
		return nil, protocol.NotConnected("")
	}
	var p protocol.InvokeParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, protocol.ErrInvalidParams
		}
	}
	if p.Method == "" {
		return nil, protocol.ErrInvalidParams
	}
	result, err := d.cli.Invoke(ctx, p.Method, p.Args)
	if err != nil {
		return nil, protocol.InternalError(err.Error())
	}
	return result, nil
}

func (d *dispatcher) updatesSubscribe(_ context.Context, sess *session, params json.RawMessage) (any, *protocol.ErrorValue) {
	var p protocol.SubscribeParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, protocol.ErrInvalidParams
		}
	}
	// Already subscribed — update types by unsubscribing first.
	if sess.sub != nil {
		d.subs.Unsubscribe(sess.sub)
	}
	types := p.Types
	if len(types) == 0 {
		types = []string{"raw"}
	}
	sess.sub = d.subs.Subscribe(sess.send, types)
	return map[string]any{"subscribed": true, "types": types}, nil
}

func (d *dispatcher) updatesUnsubscribe(_ context.Context, sess *session, _ json.RawMessage) (any, *protocol.ErrorValue) {
	if sess.sub == nil {
		return map[string]any{"subscribed": false}, nil
	}
	d.subs.Unsubscribe(sess.sub)
	sess.sub = nil
	return map[string]any{"subscribed": false}, nil
}
