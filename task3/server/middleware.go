package server

import "net/http"

// Middleware wraps an http.Handler with a single cross-cutting concern.
type Middleware func(http.Handler) http.Handler

// Chain composes a sequence of middlewares applied outermost-first.
type Chain struct {
	middlewares []Middleware
}

// NewChain creates a chain from the given middlewares.
func NewChain(mws ...Middleware) *Chain {
	return &Chain{middlewares: mws}
}

// Append returns a new chain with additional middlewares appended.
func (c *Chain) Append(mws ...Middleware) *Chain {
	combined := make([]Middleware, len(c.middlewares)+len(mws))
	copy(combined, c.middlewares)
	copy(combined[len(c.middlewares):], mws)
	return &Chain{middlewares: combined}
}

// Then wraps handler with the full middleware chain and returns the composed http.Handler.
func (c *Chain) Then(h http.Handler) http.Handler {
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}
	return h
}
