package client

import (
	"log/slog"
	"time"
)

// Chain builds a composed HttpDoer by wrapping an innermost base outward.
// Recommended call order: WithRateLimit → WithRetry → WithCache → WithLogging
// so that logging captures full latency and cache short-circuits retries.
type Chain struct {
	doer HttpDoer
}

// NewChain starts a chain from any HttpDoer (typically *http.Client).
func NewChain(base HttpDoer) *Chain {
	return &Chain{doer: base}
}

func (c *Chain) WithRateLimit(rps float64, burst int) *Chain {
	c.doer = NewRateLimitDoer(c.doer, rps, burst)
	return c
}

func (c *Chain) WithRetry(maxRetries int, baseDelay time.Duration) *Chain {
	c.doer = NewRetryDoer(c.doer, maxRetries, baseDelay)
	return c
}

func (c *Chain) WithCache(ttl time.Duration) *Chain {
	c.doer = NewCachingDoer(c.doer, ttl)
	return c
}

func (c *Chain) WithLogging(logger *slog.Logger) *Chain {
	c.doer = NewLoggingDoer(c.doer, logger)
	return c
}

// Build returns the fully composed HttpDoer.
func (c *Chain) Build() HttpDoer {
	return c.doer
}
