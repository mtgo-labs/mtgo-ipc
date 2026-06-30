// Package protocol defines the JSON-RPC 2.0 message contract for mtgo-ipc.
//
// Three message types share the wire (newline-delimited JSON):
//
//   - Request  — client → server (has method + id)
//   - Response — server → client (has result/error + id)
//   - Event    — server → client push (has event + data, no id)
package protocol

import "encoding/json"

// Version is the JSON-RPC protocol version implemented by this package.
const Version = "2.0"

// Request is a JSON-RPC 2.0 request object.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

// IsNotification reports whether the request expects no response.
func (r Request) IsNotification() bool {
	return len(r.ID) == 0
}

// Response is a JSON-RPC 2.0 response object.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorValue     `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}

// Event is a server-pushed message with no request/response pairing.
type Event struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// ErrorValue is the structured error payload inside a JSON-RPC response.
type ErrorValue struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *ErrorValue) Error() string {
	return e.Message
}

// InvokeParams is the params object for "telegram.invoke".
type InvokeParams struct {
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args,omitempty"`
}

// ConnectParams is the params object for "mtgo.connect". The client passes
// Telegram credentials here — the server itself starts with no credentials.
type ConnectParams struct {
	APIID    int    `json:"api_id"`
	APIHash  string `json:"api_hash"`
	BotToken string `json:"bot_token,omitempty"`
	Session  string `json:"session,omitempty"`
}

// SubscribeParams is the params object for "updates.subscribe".
type SubscribeParams struct {
	Types []string `json:"types,omitempty"`
}

// --- constructors ---

func NewResult(id json.RawMessage, result any) Response {
	raw, _ := json.Marshal(result)
	return Response{JSONRPC: Version, Result: raw, ID: id}
}

func NewError(id json.RawMessage, err *ErrorValue) Response {
	return Response{JSONRPC: Version, Error: err, ID: id}
}

func NewEvent(name string, data json.RawMessage) Event {
	return Event{Event: name, Data: data}
}
