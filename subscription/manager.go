// Package subscription manages IPC client subscriptions to Telegram updates.
//
// The Manager tracks which connections are subscribed and to which event types.
// When a raw Telegram update arrives, the server calls Broadcast with the
// pre-marshaled event JSON. The Manager non-blocking-sends to each subscribed
// client's outbound channel — if a client's buffer is full (slow reader), the
// event is dropped for that client only, never blocking the update loop.
package subscription

import (
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
)

// Client is a subscribed IPC connection.
type Client struct {
	id      int64
	send    chan json.RawMessage // outbound queue (shared with the server's writer goroutine)
	types   map[string]bool      // subscribed event types, e.g. {"raw": true}
	dropped atomic.Int64         // total events dropped for this client
}

// ID returns the client's unique subscription ID.
func (c *Client) ID() int64 { return c.id }

// Dropped returns the total number of events dropped for this client.
func (c *Client) Dropped() int64 { return c.dropped.Load() }

// Manager tracks subscribed clients and broadcasts events to them.
type Manager struct {
	mu      sync.RWMutex
	clients map[int64]*Client
	nextID  atomic.Int64
}

// NewManager creates an empty subscription manager.
func NewManager() *Manager {
	return &Manager{clients: make(map[int64]*Client)}
}

// Subscribe registers a client. The send channel must be bounded (buffered).
// If types is empty, defaults to ["raw"].
// Returns the Client for later Unsubscribe.
func (m *Manager) Subscribe(send chan json.RawMessage, types []string) *Client {
	if len(types) == 0 {
		types = []string{"raw"}
	}
	typeSet := make(map[string]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}
	c := &Client{
		id:    m.nextID.Add(1),
		send:  send,
		types: typeSet,
	}
	m.mu.Lock()
	m.clients[c.id] = c
	m.mu.Unlock()
	return c
}

// Unsubscribe removes a client. Safe to call with nil or already-removed client.
// Must be called BEFORE closing the send channel to prevent Broadcast from
// sending to a closed channel.
func (m *Manager) Unsubscribe(c *Client) {
	if c == nil {
		return
	}
	m.mu.Lock()
	delete(m.clients, c.id)
	m.mu.Unlock()
}

// Broadcast sends a pre-marshaled event to all clients subscribed to eventType.
// Non-blocking: if a client's send buffer is full, the event is dropped for
// that client (slow-reader policy). This never blocks the caller.
func (m *Manager) Broadcast(eventType string, msg json.RawMessage) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, c := range m.clients {
		if !c.types[eventType] {
			continue
		}
		select {
		case c.send <- msg:
		default:
			c.dropped.Add(1)
			log.Printf("subscription: dropping event for client %d (buffer full)", c.id)
		}
	}
}

// Count returns the number of subscribed clients.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}
