// Command example-go-client is a minimal mtgo-ipc client in Go.
//
// Run after starting the server:
//
//	go run ./examples/go-client
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
)

func main() {
	socketPath := "/tmp/mtgo-ipc.sock"
	if v := os.Getenv("MTGO_IPC_SOCKET"); v != "" {
		socketPath = v
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		log.Fatalf("dial %s: %v", socketPath, err)
	}
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	id := 0

	rpc := func(method string, params map[string]any) map[string]any {
		id++
		req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
		if params != nil {
			req["params"] = params
		}
		raw, _ := json.Marshal(req)
		if _, err := conn.Write(append(raw, '\n')); err != nil {
			log.Fatalf("write: %v", err)
		}
		line, err := reader.ReadBytes('\n')
		if err != nil {
			log.Fatalf("read: %v", err)
		}
		var resp map[string]any
		if err := json.Unmarshal(line, &resp); err != nil {
			log.Fatalf("decode: %v", err)
		}
		return resp
	}

	// 1. Health check
	r := rpc("health", nil)
	fmt.Println("health:       ", mustJSON(r["result"]))

	// 2. Connect to Telegram — credentials passed from the client
	r = rpc("mtgo.connect", map[string]any{
		"api_id":    envInt("API_ID"),
		"api_hash":  os.Getenv("API_HASH"),
		"bot_token": os.Getenv("BOT_TOKEN"),
		"session":   os.Getenv("SESSION"),
	})
	fmt.Println("mtgo.connect: ", mustJSON(r["result"]))

	// 3. getMe — users.getFullUser with inputUserSelf
	r = rpc("telegram.invoke", map[string]any{
		"method": "users.getFullUser",
		"args":   map[string]any{"id": map[string]any{"_": "inputUserSelf"}},
	})
	if users, ok := r["result"].(map[string]any)["users"].([]any); ok && len(users) > 0 {
		if u, ok := users[0].(map[string]any); ok {
			fmt.Printf("getMe:         id=%v username=@%v bot=%v name=%v\n",
				u["id"], u["username"], u["bot"], u["first_name"])
		}
	} else if errMsg, ok := r["error"]; ok {
		fmt.Println("getMe error:  ", mustJSON(errMsg))
	}

	// 4. Check auth
	r = rpc("auth.status", nil)
	fmt.Println("auth.status:  ", mustJSON(r["result"]))
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func envInt(key string) int {
	var n int
	fmt.Sscanf(os.Getenv(key), "%d", &n)
	return n
}
