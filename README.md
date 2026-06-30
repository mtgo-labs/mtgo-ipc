<div align="center">

# mtgo-ipc

A standalone IPC bridge for [mtgo](https://github.com/mtgo-labs/mtgo) — exposes
a single MTProto client over **JSON-RPC 2.0 on a Unix socket** so any language
or tool can drive Telegram without importing Go.

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/mtgo-labs/mtgo-ipc.svg)](https://pkg.go.dev/github.com/mtgo-labs/mtgo-ipc)
[![mtgo](https://img.shields.io/badge/mtgo-v0.11.0-00ADD8)](https://github.com/mtgo-labs/mtgo)

</div>

---

## Table of Contents

- [What is mtgo-ipc?](#what-is-mtgo-ipc)
- [Why?](#why)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Protocol Reference](#protocol-reference)
- [Wire Format](#wire-format)
- [Error Handling](#error-handling)
- [Client Examples](#client-examples)
- [Project Layout](#project-layout)
- [Design Principles](#design-principles)
- [Status & Roadmap](#status--roadmap)
- [License](#license)

---

## What is mtgo-ipc?

**mtgo-ipc** is a long-lived server process that maintains a single Telegram
MTProto connection via [mtgo](https://github.com/mtgo-labs/mtgo) and exposes it
through a local JSON-RPC 2.0 API over a Unix domain socket.

Instead of importing `github.com/mtgo-labs/mtgo` into every tool that needs
Telegram access, you run **one** mtgo-ipc server and connect to it from
**any** language: Python, Node.js, Rust, Ruby, shell scripts, even `nc`.

The design is intentionally minimal — six methods, one transport, zero
high-level wrappers. All Telegram operations pass through a single generic
`telegram.invoke` call that accepts raw TL method names and JSON arguments.

## Why?

**The problem:** mtgo is a pure-Go MTProto client. Only Go programs can use it
directly. Every other language needs either its own MTProto implementation or a
bridge.

**The solution:** mtgo-ipc runs mtgo as a daemon and exposes a trivially simple
protocol that anything can speak — one line of JSON per request, one line per
response, over a Unix socket.

| Approach | Pros | Cons |
|----------|------|------|
| Import mtgo in Go | Native, zero overhead | Go only |
| HTTP Bot API gateway | Language-agnostic | HTTP overhead, limited to Bot API |
| **mtgo-ipc** | Language-agnostic, full MTProto, zero-copy local IPC | Needs a running server process |

### What makes it different

- **Full MTProto, not Bot API.** `telegram.invoke` calls any TL method —
  `help.getConfig`, `messages.sendMessage`, `channels.createChannel`,
  `upload.getFile`, anything in the schema. No artificial API surface
  limitation.
- **One connection, many consumers.** The server holds a single authenticated
  Telegram session. Any number of local clients can connect to the socket and
  share it — no re-auth, no connection pooling on the client side.
- **No HTTP, no framework, no CGO.** Just a Unix socket and newline-delimited
  JSON. Zero network exposure by default.
- **Boring wire protocol.** JSON-RPC 2.0 is a standard. Existing JSON-RPC
  clients in every language work out of the box.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    External Clients                        │
│              Go · Node.js · Python · Rust · nc             │
└──────────────────────┬───────────────────────────────────┘
                       │
                       │  JSON-RPC 2.0 (newline-delimited JSON)
                       │  over Unix domain socket
                       │
                       ▼
┌──────────────────────────────────────────────────────────┐
│  transport/    Unix socket listener + message framing      │
│                (one goroutine per accepted connection)     │
└──────────────────────┬───────────────────────────────────┘
                       │  decoded JSON-RPC messages
                       ▼
┌──────────────────────────────────────────────────────────┐
│  protocol/     Request · Response · Event · ErrorValue       │
└──────────────────────┬───────────────────────────────────┘
                       │  dispatch by method name
                       ▼
┌──────────────────────────────────────────────────────────┐
│  server/       8-method dispatch + accept loop + sessions    │
│                (reader + writer goroutine per connection)    │
└──────────────────────┬───────────────────────────────────┘
                       │  raw updates → subscription manager
                       ▼
┌──────────────────────────────────────────────────────────┐
│  subscription/ Per-client bounded queues + slow-reader    │
│                drop policy · non-blocking broadcast       │
└──────────────────────┬───────────────────────────────────┘
                       │  MTProto over TCP
                       ▼
                    Telegram
```

### Layer responsibilities

| Layer | Package | Role |
|-------|---------|------|
| **Transport** | `transport/` | Accepts Unix socket connections. Frames messages as newline-delimited JSON. One goroutine per connection (multi-client by design). |
| **Protocol** | `protocol/` | Three message types: `Request`, `Response`, `Event`, plus `ErrorValue` and error codes. No business logic. |
| **Server** | `server/` | Accept loop. Per-connection reader + writer goroutines. Dispatches 8 methods. Owns graceful shutdown and session lifecycle. |
| **Client** | `client/` | The only package that imports mtgo. Wraps `telegram.Client` — creates the connection, authenticates, exposes `InvokeJSON` for raw TL calls and `OnRawUpdate` for push events. |
| **Subscription** | `subscription/` | Per-client bounded queues. Non-blocking broadcast with slow-reader drop policy. Never blocks the update loop. |

## Quick Start

### Prerequisites

- Go 1.26+
- Telegram API credentials from [my.telegram.org](https://my.telegram.org)
- A bot token (from [@BotFather](https://t.me/BotFather)) or a session string

### Build & run

```bash
git clone https://github.com/mtgo-labs/mtgo-ipc.git
cd mtgo-ipc

# Build
go build -o mtgo-ipc ./cmd/mtgo-ipc

# Start with a bot token
./mtgo-ipc \
  --api-id 123 \
  --api-hash abc \
  --bot-token "123456789:AAE..." \
  --socket /tmp/mtgo-ipc.sock

# → 2026/06/30 20:00:51 mtgo-ipc listening on /tmp/mtgo-ipc.sock
```

### Connect and call Telegram

From another terminal, send JSON-RPC requests to the socket:

```bash
# 1. Connect to Telegram (authenticates with bot token / session string)
echo '{"jsonrpc":"2.0","id":1,"method":"mtgo.connect"}' | socat - UNIX-CONNECT:/tmp/mtgo-ipc.sock
# → {"jsonrpc":"2.0","result":{"connected":true},"id":1}

# 2. Invoke a raw TL method
echo '{"jsonrpc":"2.0","id":2,"method":"telegram.invoke","params":{"method":"help.getConfig","args":{}}}' | socat - UNIX-CONNECT:/tmp/mtgo-ipc.sock
# → {"jsonrpc":"2.0","result":{"_":"config","dc_options":[...],"date":1782838901,...},"id":2}

# 3. Check auth status
echo '{"jsonrpc":"2.0","id":3,"method":"auth.status"}' | socat - UNIX-CONNECT:/tmp/mtgo-ipc.sock
# → {"jsonrpc":"2.0","result":{"authorized":true,"user_id":5998453459,"is_bot":true,"username":"DavidSiteBot"},"id":3}
```

### Using the bundled examples

```bash
# Go client — sends a health ping
go run ./examples/go-client

# Node.js client — sends a health ping
node examples/node-client/client.js
```

## Configuration

All flags can also be set via environment variables.

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--socket` | `MTGO_IPC_SOCKET` | `/tmp/mtgo-ipc.sock` | Unix socket path |
| `--api-id` | `API_ID` | — | Telegram API ID (**required**) |
| `--api-hash` | `API_HASH` | — | Telegram API hash (**required**) |
| `--bot-token` | `BOT_TOKEN` | — | Bot token (from @BotFather) |
| `--session` | `SESSION` | — | Session string (Telethon/Pyrogram/GramJS/mtcute — auto-detected) |

> Either `--bot-token` or `--session` must be provided for authentication.

### Authentication modes

**Bot token** — simplest, authenticates as a bot:
```bash
./mtgo-ipc --api-id ID --api-hash HASH --bot-token "123:ABC..."
```

**Session string** — authenticates as a user account (any auto-detected format):
```bash
./mtgo-ipc --api-id ID --api-hash HASH --session "1BQF..."
```

## Protocol Reference

JSON-RPC 2.0 — [specification](https://www.jsonrpc.org/specification).

### Methods

#### `health`

Liveness probe. Does not touch the Telegram connection.

```json
→ {"jsonrpc":"2.0","id":1,"method":"health"}
← {"jsonrpc":"2.0","id":1,"result":{"status":"ok"}}
```

#### `mtgo.info`

Returns build metadata and connection state.

```json
→ {"jsonrpc":"2.0","id":2,"method":"mtgo.info"}
← {"jsonrpc":"2.0","id":2,"result":{"name":"mtgo-ipc","version":"0.1.0-dev","connected":true,"api_id":22333936,"dc":4}}
```

#### `mtgo.connect`

Establishes the Telegram connection. Authenticates using the configured bot
token or session string. Idempotent — calling when already connected is safe.

```json
→ {"jsonrpc":"2.0","id":3,"method":"mtgo.connect"}
← {"jsonrpc":"2.0","id":3,"result":{"connected":true}}
```

#### `mtgo.close`

Disconnects from Telegram. The server process stays alive and accepts new
connections; call `mtgo.connect` to reconnect.

```json
→ {"jsonrpc":"2.0","id":4,"method":"mtgo.close"}
← {"jsonrpc":"2.0","id":4,"result":{"connected":false}}
```

#### `auth.status`

Reports the current authentication state from the active Telegram session.

```json
→ {"jsonrpc":"2.0","id":5,"method":"auth.status"}
← {"jsonrpc":"2.0","id":5,"result":{"authorized":true,"user_id":5998453459,"is_bot":true,"username":"DavidSiteBot","first_name":"..."}}
```

When not yet authenticated:
```json
← {"jsonrpc":"2.0","id":5,"result":{"authorized":false}}
```

#### `telegram.invoke`

The core method. Executes any Telegram MTProto TL function by name with JSON
arguments. The response is the raw TL result serialized as JSON.

**Params:**

| Field | Type | Description |
|-------|------|-------------|
| `method` | `string` | TL function name (e.g. `"help.getConfig"`, `"messages.sendMessage"`) |
| `args` | `object` | JSON arguments matching the TL request struct fields |

**Example — get config:**
```json
→ {"jsonrpc":"2.0","id":6,"method":"telegram.invoke","params":{"method":"help.getConfig","args":{}}}
← {"jsonrpc":"2.0","id":6,"result":{"_":"config","dc_options":[...],"date":1782838901,...}}
```

**Example — send a message:**
```json
→ {"jsonrpc":"2.0","id":7,"method":"telegram.invoke","params":{
     "method":"messages.sendMessage",
     "args":{
       "peer":{"_":"inputPeerUser","user_id":12345678,"access_hash":87654321},
       "message":"Hello from mtgo-ipc!",
       "random_id":42
     }
  }}
← {"jsonrpc":"2.0","id":7,"result":{"_":"updates","...":"..."}}
```

**Example — get dialogs:**
```json
→ {"jsonrpc":"2.0","id":8,"method":"telegram.invoke","params":{
     "method":"messages.getDialogs",
     "args":{
       "offset_date":0,
       "offset_id":0,
       "offset_peer":{"_":"inputPeerEmpty"},
       "limit":100,
       "hash":0
     }
  }}
```

> **Error if not connected:** `telegram.invoke` returns error code `-32001`
> (not connected) if called before `mtgo.connect` succeeds.

### Update Events

The server pushes Telegram updates to subscribed clients as event messages.
These are **not** JSON-RPC responses — they have no `id` field. Clients
distinguish them by the presence of the `event` key.

#### `updates.subscribe`

Subscribe to push events. The server begins forwarding Telegram updates to this
connection immediately.

**Params:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `types` | `string[]` | `["raw"]` | Update types to receive |

```json
→ {"jsonrpc":"2.0","id":9,"method":"updates.subscribe","params":{"types":["raw"]}}
← {"jsonrpc":"2.0","id":9,"result":{"subscribed":true,"types":["raw"]}}
```

#### `updates.unsubscribe`

Stop receiving push events.

```json
→ {"jsonrpc":"2.0","id":10,"method":"updates.unsubscribe"}
← {"jsonrpc":"2.0","id":10,"result":{"subscribed":false}}
```

#### Event format

Once subscribed, the server pushes raw Telegram updates as they arrive:

```json
← {"event":"updates.raw","data":{"_":"updateNewMessage","message":{...},"pts":123,"pts_count":1}}
```

- `event` is always `"updates.raw"` for raw updates.
- `data` is the raw TL update object with a `_` type discriminator.
- Events arrive on the **same connection** — no separate socket needed.
- If a client is too slow to read, events are **dropped** (not queued
  indefinitely). The buffer depth is 256 messages per client.

> Multiple clients can subscribe independently. Each receives its own copy of
> every update. No polling — events are pushed the instant mtgo receives them.

## Wire Format

Three message types share the wire, all newline-delimited JSON:

- **Transport:** Unix domain socket (`SOCK_STREAM`)
- **Framing:** One JSON object per line, terminated by `\n`
- **Protocol:** [JSON-RPC 2.0](https://www.jsonrpc.org/specification) for
  requests/responses; custom event envelope for push updates

| Direction | Type | Key field | Example |
|-----------|------|-----------|---------|
| client → server | Request | `method` + `id` | `{"jsonrpc":"2.0","id":1,"method":"health"}` |
| server → client | Response | `result`/`error` + `id` | `{"jsonrpc":"2.0","id":1,"result":{"status":"ok"}}` |
| server → client | Event | `event` (no `id`) | `{"event":"updates.raw","data":{...}}` |

Clients distinguish messages by shape: if it has `event`, it's a push update;
if it has `id` without `method`, it's a response; if it has `method`, it's a
request (only client → server).

Multiple requests can be pipelined on a single connection. Responses and events
are interleaved on the same socket. Blank lines are ignored.

## Error Handling

Errors use the standard JSON-RPC error envelope with structured codes.

```json
{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":5}
```

### Error codes

| Code | Meaning | When |
|------|---------|------|
| `-32700` | Parse error | Malformed JSON |
| `-32600` | Invalid Request | Missing required fields |
| `-32601` | Method not found | Unknown method name |
| `-32602` | Invalid params | Missing/invalid `telegram.invoke` params |
| `-32603` | Internal error | Unexpected server-side failure |
| `-32001` | Not connected | `telegram.invoke` called before `mtgo.connect` |

Application errors (code range `-32000` to `-32099`) may include a `data` field
with additional detail:

```json
{"jsonrpc":"2.0","error":{"code":-32002,"message":"not implemented","data":"method name"},"id":9}
```

## Client Examples

### Go

```go
conn, _ := net.Dial("unix", "/tmp/mtgo-ipc.sock")
defer conn.Close()

req := `{"jsonrpc":"2.0","id":1,"method":"telegram.invoke","params":{"method":"help.getConfig","args":{}}}`
conn.Write(append([]byte(req), '\n'))

reader := bufio.NewReader(conn)
line, _ := reader.ReadBytes('\n')
fmt.Println(string(line))
// → {"jsonrpc":"2.0","result":{"_":"config",...},"id":1}
```

See [`examples/go-client/main.go`](examples/go-client/main.go) for a complete example.

### Node.js

```javascript
const net = require("net");
const conn = net.createConnection("/tmp/mtgo-ipc.sock");

conn.on("connect", () => {
  conn.write(JSON.stringify({
    jsonrpc: "2.0", id: 1, method: "telegram.invoke",
    params: { method: "help.getConfig", args: {} }
  }) + "\n");
});

let buf = "";
conn.on("data", (chunk) => {
  buf += chunk;
  const nl = buf.indexOf("\n");
  if (nl !== -1) {
    console.log(JSON.parse(buf.slice(0, nl)));
    conn.end();
  }
});
```

See [`examples/node-client/client.js`](examples/node-client/client.js) for a complete example.

### Python

```python
import socket, json

sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.connect("/tmp/mtgo-ipc.sock")

req = json.dumps({
    "jsonrpc": "2.0", "id": 1, "method": "telegram.invoke",
    "params": {"method": "help.getConfig", "args": {}}
})
sock.sendall((req + "\n").encode())

data = b""
while b"\n" not in data:
    data += sock.recv(65536)

print(json.loads(data.decode()))
sock.close()
```

### Shell (`socat` / `nc`)

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"health"}' | socat - UNIX-CONNECT:/tmp/mtgo-ipc.sock
```

## Project Layout

```
mtgo-ipc/
├── cmd/mtgo-ipc/        Server binary entry point (flags, signals, lifecycle)
├── server/              IPC server: accept loop + 8-method dispatch + sessions
├── protocol/            JSON-RPC 2.0 types (Request/Response/Event), error codes
├── transport/           Unix socket transport: listener, framing (newline JSON)
├── client/              mtgo integration: telegram.Client + InvokeJSON + OnRawUpdate
├── subscription/        Per-client bounded queues, slow-reader drop policy
├── examples/
│   ├── go-client/       Minimal Go client
│   ├── node-client/     Bun + TypeScript client
│   ├── python/          Python client (stdlib)
│   └── lua/             Lua client (LuaSocket + dkjson)
├── docs/                Architecture diagram + protocol reference
├── go.mod               github.com/mtgo-labs/mtgo v0.11.0 (no replace)
└── README.md
```

## Design Principles

1. **One transport, one protocol, one client.** Unix socket. JSON-RPC 2.0. A
   single mtgo session. No abstraction layers for things that don't exist yet.

2. **`telegram.invoke` is the API.** No `sendMessage`, `getMe`, `joinChannel`
   wrappers. Every Telegram operation is one method with a TL name and JSON
   args. This keeps the surface tiny and the schema automatically tracks mtgo's
   generated TL layer.

3. **Multi-client by design.** The accept loop spawns a goroutine per
   connection. Any number of IPC clients share one Telegram session
   simultaneously — the server serializes RPC calls through a single MTProto
   connection.

4. **Context propagation.** Every handler receives the request's `context.Context`,
   threaded from the server's lifecycle. Cancellation and deadlines flow through
   to `client.InvokeJSON`.

5. **Clean layer boundaries.** Only `client/` imports mtgo. The transport,
   protocol, and server packages are pure Go with zero external dependencies.
   This means the protocol contract can be tested and reasoned about in
   isolation.

## Status & Roadmap

### Working

- ✅ Unix socket transport with newline-delimited JSON framing
- ✅ JSON-RPC 2.0 protocol (request, response, error, event types)
- ✅ Eight IPC methods: `health`, `mtgo.info`, `mtgo.connect`, `mtgo.close`,
  `auth.status`, `telegram.invoke`, `updates.subscribe`, `updates.unsubscribe`
- ✅ Raw update streaming (push events to subscribed clients)
- ✅ Per-client bounded queues with slow-reader drop policy
- ✅ Real mtgo v0.11.0 integration (bot token + session string auth)
- ✅ Multi-client concurrent connections (each can subscribe independently)
- ✅ Graceful shutdown (SIGINT/SIGTERM)
- ✅ Client examples: Go, Bun/TypeScript, Python, Lua

### Not yet implemented

- Normalized/typed update events (only raw updates for now)
- WebSocket transport
- Multi-session support (multiple Telegram accounts)
- IPC authentication tokens
- TLS/encrypted socket
- Update persistence / replay
- High-level method wrappers (by design — `telegram.invoke` covers everything)

## License

[Apache License 2.0](LICENSE) — Copyright 2026 mtgo-labs
