package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// contextKey is unexported to prevent collisions with other packages.
type contextKey int

const requestIDKey contextKey = iota

// RequestIDFromContext returns the request ID stored by the RequestID middleware.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// RequestID injects a unique request ID into the request context and the X-Request-ID
// response header. Passes through any existing X-Request-ID from the incoming request.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if id == "" {
				id = newRequestID()
			}
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			w.Header().Set("X-Request-ID", id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
