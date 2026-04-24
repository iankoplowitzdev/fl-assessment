package client

import (
	"bytes"
	"io"
	"math/rand"
	"net/http"
	"time"
)

type retryDoer struct {
	inner      HttpDoer
	maxRetries int
	baseDelay  time.Duration
}

// NewRetryDoer wraps inner with exponential backoff + jitter retry on network errors and 5xx responses.
func NewRetryDoer(inner HttpDoer, maxRetries int, baseDelay time.Duration) HttpDoer {
	return &retryDoer{inner: inner, maxRetries: maxRetries, baseDelay: baseDelay}
}

func (r *retryDoer) Do(req *http.Request) (*http.Response, error) {
	// Buffer the body once so each attempt gets a fresh reader.
	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
	}

	var (
		lastResp *http.Response
		lastErr  error
	)

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(r.backoff(attempt - 1)):
			}
		}

		clone := req.Clone(req.Context())
		if bodyBytes != nil {
			clone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			clone.ContentLength = int64(len(bodyBytes))
		}

		resp, err := r.inner.Do(clone)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode < 500 {
			return resp, nil
		}

		// Drain and close the body before retrying to allow connection reuse.
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		lastResp = resp
		lastErr = nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return lastResp, nil
}

func (r *retryDoer) backoff(attempt int) time.Duration {
	delay := r.baseDelay * (1 << attempt)
	jitter := time.Duration(rand.Int63n(int64(r.baseDelay) + 1))
	return delay + jitter
}
