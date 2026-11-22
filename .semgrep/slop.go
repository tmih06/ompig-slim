package tests

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"
)

func doWork() error                             { return nil }
func fetch(ctx context.Context) (string, error) { return "", nil }

// ---- slop.blank-error-assign ----
func badBlank() {
	// ruleid: slop.blank-error-assign
	_ = doWork()
}

func goodReturn() error {
	// ok: slop.blank-error-assign
	return doWork()
}

func goodCleanup() {
	f, _ := os.Open("/tmp/x")
	// ok: slop.blank-error-assign
	_ = f.Close()
}

// ---- slop.empty-err-check ----
func badEmpty() {
	err := doWork()
	// ruleid: slop.empty-err-check
	if err != nil {
	}
	_ = err
}

func goodErr() error {
	if err := doWork(); err != nil {
		return err
	}
	return nil
}

// ---- slop.bare-goroutine ----
func badGo() {
	// ruleid: slop.bare-goroutine
	go func() {
		doWork()
	}()
}

func goodGo(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	// ok: slop.bare-goroutine
	go func() {
		defer close(done)
		doWork()
	}()
	return done
}

func goodGoWg(wg *sync.WaitGroup) {
	wg.Add(1)
	// ok: slop.bare-goroutine
	go func() {
		defer wg.Done()
		doWork()
	}()
}

// ---- slop.context-in-request ----
func badCtx(ctx context.Context) {
	// ruleid: slop.context-in-request
	root := context.Background()
	_ = root
}

func goodCtx(ctx context.Context) {
	child, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = child
}

// ---- slop.reimplement-privatefs ----
func badPrivate() {
	// ruleid: slop.reimplement-privatefs
	os.MkdirAll("/root/.cred", 0o700)
	// ruleid: slop.reimplement-privatefs
	os.WriteFile("/root/.cred/token", []byte("x"), 0o600)
}

func goodPrivate() {
	// ok: slop.reimplement-privatefs
	os.MkdirAll("/tmp/public", 0o755)
}

// ---- slop.os-exit-in-package ----
func badExit() {
	// ruleid: slop.os-exit-in-package
	os.Exit(1)
}

// ---- slop.defer-in-loop ----
func badDeferLoop(paths []string) {
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		// ruleid: slop.defer-in-loop
		defer f.Close()
	}
}

func goodDeferHelper(p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	// ok: slop.defer-in-loop
	defer f.Close()
	return nil
}

// ---- slop.time-after-in-loop ----
func badTimeAfterLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		// ruleid: slop.time-after-in-loop
		case <-time.After(time.Second):
		}
	}
}

func goodTickerLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		// ok: slop.time-after-in-loop
		case <-ticker.C:
		}
	}
}

// ---- slop.waitgroup-add-in-goroutine ----
func badWgAdd(wg *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		// ruleid: slop.waitgroup-add-in-goroutine
		wg.Add(1)
		defer wg.Done()
	}()
}

// ---- slop.http-default-client-or-mux ----
func badHTTP() (*http.Response, error) {
	// ruleid: slop.http-default-client-or-mux
	return http.Get("https://example.com")
}

func goodHTTP(ctx context.Context) (*http.Response, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", nil)
	if err != nil {
		return nil, err
	}
	// ok: slop.http-default-client-or-mux
	return client.Do(req)
}

// ---- slop.decimal-file-mode ----
func badDecimalMode() error {
	// ruleid: slop.decimal-file-mode
	if err := os.MkdirAll("/tmp/dir", 755); err != nil {
		return err
	}
	// ruleid: slop.decimal-file-mode
	return os.WriteFile("/tmp/dir/file", []byte("x"), 644)
}

func goodOctalMode() error {
	// ok: slop.decimal-file-mode
	return os.WriteFile("/tmp/dir/file", []byte("x"), 0o644)
}

// ---- slop.errorf-sprintf ----
func badErrorfSprintf(name string) error {
	// ruleid: slop.errorf-sprintf
	return errors.New(fmt.Sprintf("bad %s", name))
}

func goodErrorf(name string) error {
	// ok: slop.errorf-sprintf
	return fmt.Errorf("bad %s", name)
}

// ---- slop.raw-signal-notify ----
func badSignalNotify() {
	ch := make(chan os.Signal, 1)
	// ruleid: slop.raw-signal-notify
	signal.Notify(ch, os.Interrupt)
}

func goodSignalNotify(ctx context.Context) {
	// ok: slop.raw-signal-notify
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	_ = sigCtx
}
