# mtgo-ipc Documentation

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                  External Client                      │
│            (Go, Node.js, Python, …)                   │
└──────────────────┬───────────────────────────────────┘
                   │  JSON-RPC 2.0 over Unix socket
                   │  (newline-delimited JSON)
                   ▼
┌──────────────────────────────────────────────────────┐
│  transport/    Unix socket listener + framing         │
└──────────────────┬───────────────────────────────────┘
                   │  decoded JSON-RPC messages
                   ▼
┌──────────────────────────────────────────────────────┐
│  protocol/     Request / Response / ErrorValue        │
└──────────────────┬───────────────────────────────────┘
                   │  dispatch by method name
                   ▼
┌──────────────────────────────────────────────────────┐
│  server/       6-method registry + accept loop        │
└──────────────────┬───────────────────────────────────┘
                   │  Telegram calls
                   ▼
┌──────────────────────────────────────────────────────┐
│  client/       mtgo integration (telegram.Client)     │
└──────────────────┬───────────────────────────────────┘
                   │  MTProto
                   ▼
                  Telegram
```

## Wire protocol

JSON-RPC 2.0, one message per line (newline-delimited JSON).

### Methods

| Method | Params | Description |
|--------|--------|-------------|
| `health` | — | Liveness probe |
| `mtgo.info` | — | Build + connection metadata |
| `mtgo.connect` | — | Establish Telegram connection |
| `mtgo.close` | — | Tear down Telegram connection |
| `auth.status` | — | Current authentication state |
| `telegram.invoke` | `{method, args}` | Generic raw MTProto call |

### Example

Request:
```json
{"jsonrpc": "2.0", "id": 1, "method": "telegram.invoke", "params": {"method": "help.getConfig", "args": {}}}
```

Response:
```json
{"jsonrpc": "2.0", "id": 1, "result": {"_": "config", ...}}
```

### Error codes

| Code | Meaning |
|------|---------|
| -32700 | Parse error |
| -32600 | Invalid Request |
| -32601 | Method not found |
| -32602 | Invalid params |
| -32603 | Internal error |
| -32001 | Not connected |
| -32002 | Not implemented in this build |

## Roadmap

The foundation establishes the protocol and server skeleton. Planned (not yet
implemented):

- mtgo client wiring (`client/` → real `telegram.Client`)
- Update streaming over the socket
- Session persistence
