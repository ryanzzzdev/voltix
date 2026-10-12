// upstream-mock is the standalone telemetry receiver used by the
// container demo and fault harness. It serves the durable transactional
// receiver (internal/upstream) with its documented test-only control
// routes (/control/*). Not a production component.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"voltix/internal/upstream"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8081", "telemetry listen address")
	dir := flag.String("dir", "", "state directory for receiver.db (required)")
	flag.Parse()
	token := os.Getenv("VOLTIX_TELEMETRY_TOKEN")
	if *dir == "" || token == "" {
		fmt.Fprintln(os.Stderr, "upstream-mock requires -dir and VOLTIX_TELEMETRY_TOKEN")
		os.Exit(2)
	}
	r, err := upstream.OpenReceiver(*dir, token)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open receiver:", err)
		os.Exit(1)
	}
	defer r.Close()

	srv := &http.Server{
		Addr:              *listen,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return context.Background() },
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()
	fmt.Println("upstream-mock listening on", *listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}
