package client

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"time"
)

type cacheEntry struct {
	status  int
	header  http.Header
	body    []byte
	expiry  time.Time
}

type cachingDoer struct {
	inner HttpDoer
	ttl   time.Duration
	mu    sync.Mutex
	store map[string]*cacheEntry
}

// NewCachingDoer wraps inner with a TTL-based in-memory response cache.
// Only successful (2xx) GET responses are cached.
func NewCachingDoer(inner HttpDoer, ttl time.Duration) HttpDoer {
	return &cachingDoer{
		inner: inner,
		ttl:   ttl,
		store: make(map[string]*cacheEntry),
	}
}

func (c *cachingDoer) Do(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return c.inner.Do(req)
	}

	key := req.Method + " " + req.URL.String()

	c.mu.Lock()
	entry, ok := c.store[key]
	c.mu.Unlock()

	if ok && time.Now().Before(entry.expiry) {
		return c.buildResponse(entry), nil
	}

	resp, err := c.inner.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		c.mu.Lock()
		c.store[key] = &cacheEntry{
			status: resp.StatusCode,
			header: resp.Header.Clone(),
			body:   body,
			expiry: time.Now().Add(c.ttl),
		}
		c.mu.Unlock()

		resp.Body = io.NopCloser(bytes.NewReader(body))
	}

	return resp, nil
}

func (c *cachingDoer) buildResponse(e *cacheEntry) *http.Response {
	return &http.Response{
		StatusCode: e.status,
		Header:     e.header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(e.body)),
	}
}
