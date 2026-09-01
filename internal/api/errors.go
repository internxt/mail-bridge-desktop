package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Sentinel errors callers can test for with errors.Is. ErrUnauthorized is the
// important one: it means the token is gone or expired and the account has to
// authenticate again.
var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrNotFound     = errors.New("not found")
	ErrRateLimited  = errors.New("rate limited")
	ErrServer       = errors.New("server error")
)

// maxBodyInError caps how much of a failed response we keep, so a large error
// page does not end up in the logs.
const maxBodyInError = 512

// Error is a failed API response.
type Error struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
	Err        error // one of the sentinels above, when it maps to one

	// retryAfter carries the server's Retry-After hint, when it sent one.
	retryAfter time.Duration
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("api: %s %s: %d", e.Method, e.URL, e.StatusCode)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

// Unwrap lets errors.Is reach the sentinel.
func (e *Error) Unwrap() error { return e.Err }

// sentinelFor maps a status code onto a sentinel error, or nil when there is
// no useful category for it.
func sentinelFor(statusCode int) error {
	switch {
	case statusCode == http.StatusUnauthorized, statusCode == http.StatusForbidden:
		return ErrUnauthorized
	case statusCode == http.StatusNotFound:
		return ErrNotFound
	case statusCode == http.StatusTooManyRequests:
		return ErrRateLimited
	case statusCode >= 500:
		return ErrServer
	}
	return nil
}

// isRetryableStatus reports whether a status code is worth retrying. A 400 or
// a 401 would fail the same way again, so only transient server-side problems
// qualify.
func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// isIdempotent reports whether a method can safely be sent twice.
//
// POST is deliberately excluded: retrying email/send would deliver the message
// twice, which is worse than surfacing the error.
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	}
	return false
}
