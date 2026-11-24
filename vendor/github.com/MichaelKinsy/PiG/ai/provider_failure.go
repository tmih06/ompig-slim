package ai

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ProviderFailure contains recovery facts, never response bodies or credentials.
// RetryAt is an evidence-backed deadline in epoch milliseconds, not a local cooldown.
type ProviderFailure struct {
	Kind    string `json:"kind"`
	Status  int    `json:"status,omitempty"`
	Code    string `json:"code,omitempty"`
	RetryAt int64  `json:"retryAt,omitempty"`
}

func (failure ProviderFailure) Retryable() bool {
	return failure.Kind == "transient" || failure.Kind == "rate_limit" || failure.Kind == "quota" && failure.RetryAt > 0
}

type providerFailureError struct {
	cause   error
	failure ProviderFailure
}

func (err *providerFailureError) Error() string { return err.cause.Error() }
func (err *providerFailureError) Unwrap() error { return err.cause }

func providerWireError(message, body string) error {
	return &providerFailureError{cause: errors.New(message), failure: ClassifyProviderFailure(&providerError{message: body})}
}

// ClassifyProviderFailure is the shared boundary for native and custom providers.
func ClassifyProviderFailure(err error) ProviderFailure {
	return classifyProviderFailureAt(err, time.Now())
}

func classifyProviderFailureAt(err error, now time.Time) ProviderFailure {
	if err == nil {
		return ProviderFailure{Kind: "unknown"}
	}
	if carried, ok := errors.AsType[*providerFailureError](err); ok {
		return carried.failure
	}
	failure := ProviderFailure{}
	normalized := NormalizeProviderError(err)
	if request, ok := errors.AsType[*ClassifierHTTPError](err); ok {
		normalized = NormalizeProviderError(request.providerError())
	}
	if normalized.Status != nil {
		failure.Status = *normalized.Status
	}
	var headers http.Header
	if response, ok := errors.AsType[ProviderRequestError](err); ok {
		headers = response.ProviderHeaders()
		if status := response.ProviderStatus(); status != nil {
			failure.Status = *status
		}
	}
	if exceeded, ok := errors.AsType[*providerRetryDelayError](err); ok {
		if exceeded.headers != nil {
			headers = exceeded.headers
		}
		if exceeded.status != 0 {
			failure.Status = exceeded.status
		}
	}
	wait := max(normalized.UsageResetAfter, retryAfterFromHeadersAt(headers, now.UnixMilli()))
	text := err.Error()
	body := text
	if normalized.Body != nil {
		body = *normalized.Body
	}
	code, bodyWait := providerFailureBody(body, now)
	failure.Code = code
	if sdk, ok := errors.AsType[interface {
		error
		ErrorCode() string
	}](err); ok {
		failure.Code = sdk.ErrorCode()
	}
	if response, ok := errors.AsType[interface {
		error
		HTTPStatusCode() int
	}](err); ok {
		failure.Status = response.HTTPStatusCode()
	}
	wait = max(wait, bodyWait, ProviderRetryHintDelay(text))
	if wait > 0 {
		failure.RetryAt = now.Add(wait).UnixMilli()
	}
	lower := strings.ToLower(failure.Code + " " + text)
	switch {
	case errors.Is(err, context.Canceled) || strings.Contains(lower, "request aborted"):
		failure.Kind = "cancelled"
	case failure.Status == 401 || strings.Contains(lower, "invalid_api_key") || strings.Contains(lower, "authentication_error"):
		failure.Kind = "auth"
	case failure.Status == 403 || strings.Contains(lower, "permission_denied") || strings.Contains(lower, "accessdenied"):
		failure.Kind = "permission"
	case failure.Status == 402 || strings.Contains(lower, "billing") || strings.Contains(lower, "payment_required"):
		failure.Kind = "billing"
	case IsAccountQuotaErrorMessage(lower) || strings.Contains(lower, "quota reset") || strings.Contains(lower, "quota_exhausted") || strings.Contains(lower, "quotaexceeded") || failure.Code == "1308":
		failure.Kind = "quota"
	case failure.Status >= 400 && failure.Status < 500 && failure.Status != 408 && failure.Status != 409 && failure.Status != 429:
		failure.Kind = "invalid"
	case failure.Status == 429 || strings.Contains(lower, "resource_exhausted") || strings.Contains(lower, "throttl") || strings.Contains(lower, "rate_limit"):
		failure.Kind = "rate_limit"
	case failure.Status == 408 || failure.Status == 409 || failure.Status >= 500 && failure.Status <= 599 || strings.Contains(lower, "overloaded_error") || retryableProviderErrorPattern.MatchString(text):
		failure.Kind = "transient"
	default:
		failure.Kind = "unknown"
	}
	if headers.Get("x-should-retry") == "false" && failure.Retryable() {
		failure.Kind = "permanent"
	}
	return failure
}

func providerFailureBody(body string, now time.Time) (string, time.Duration) {
	var fields map[string]json.RawMessage
	if start := strings.IndexByte(body, '{'); start >= 0 {
		body = body[start:]
	}
	if json.Unmarshal([]byte(body), &fields) != nil {
		return "", 0
	}
	if response := fields["response"]; len(response) > 0 {
		var nested map[string]json.RawMessage
		if json.Unmarshal(response, &nested) == nil && len(nested["error"]) > 0 {
			fields = nested
		}
	}
	if nested := fields["error"]; len(nested) > 0 {
		var inner map[string]json.RawMessage
		if json.Unmarshal(nested, &inner) == nil && inner != nil {
			fields = inner
		}
	}
	code := ""
	for _, key := range []string{"code", "type", "status"} {
		var value string
		if json.Unmarshal(fields[key], &value) == nil && value != "" {
			code = value
			break
		}
		if key == "code" {
			var number json.Number
			if json.Unmarshal(fields[key], &number) == nil && number != "" {
				code = number.String()
				break
			}
		}
	}
	var wait time.Duration
	for _, field := range []struct {
		name  string
		scale time.Duration
	}{{"retry_after_ms", time.Millisecond}, {"retry_after", time.Second}, {"resets_in_seconds", time.Second}} {
		var number json.Number
		if json.Unmarshal(fields[field.name], &number) == nil {
			wait = max(wait, providerDelay(number.String(), field.scale))
		}
	}
	var reset json.Number
	if json.Unmarshal(fields["resets_at"], &reset) == nil {
		if seconds, err := strconv.ParseFloat(reset.String(), 64); err == nil && seconds > 0 && seconds < float64(math.MaxInt64/int64(time.Second)) {
			wait = max(wait, time.Unix(0, int64(seconds*float64(time.Second))).Sub(now))
		}
	}
	var details []struct {
		Type       string `json:"@type"`
		RetryDelay string `json:"retryDelay"`
	}
	if json.Unmarshal(fields["details"], &details) == nil {
		for _, detail := range details {
			if strings.HasSuffix(detail.Type, "google.rpc.RetryInfo") {
				if delay, err := time.ParseDuration(detail.RetryDelay); err == nil {
					wait = max(wait, delay)
				}
			}
		}
	}
	return code, wait
}

func providerDelay(value string, scale time.Duration) time.Duration {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 || number >= float64(math.MaxInt64)/float64(scale) {
		return 0
	}
	return time.Duration(math.Ceil(number * float64(scale)))
}

// RecordProviderFailure preserves recovery facts in the existing diagnostic wire format.
func RecordProviderFailure(message *AssistantMessage, err error) {
	failure := ClassifyProviderFailure(err)
	if failure.Kind == "unknown" {
		return
	}
	details := map[string]any{"kind": failure.Kind, "status": failure.Status, "code": failure.Code, "retryAt": failure.RetryAt}
	AppendAssistantMessageDiagnostic(message, AssistantMessageDiagnostic{Type: "provider_failure", Timestamp: time.Now().UnixMilli(), Details: details})
}

// ProviderFailureForMessage validates persisted diagnostics once at the read boundary.
func ProviderFailureForMessage(message AssistantMessage) ProviderFailure {
	for _, diagnostic := range message.Diagnostics {
		if diagnostic.Type != "provider_failure" {
			continue
		}
		raw, err := json.Marshal(diagnostic.Details)
		if err != nil {
			continue
		}
		var failure ProviderFailure
		if json.Unmarshal(raw, &failure) != nil || failure.RetryAt < 0 || failure.RetryAt > time.Now().UnixMilli()+math.MaxInt64/int64(time.Millisecond) {
			continue
		}
		switch failure.Kind {
		case "transient", "rate_limit", "quota", "auth", "permission", "billing", "invalid", "cancelled", "permanent":
			return failure
		}
	}
	return ClassifyProviderFailure(errors.New(message.ErrorMessage))
}

// RetryDelayForAssistant never shortens a provider's deadline to the backoff ceiling.
func RetryDelayForAssistant(policy RetryPolicy, attempt int, message AssistantMessage) int {
	delay := RetryDelayMs(policy, attempt)
	failure := ProviderFailureForMessage(message)
	if failure.RetryAt > 0 {
		delay = max(delay, int(max(int64(0), failure.RetryAt-time.Now().UnixMilli())))
	}
	return delay
}
