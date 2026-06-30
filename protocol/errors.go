package protocol

import "encoding/json"

// Standard JSON-RPC 2.0 error codes (§5.1 of the spec).
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Application-level error codes (server-defined range: -32000 to -32099).
const (
	// CodeNotConnected is returned when a Telegram operation is requested
	// before mtgo.connect has succeeded.
	CodeNotConnected = -32001
	// CodeNotImplemented is returned when a handler exists but the underlying
	// mtgo call has not been wired yet (foundation phase).
	CodeNotImplemented = -32002
)

// Pre-built standard error values.
var (
	ErrParseError     = &ErrorValue{Code: CodeParseError, Message: "Parse error"}
	ErrInvalidRequest = &ErrorValue{Code: CodeInvalidRequest, Message: "Invalid Request"}
	ErrMethodNotFound = &ErrorValue{Code: CodeMethodNotFound, Message: "Method not found"}
	ErrInvalidParams  = &ErrorValue{Code: CodeInvalidParams, Message: "Invalid params"}
	ErrInternal       = &ErrorValue{Code: CodeInternalError, Message: "Internal error"}
)

// Application-level error constructors.

// NotConnected returns a -32001 error with an optional detail message.
func NotConnected(detail string) *ErrorValue {
	return appError(CodeNotConnected, "not connected — call mtgo.connect first", detail)
}

// NotImplemented returns a -32002 error.
func NotImplemented(method string) *ErrorValue {
	return appError(CodeNotImplemented, "method not implemented in this build", method)
}

// InternalError wraps a server-side error into a JSON-RPC error value.
func InternalError(detail string) *ErrorValue {
	return appError(CodeInternalError, "internal error", detail)
}

func appError(code int, message, detail string) *ErrorValue {
	ev := &ErrorValue{Code: code, Message: message}
	if detail != "" {
		ev.Data, _ = json.Marshal(detail)
	}
	return ev
}
