# Examples

Minimal clients demonstrating the mtgo-ipc JSON-RPC 2.0 contract.

## Go client

```bash
# Terminal 1 — start the server
go run ./cmd/mtgo-ipc --api-id 12345 --api-hash <hash>

# Terminal 2 — send a health ping
go run ./examples/go-client
```

## Node.js client

```bash
# After the server is running (Terminal 1 above)
node examples/node-client/client.js
```

## What each example does

Both connect to the Unix socket (`/tmp/mtgo-ipc.sock` by default), send a
`health` JSON-RPC request, and print the response. They show the wire protocol
from the client perspective — the same pattern generalises to every method:

```json
{"jsonrpc": "2.0", "id": 1, "method": "telegram.invoke", "params": {"method": "help.getConfig", "args": {}}}
```
