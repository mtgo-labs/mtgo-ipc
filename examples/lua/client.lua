#!/usr/bin/env lua
-- Minimal mtgo-ipc client in Lua (requires LuaSocket + dkjson or cjson).
--
-- Install:
--   luarocks install luasocket
--   luarocks install dkjson          -- or use cjson if available
--
-- Run after starting the server:
--   lua examples/lua/client.lua

local socket = require("socket")
local unix   = require("socket.unix")
local json   = require("dkjson")  -- alternative: local json = require("cjson")

local SOCKET_PATH = os.getenv("MTGO_IPC_SOCKET") or "/tmp/mtgo-ipc.sock"

-- --- helpers ---

local function rpc(sock, id, method, params)
    local req = { jsonrpc = "2.0", id = id, method = method }
    if params then req.params = params end

    sock:send(json.encode(req) .. "\n")

    local line, err = sock:receive("*l")
    if err then error("read: " .. err) end
    return json.decode(line)
end

-- --- connect to the Unix socket ---

local sock = unix()
sock:settimeout(30)

local ok, err = sock:connect(SOCKET_PATH)
if not ok then
    io.stderr:write("connect error: " .. (err or "?") .. "\n")
    os.exit(1)
end

-- --- protocol flow ---

-- 1. Health check
local r = rpc(sock, 1, "health")
print("health:        " .. json.encode(r.result))

-- 2. Connect to Telegram — credentials passed from the client
local api_id = tonumber(os.getenv("API_ID") or "0")
r = rpc(sock, 2, "mtgo.connect", {
    api_id    = api_id,
    api_hash  = os.getenv("API_HASH") or "",
    bot_token = os.getenv("BOT_TOKEN") or "",
    session   = os.getenv("SESSION") or "",
})
print("mtgo.connect:  " .. json.encode(r.result))

-- 3. Invoke a raw TL method
-- Note: Lua empty tables encode as [] in dkjson, so omit args when empty.
r = rpc(sock, 3, "telegram.invoke", { method = "help.getConfig" })
if r.result and r.result.this_dc then
    print("getConfig dc:  " .. r.result.this_dc)
elseif r.error then
    print("getConfig err: " .. r.error.message)
end

-- 4. Check auth
r = rpc(sock, 4, "auth.status")
print("auth.status:   " .. json.encode(r.result))

-- 5. getMe — users.getFullUser with inputUserSelf
r = rpc(sock, 5, "telegram.invoke", {
    method = "users.getFullUser",
    args = { id = { ["_"] = "inputUserSelf" } }
})
if r.result and r.result.users and r.result.users[1] then
    local u = r.result.users[1]
    print(string.format("getMe:         id=%s @%s bot=%s name=%s",
        tostring(u.id or "?"), tostring(u.username or "?"),
        tostring(u.bot or "?"), tostring(u.first_name or "?")))
elseif r.error then
    print("getMe err:     " .. r.error.message)
end

sock:close()
