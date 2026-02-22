package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type HTTPForwarder struct {
	target    *url.URL
	client    *http.Client
	logger    *slog.Logger
	forwardUA bool
}

func NewHTTPForwarder(cfg Config, logger *slog.Logger) *HTTPForwarder {
	target, err := url.Parse(cfg.ForwardURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		target, _ = url.Parse(defaultForwardURL)
	}

	return &HTTPForwarder{
		target: target,
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
		logger:    logger,
		forwardUA: cfg.ForwardUA,
	}
}

func (f *HTTPForwarder) Forward(ctx context.Context, requestPath string, body []byte, authHeader string, userAgent string) error {
	forwardURL := *f.target
	forwardURL.Path = normalizePath(f.target.Path, requestPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, forwardURL.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(authHeader) != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if f.forwardUA && strings.TrimSpace(userAgent) != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	start := time.Now()
	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("forward request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	f.logger.Debug("forward complete", "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("forward returned status %d", resp.StatusCode)
	}

	return nil
}

func normalizePath(defaultPath, requestPath string) string {
	p := strings.TrimSpace(requestPath)
	if p == "" || p == "/" {
		if strings.TrimSpace(defaultPath) == "" {
			return "/services/collector/event"
		}
		return defaultPath
	}
	return path.Clean("/" + strings.TrimPrefix(p, "/"))
}
