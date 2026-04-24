package client

import (
	"net/http"

	"golang.org/x/time/rate"
)

type rateLimitDoer struct {
	inner   HttpDoer
	limiter *rate.Limiter
}

// NewRateLimitDoer wraps inner with a token-bucket rate limiter.
// rps is the sustained token replenishment rate; burst is the maximum token capacity.
func NewRateLimitDoer(inner HttpDoer, rps float64, burst int) HttpDoer {
	return &rateLimitDoer{
		inner:   inner,
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
}

func (r *rateLimitDoer) Do(req *http.Request) (*http.Response, error) {
	if err := r.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return r.inner.Do(req)
}
