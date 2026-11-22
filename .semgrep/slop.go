package tests

import (
	"context"
	"os"
	"sync"
	"time"
)

func doWork() error { return nil }
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
