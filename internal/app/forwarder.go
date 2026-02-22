package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type ForwardMeta struct {
	AuthHeader      string
	UserAgent       string
	ClientIP        string
	Host            string
	Proto           string
	XForwardedFor   string
	XForwardedHost  string
	XForwardedProto string
	Forwarded       string
}

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

func (f *HTTPForwarder) Forward(ctx context.Context, requestPath string, body []byte, meta ForwardMeta) error {
	forwardURL := *f.target
	forwardURL.Path = normalizePath(f.target.Path, requestPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, forwardURL.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(meta.AuthHeader) != "" {
		req.Header.Set("Authorization", meta.AuthHeader)
	}
	if f.forwardUA && strings.TrimSpace(meta.UserAgent) != "" {
		req.Header.Set("User-Agent", meta.UserAgent)
	}

	if clientIP := strings.TrimSpace(meta.ClientIP); clientIP != "" {
		req.Header.Set("X-Real-IP", clientIP)
	}

	xff := buildXForwardedFor(meta.XForwardedFor, meta.ClientIP)
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}

	xfh := strings.TrimSpace(meta.XForwardedHost)
	if xfh == "" {
		xfh = strings.TrimSpace(meta.Host)
	}
	if xfh != "" {
		req.Header.Set("X-Forwarded-Host", xfh)
	}

	xfp := strings.ToLower(strings.TrimSpace(meta.XForwardedProto))
	if xfp == "" {
		xfp = strings.ToLower(strings.TrimSpace(meta.Proto))
	}
	if xfp != "" {
		req.Header.Set("X-Forwarded-Proto", xfp)
	}

	forwarded := buildForwarded(meta.Forwarded, meta.ClientIP, xfp, xfh)
	if forwarded != "" {
		req.Header.Set("Forwarded", forwarded)
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

func clientIPFromRemoteAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return strings.TrimSpace(host)
}

func buildXForwardedFor(existing string, clientIP string) string {
	existing = strings.TrimSpace(existing)
	clientIP = strings.TrimSpace(clientIP)
	switch {
	case existing == "":
		return clientIP
	case clientIP == "":
		return existing
	default:
		return existing + ", " + clientIP
	}
}

func buildForwarded(existing string, clientIP string, proto string, host string) string {
	parts := make([]string, 0, 3)

	clientIP = strings.TrimSpace(clientIP)
	if clientIP != "" {
		if strings.Contains(clientIP, ":") {
			parts = append(parts, fmt.Sprintf(`for="%s"`, "["+clientIP+"]"))
		} else {
			parts = append(parts, "for="+clientIP)
		}
	}

	proto = strings.ToLower(strings.TrimSpace(proto))
	if proto != "" {
		parts = append(parts, "proto="+proto)
	}

	host = strings.TrimSpace(host)
	if host != "" {
		parts = append(parts, fmt.Sprintf(`host="%s"`, host))
	}

	entry := strings.Join(parts, ";")
	existing = strings.TrimSpace(existing)

	switch {
	case existing == "":
		return entry
	case entry == "":
		return existing
	default:
		return existing + ", " + entry
	}
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
