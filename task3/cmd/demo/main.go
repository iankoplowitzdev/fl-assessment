// demo wires the full server middleware chain and client decorator chain together,
// routes requests through both, and logs observable state transitions.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"fl-assessment/task3/client"
	"fl-assessment/task3/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// --- Fake downstream service ---
	var downstreamHits atomic.Int64
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := downstreamHits.Add(1)
		logger.Info("downstream hit", "n", n, "path", r.URL.Path)
		fmt.Fprintf(w, `{"hit":%d}`, n)
	}))
	defer downstream.Close()

	// --- Client chain (used by the server handler to call the downstream) ---
	httpClient := downstream.Client() // uses the test server's TLS config if needed
	httpClient.Timeout = 5 * time.Second
	doer := client.NewChain(httpClient).
		WithRateLimit(50, 10).
		WithRetry(2, 10*time.Millisecond).
		WithCache(5 * time.Second).
		WithLogging(logger.With("component", "client")).
		Build()

	// --- Application handler: proxies to downstream via the client chain ---
	appHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), "GET", downstream.URL+r.URL.Path, nil)
		if err != nil {
			http.Error(w, "bad request", http.StatusInternalServerError)
			return
		}
		resp, err := doer.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
	})

	// --- Server middleware chain ---
	tokens := map[string]struct{}{"demo-token": {}}
	chain := server.NewChain(
		server.RequestID(),
		server.Logging(logger.With("component", "server")),
		server.RateLimit(10, 5),
		server.Auth(tokens),
	)

	frontendServer := httptest.NewServer(chain.Then(appHandler))
	defer frontendServer.Close()

	base := &http.Client{Timeout: 5 * time.Second}
	call := func(label, path, token string) {
		req, _ := http.NewRequest("GET", frontendServer.URL+path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := base.Do(req)
		if err != nil {
			fmt.Printf("[%-20s] ERROR: %v\n", label, err)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("[%-20s] %d  rid=%-18s  body=%s\n",
			label, resp.StatusCode,
			resp.Header.Get("X-Request-ID"),
			truncate(string(body), 40))
	}

	fmt.Print("\n=== Demo: layered HTTP middleware ===\n\n")

	call("no auth", "/data", "")
	call("wrong token", "/data", "bad-token")
	call("valid auth #1", "/data", "demo-token")
	call("valid auth #2 (cached)", "/data", "demo-token") // cache hit: downstream count unchanged
	call("different path", "/other", "demo-token")        // distinct cache key

	// Flood to trigger server-side rate limiting
	fmt.Println()
	passed, blocked := 0, 0
	for i := 0; i < 20; i++ {
		req, _ := http.NewRequest("GET", frontendServer.URL+"/flood", nil)
		req.Header.Set("Authorization", "Bearer demo-token")
		resp, _ := base.Do(req)
		if resp != nil {
			if resp.StatusCode == http.StatusTooManyRequests {
				blocked++
			} else {
				passed++
			}
			resp.Body.Close()
		}
	}
	fmt.Printf("[rate limit flood   ] passed=%d blocked=%d\n", passed, blocked)
	fmt.Printf("\nDownstream total hits: %d\n", downstreamHits.Load())
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
