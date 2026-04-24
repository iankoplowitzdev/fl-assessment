package client

import (
	"log/slog"
	"net/http"
	"time"
)

type loggingDoer struct {
	inner  HttpDoer
	logger *slog.Logger
}

// NewLoggingDoer wraps inner with structured request/response logging.
func NewLoggingDoer(inner HttpDoer, logger *slog.Logger) HttpDoer {
	return &loggingDoer{inner: inner, logger: logger}
}

func (l *loggingDoer) Do(req *http.Request) (*http.Response, error) {
	start := time.Now()
	l.logger.Info("outbound_request",
		"method", req.Method,
		"url", req.URL.String(),
	)

	resp, err := l.inner.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		l.logger.Error("outbound_error",
			"method", req.Method,
			"url", req.URL.String(),
			"elapsed_ms", elapsed.Milliseconds(),
			"error", err.Error(),
		)
		return nil, err
	}

	l.logger.Info("outbound_response",
		"method", req.Method,
		"url", req.URL.String(),
		"status", resp.StatusCode,
		"elapsed_ms", elapsed.Milliseconds(),
	)
	return resp, nil
}
