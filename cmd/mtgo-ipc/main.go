// Command mtgo-ipc is the IPC bridge server entry point.
//
// It starts a JSON-RPC server on a Unix domain socket. Telegram credentials are
// provided by the client via the "mtgo.connect" method — the server itself
// starts with no API keys or tokens.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mtgo-labs/mtgo-ipc/server"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("mtgo-ipc", flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(fs.Output(), "Usage: mtgo-ipc [--socket=<path>]")
		_, _ = fmt.Fprintln(fs.Output(), "mtgo IPC bridge server (JSON-RPC over Unix socket)")
		_, _ = fmt.Fprintln(fs.Output(), "Credentials are passed by the client via mtgo.connect.")
		fs.PrintDefaults()
	}

	var socketPath string
	fs.StringVar(&socketPath, "socket", envStr("MTGO_IPC_SOCKET", "/tmp/mtgo-ipc.sock"), "Unix socket path (or $MTGO_IPC_SOCKET)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}

	srv, err := server.New(server.Config{
		SocketPath: socketPath,
	})
	if err != nil {
		return fmt.Errorf("init server: %w", err)
	}
	defer func() { _ = srv.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("mtgo-ipc listening on %s", srv.Addr())
	if err := srv.Serve(ctx); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	log.Printf("mtgo-ipc stopped")
	return nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
