// Package client wraps a single mtgo Telegram client.
//
// The mtgo client is created lazily: the server starts with no credentials,
// and the first "mtgo.connect" IPC call provides them. This package is the
// only place in mtgo-ipc that imports github.com/mtgo-labs/mtgo.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mtgo-labs/mtgo/telegram"
	"github.com/mtgo-labs/mtgo/tg"
)

// Config holds the credentials needed to connect to Telegram.
type Config struct {
	APIID    int
	APIHash  string
	Session  string
	BotToken string
}

// Client wraps a single mtgo telegram.Client. The mtgo client is nil until
// Connect is called with credentials.
type Client struct {
	mu            sync.Mutex
	tg            *telegram.Client
	cfg           Config
	updateHandler func(json.RawMessage)
}

// New creates an empty client with no credentials. Call Connect to create
// the mtgo client and establish the Telegram connection.
func New() *Client {
	return &Client{}
}

// OnRawUpdate stores a handler called when raw Telegram updates arrive.
// The handler is wired to the mtgo client on the next successful Connect.
func (c *Client) OnRawUpdate(handler func(json.RawMessage)) {
	c.updateHandler = handler
}

// Connect creates the mtgo client with cfg and connects to Telegram.
// Idempotent: if already connected, returns nil. The first caller's
// credentials establish the session; subsequent calls with different
// credentials are ignored (the session persists).
func (c *Client) Connect(ctx context.Context, cfg Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.tg != nil {
		if c.tg.IsConnected() {
			return nil
		}
		return c.tg.Connect(timeoutFromCtx(ctx))
	}

	if cfg.APIID == 0 || cfg.APIHash == "" {
		return fmt.Errorf("client: api_id and api_hash are required")
	}

	tg, err := telegram.NewClient(int32(cfg.APIID), cfg.APIHash, &telegram.Config{
		SessionString: cfg.Session,
		BotToken:      cfg.BotToken,
		InMemory:      true,
	})
	if err != nil {
		return fmt.Errorf("create mtgo client: %w", err)
	}

	if c.updateHandler != nil {
		tg.OnRawUpdate(func(ctx *telegram.Context) {
			if ctx.Update == nil || ctx.Update.Raw == nil {
				return
			}
			data, err := encodeTLToJSON(ctx.Update.Raw)
			if err != nil {
				return
			}
			c.updateHandler(data)
		})
	}

	c.tg = tg
	c.cfg = cfg
	return tg.Connect(timeoutFromCtx(ctx))
}

// Close disconnects the Telegram client.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tg == nil {
		return nil
	}
	return c.tg.Disconnect()
}

// Info returns metadata about the build and connection state.
func (c *Client) Info() (any, error) {
	c.mu.Lock()
	tg := c.tg
	c.mu.Unlock()

	if tg == nil {
		return map[string]any{"name": "mtgo-ipc", "version": "0.1.0-dev", "connected": false}, nil
	}
	cfg := tg.Config()
	return map[string]any{
		"name":      "mtgo-ipc",
		"version":   "0.1.0-dev",
		"connected": tg.IsConnected(),
		"api_id":    cfg.APIID,
		"dc":        cfg.DC,
	}, nil
}

// AuthStatus reports the current authentication state.
func (c *Client) AuthStatus() (any, error) {
	c.mu.Lock()
	tg := c.tg
	c.mu.Unlock()

	if tg == nil {
		return map[string]any{"authorized": false}, nil
	}
	me := tg.Me()
	if me == nil {
		return map[string]any{"authorized": false}, nil
	}
	return map[string]any{
		"authorized": true,
		"user_id":    me.ID,
		"is_bot":     me.IsBot,
		"username":   me.Username,
		"first_name": me.FirstName,
	}, nil
}

// Invoke executes a raw Telegram MTProto method call via JSON.
func (c *Client) Invoke(ctx context.Context, method string, args json.RawMessage) (json.RawMessage, error) {
	c.mu.Lock()
	tg := c.tg
	c.mu.Unlock()

	if tg == nil {
		return nil, fmt.Errorf("not connected")
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	return tg.InvokeJSON(ctx, method, args, false)
}

// Connected reports whether the client has an active Telegram connection.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tg != nil && c.tg.IsConnected()
}

func timeoutFromCtx(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		return time.Until(deadline)
	}
	return 0
}

// --- TL → JSON encoding (with type discriminator) ---

var (
	idToName     map[uint32]string
	idToNameOnce sync.Once
)

func getIDToName() map[uint32]string {
	idToNameOnce.Do(func() {
		m := make(map[uint32]string, len(tg.NamesMap))
		for name, id := range tg.NamesMap {
			m[id] = name
		}
		idToName = m
	})
	return idToName
}

func encodeTLToJSON(obj tg.TLObject) ([]byte, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if name, ok := getIDToName()[obj.ConstructorID()]; ok {
		raw["_"] = name
	}
	return json.Marshal(raw)
}
