package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	maxAttempts = 3
	baseBackoff = 200 * time.Millisecond
	maxBackoff  = 5 * time.Second
)

type request struct {
	svc    *service
	method string
	path   string
	query  url.Values
	token  string
	body   any
}

// Performs the request using `doRaw` and unmarshals the response into `out`.
func (c *Client) do(ctx context.Context, req request, out any) error {
	raw, err := c.doRaw(ctx, req)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("api: %s %s: decode response: %w", req.method, req.path, err)
	}
	return nil
}

// Performs the request and returns the response untouched to be handled outside.
func (c *Client) doRaw(ctx context.Context, req request) ([]byte, error) {
	// Marshal once and keep the bytes: a retry needs to send the same body
	// again, and an io.Reader cannot be replayed once consumed.
	var payload []byte
	if req.body != nil {
		var err error
		payload, err = json.Marshal(req.body)
		if err != nil {
			return nil, fmt.Errorf("api: encode request: %w", err)
		}
	}

	endpoint := req.svc.baseURL + req.path
	if len(req.query) > 0 {
		endpoint += "?" + req.query.Encode()
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			if err := sleep(ctx, backoffFor(attempt, lastErr)); err != nil {
				return nil, err
			}
		}

		body, status, err := c.attempt(ctx, req, endpoint, payload)
		if err == nil {
			return body, nil
		}
		lastErr = err

		retryable := isIdempotent(req.method) && (status == 0 || isRetryableStatus(status))
		if !retryable || attempt == maxAttempts || ctx.Err() != nil {
			return nil, err
		}
		c.log.Warn("%s %s failed (attempt %d/%d): %v", req.method, endpoint, attempt, maxAttempts, err)
	}
	return nil, lastErr
}

// attempt performs a single request.
func (c *Client) attempt(ctx context.Context, req request, endpoint string, payload []byte) ([]byte, int, error) {
	var reader io.Reader
	userToken := req.token
	if payload != nil {
		reader = bytes.NewReader(payload)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.method, endpoint, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("api: build request: %w", err)
	}

	httpReq.Header = req.svc.headers()
	injectHeaders(httpReq.Header, userToken, payload)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("api: %s %s: %w", req.method, endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("api: %s %s: read body: %w", req.method, endpoint, err)
	}

	if resp.StatusCode >= 300 {
		return nil, resp.StatusCode, &Error{
			StatusCode: resp.StatusCode,
			Method:     req.method,
			URL:        endpoint,
			Body:       truncate(string(body), maxBodyInError),
			Err:        sentinelFor(resp.StatusCode),
			retryAfter: retryAfter(resp.Header),
		}
	}
	return body, resp.StatusCode, nil
}

func injectHeaders(header http.Header, token string, payload []byte) {
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	if payload != nil {
		header.Set("Content-Type", "application/json; charset=utf-8")
	}
}

func backoffFor(attempt int, lastErr error) time.Duration {
	var apiErr *Error
	if errors.As(lastErr, &apiErr) && apiErr.retryAfter > 0 {
		return min(apiErr.retryAfter, maxBackoff)
	}

	d := min(baseBackoff*time.Duration(1<<(attempt-2)), maxBackoff)
	jitter := time.Duration(rand.Int63n(int64(d/2 + 1)))
	return d + jitter
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func retryAfter(h http.Header) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
