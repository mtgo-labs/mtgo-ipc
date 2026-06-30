#!/usr/bin/env python3
"""Minimal mtgo-ipc client in Python (stdlib only).

Run after starting the server:
    python3 examples/python/client.py
"""
import json
import os
import socket
import sys


def rpc(sock: socket.socket, method: str, params: dict | None = None, rid: int = 1) -> dict:
    """Send one JSON-RPC request and read the response."""
    req = {"jsonrpc": "2.0", "id": rid, "method": method}
    if params is not None:
        req["params"] = params
    sock.sendall((json.dumps(req) + "\n").encode())

    data = b""
    while b"\n" not in data:
        chunk = sock.recv(65536)
        if not chunk:
            raise ConnectionError("server closed connection")
        data += chunk
    return json.loads(data.decode().strip())


def main() -> None:
    socket_path = os.environ.get("MTGO_IPC_SOCKET", "/tmp/mtgo-ipc.sock")

    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as sock:
        sock.connect(socket_path)

        # 1. Health check
        print("health:        ", json.dumps(rpc(sock, "health", rid=1)["result"]))

        # 2. Connect to Telegram — credentials passed from the client
        print("mtgo.connect:  ", json.dumps(rpc(sock, "mtgo.connect", {
            "api_id": int(os.environ.get("API_ID", 0)),
            "api_hash": os.environ.get("API_HASH", ""),
            "bot_token": os.environ.get("BOT_TOKEN", ""),
            "session": os.environ.get("SESSION", ""),
        }, rid=2)["result"]))

        # 3. Invoke a raw TL method
        resp = rpc(sock, "telegram.invoke", {"method": "help.getConfig", "args": {}}, rid=3)
        result = resp.get("result", {})
        print("getConfig dc:  ", result.get("this_dc", "?"))

        # 4. Check auth
        resp = rpc(sock, "auth.status", rid=4)
        print("auth.status:   ", json.dumps(resp["result"]))

        # 5. getMe — users.getFullUser with inputUserSelf
        resp = rpc(sock, "telegram.invoke", {
            "method": "users.getFullUser",
            "args": {"id": {"_": "inputUserSelf"}},
        }, rid=5)
        result = resp.get("result", {})
        users = result.get("users", [])
        if users:
            u = users[0]
            print(f"getMe:         id={u.get('id')} @{u.get('username')} bot={u.get('bot')} name={u.get('first_name')}")
        elif resp.get("error"):
            print("getMe error:  ", resp["error"]["message"])


if __name__ == "__main__":
    try:
        main()
    except (ConnectionError, FileNotFoundError) as e:
        print(f"error: {e}", file=sys.stderr)
        sys.exit(1)
