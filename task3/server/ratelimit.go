package server

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type clientEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipRateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientEntry
	rps     rate.Limit
	burst   int
}

// RateLimit returns a middleware enforcing per-client-IP token bucket rate limiting.
// Clients exceeding the limit receive 429. Stale entries are evicted every minute.
func RateLimit(rps float64, burst int) Middleware {
	rl := &ipRateLimiter{
		clients: make(map[string]*clientEntry),
		rps:     rate.Limit(rps),
		burst:   burst,
	}
	go rl.cleanupLoop()
	return rl.middleware
}

func (rl *ipRateLimiter) limiterFor(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.clients[ip]
	if !ok {
		e = &clientEntry{limiter: rate.NewLimiter(rl.rps, rl.burst)}
		rl.clients[ip] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

func (rl *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		if !rl.limiterFor(ip).Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *ipRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, e := range rl.clients {
			if time.Since(e.lastSeen) > 5*time.Minute {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}
