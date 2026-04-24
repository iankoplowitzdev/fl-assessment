// demo wires the full server middleware chain and client decorator chain together,
// routes requests through both, and logs observable state transitions.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"fl-assessment/task3/client"
	"fl-assessment/task3/server"
)

const (
	listenAddr    = ":8080"
	downstreamURL = "http://localhost:8081" // nginx — run: docker compose up
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// --- Client chain (proxies requests to nginx) ---
	doer := client.NewChain(&http.Client{Timeout: 5 * time.Second}).
		WithRateLimit(50, 10).
		WithRetry(2, 10*time.Millisecond).
		WithCache(5 * time.Second).
		WithLogging(logger.With("component", "client")).
		Build()

	// --- Application handler: proxies to nginx via the client chain ---
	appHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), "GET", downstreamURL+r.URL.Path, nil)
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
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
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

	srv := &http.Server{Addr: listenAddr, Handler: chain.Then(appHandler)}

	go func() {
		logger.Info("server starting", "addr", listenAddr, "downstream", downstreamURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	time.Sleep(100 * time.Millisecond)
	runDemo()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func runDemo() {
	base := &http.Client{Timeout: 5 * time.Second}
	baseURL := "http://localhost" + listenAddr

	call := func(label, path, token string) {
		req, _ := http.NewRequest("GET", baseURL+path, nil)
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

	call("no auth", "/", "")
	call("wrong token", "/", "bad-token")
	call("valid auth #1", "/", "demo-token")
	call("valid auth #2 (cached)", "/", "demo-token")
	call("different path", "/other", "demo-token")

	fmt.Println()
	passed, blocked := 0, 0
	for i := 0; i < 20; i++ {
		req, _ := http.NewRequest("GET", baseURL+"/flood", nil)
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
	fmt.Printf("[rate limit flood   ] passed=%d blocked=%d\n\n", passed, blocked)
	fmt.Println("Press Ctrl+C to stop.")
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
