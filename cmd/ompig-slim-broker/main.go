// ompig-slim-broker hosts opaque profile documents. It contains no model runtime,
// provider transport, OAuth refresher or inference forwarding routes.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"ompig-slim/internal/broker"
)

// main reports startup/storage failures without printing any credentials.
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run owns one data directory and drains HTTP handlers before releasing its
// process lock. TLS/public ingress is the deployment operator's responsibility.
func run() error {
	bind := flag.String("bind", "127.0.0.1:8765", "HTTP listen address")
	dir := flag.String("data-dir", "./broker-data", "persistent hosted profile storage")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if err := os.MkdirAll(*dir, 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(*dir, "server.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("broker data directory already in use")
	}
	defer lock.Unlock()
	handler, err := broker.NewHandler(*dir, os.Getenv("OMPIG_BROKER_TOKEN"))
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: *bind, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	select {
	case err = <-result:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err = server.Shutdown(shutdown); err != nil {
			server.Close()
			return err
		}
		return nil
	}
}
