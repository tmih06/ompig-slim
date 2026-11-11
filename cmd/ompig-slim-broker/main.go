// ompig-slim-broker owns the SQLite credential vault and OAuth refresher.
// It also hosts preferences, but never forwards inference or constructs a runtime.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"ompig-slim/internal/broker"
)

// main reports startup/storage failures without printing any credentials. A
// published image must be able to identify itself, so a release build injects
// its version with -ldflags "-X main.version=…"; the default marks a local build.
func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("ompig-slim-broker %s\n", version)
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// version is the build identity reported by --version and the release tag.
var version = "dev"

// run owns one data directory and drains HTTP handlers before releasing its
// process lock. TLS/public ingress is the deployment operator's responsibility.
func run() error {
	bind := flag.String("bind", "127.0.0.1:8765", "HTTP listen address")
	dir := flag.String("data-dir", "./broker-data", "persistent hosted profile storage")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	// Validate configuration before touching the filesystem: a missing secret is an
	// operator mistake, and failing here leaves no data directory or lock behind.
	token := os.Getenv("OMPIG_BROKER_TOKEN")
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("OMPIG_BROKER_TOKEN is required: set it to a long random secret and keep it out of committed files")
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
	handler, err := broker.NewHandler(*dir, token)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	refreshed := make(chan struct{})
	go func() {
		defer close(refreshed)
		handler.RunRefresher(ctx, func(err error) { fmt.Fprintln(os.Stderr, err) })
	}()
	defer func() {
		// Also stop the sweep when listener startup fails without a signal.
		stop()
		<-refreshed
		handler.Close()
	}()
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
