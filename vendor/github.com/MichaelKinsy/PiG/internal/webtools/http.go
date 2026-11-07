// Package webtools implements credential-free search and local web document rendering.
package webtools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DefaultMaxBytes int64 = 50 * 1024 * 1024

// HTTPResponse owns a bounded body and the final URL after redirects.
type HTTPResponse struct {
	URL        string
	Header     http.Header
	Body       []byte
	Truncated  bool
	StatusCode int
}

// NewHTTPClient returns a request-scoped client. Call CloseIdleConnections when done.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 300 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			MaxIdleConns:          4,
		},
	}
}

type transportError struct{ cause error }

func (e transportError) Error() string { return "HTTP transport failed" }
func (e transportError) Unwrap() error { return e.cause }

var userAgents = [...]string{
	"curl/8.0",
	"Mozilla/5.0 (compatible; ompig-slim/0.1; text reader)",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36",
}

// Load retries refusals with a different browser profile, never beyond the caller's
// deadline. Provider bodies and request URLs are deliberately absent from errors:
// those can contain credentials even on nominally anonymous endpoints.
func Load(ctx context.Context, client *http.Client, method, target string, body []byte, headers http.Header, maxBytes int64) (HTTPResponse, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if client == nil {
		client = NewHTTPClient()
		defer client.CloseIdleConnections()
	}
	attempts := len(userAgents)
	if headers.Get("User-Agent") != "" {
		attempts = 1
	}
	for attempt := range attempts {
		if err := ctx.Err(); err != nil {
			return HTTPResponse{}, err
		}
		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
		if err != nil {
			return HTTPResponse{}, fmt.Errorf("invalid HTTP request")
		}
		req.Header = headers.Clone()
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", userAgents[attempt])
		}
		req.Header.Set("Accept-Encoding", "identity")
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return HTTPResponse{}, ctx.Err()
			}
			return HTTPResponse{}, transportError{err}
		}
		result := HTTPResponse{URL: resp.Request.URL.String(), Header: resp.Header, StatusCode: resp.StatusCode}
		if retryableStatus(resp.StatusCode) && attempt+1 < attempts {
			resp.Body.Close()
			if err := waitRetry(ctx, resp.Header.Get("Retry-After")); err != nil {
				return HTTPResponse{}, err
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden:
				return result, fmt.Errorf("authorization failed (%d)", resp.StatusCode)
			case http.StatusTooManyRequests:
				return result, fmt.Errorf("rate limited (429); try again later or reorder webSearch.chain")
			default:
				return result, fmt.Errorf("HTTP status %d", resp.StatusCode)
			}
		}
		// Read one extra byte to distinguish an exact-cap response from truncation.
		result.Body, err = io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
		resp.Body.Close()
		if err != nil {
			if ctx.Err() != nil {
				return HTTPResponse{}, ctx.Err()
			}
			return HTTPResponse{}, transportError{err}
		}
		if int64(len(result.Body)) > maxBytes {
			result.Body = result.Body[:maxBytes]
			result.Truncated = true
		}
		return result, nil
	}
	panic("unreachable HTTP retry state")
}

func retryableStatus(status int) bool {
	return status == 403 || status == 429 || status == 503 || status == 999
}

func waitRetry(ctx context.Context, value string) error {
	delay := time.Duration(0)
	if seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32); err == nil {
		if seconds > 0 {
			delay = time.Duration(seconds) * time.Second
		}
	} else if date, err := http.ParseTime(value); err == nil {
		delay = time.Until(date)
	}
	if delay <= 0 {
		return ctx.Err()
	}
	if delay > 10*time.Second {
		delay = 10 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
