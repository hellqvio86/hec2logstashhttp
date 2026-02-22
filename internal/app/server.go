package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	maxBodyLogBytes = 256
)

type Forwarder interface {
	Forward(ctx context.Context, path string, body []byte, meta ForwardMeta) error
}

type Server struct {
	httpServer *http.Server
}

func NewServer(cfg Config, logger *slog.Logger) *Server {
	forwarder := NewHTTPForwarder(cfg, logger)
	handler := newHandler(cfg, logger, forwarder)

	return &Server{
		httpServer: &http.Server{
			Addr:              cfg.ListenAddr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func newHandler(cfg Config, logger *slog.Logger, forwarder Forwarder) http.Handler {
	mux := http.NewServeMux()
	h := &hecHandler{
		cfg:       cfg,
		logger:    logger,
		forwarder: forwarder,
	}

	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/services/collector", h.collect)
	mux.HandleFunc("/services/collector/event", h.collect)

	return mux
}

type hecHandler struct {
	cfg       Config
	logger    *slog.Logger
	forwarder Forwarder
}

func (h *hecHandler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HECResponse{Text: "ok", Code: 0})
}

func (h *hecHandler) collect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, HECResponse{Text: "Method not allowed", Code: 6})
		return
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	userAgent := strings.TrimSpace(r.Header.Get("User-Agent"))
	if !h.authorized(authHeader) {
		writeJSON(w, http.StatusUnauthorized, HECResponse{Text: "Invalid token", Code: 4})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, HECResponse{Text: "Invalid data format", Code: 6})
		return
	}
	if len(bytes.TrimSpace(body)) == 0 {
		writeJSON(w, http.StatusOK, HECResponse{Text: "No data", Code: 5})
		return
	}

	forwardBody, ok := normalizeHECPayload(body)
	if !ok {
		writeJSON(w, http.StatusBadRequest, HECResponse{Text: "Invalid data format", Code: 6})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()

	meta := ForwardMeta{
		AuthHeader:      authHeader,
		UserAgent:       userAgent,
		ClientIP:        clientIPFromRequest(r),
		Host:            strings.TrimSpace(r.Host),
		Proto:           requestProto(r),
		XForwardedFor:   strings.TrimSpace(r.Header.Get("X-Forwarded-For")),
		XForwardedHost:  strings.TrimSpace(r.Header.Get("X-Forwarded-Host")),
		XForwardedProto: strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")),
		Forwarded:       strings.TrimSpace(r.Header.Get("Forwarded")),
	}

	if err := h.forwarder.Forward(ctx, r.URL.Path, forwardBody, meta); err != nil {
		h.logger.Warn("forward failed", "err", err, "path", r.URL.Path, "preview", previewBody(body))
		writeJSON(w, http.StatusServiceUnavailable, HECResponse{Text: "Server is busy", Code: 9})
		return
	}

	writeJSON(w, http.StatusOK, HECResponse{Text: "Success", Code: 0})
}

func requestProto(r *http.Request) string {
	if p := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); p != "" {
		return strings.ToLower(p)
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func clientIPFromRequest(r *http.Request) string {
	if ip := firstForwardedFor(strings.TrimSpace(r.Header.Get("Forwarded"))); ip != "" {
		return ip
	}
	if ip := firstXForwardedFor(strings.TrimSpace(r.Header.Get("X-Forwarded-For"))); ip != "" {
		return ip
	}
	if ip := normalizeClientIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != "" {
		return ip
	}
	return clientIPFromRemoteAddr(r.RemoteAddr)
}

func firstForwardedFor(value string) string {
	if value == "" {
		return ""
	}

	entries := strings.Split(value, ",")
	for _, entry := range entries {
		parts := strings.Split(entry, ";")
		for _, part := range parts {
			keyValue := strings.SplitN(part, "=", 2)
			if len(keyValue) != 2 {
				continue
			}

			if strings.ToLower(strings.TrimSpace(keyValue[0])) != "for" {
				continue
			}

			if candidate := normalizeClientIP(keyValue[1]); candidate != "" {
				return candidate
			}
		}
	}

	return ""
}

func firstXForwardedFor(value string) string {
	if value == "" {
		return ""
	}
	first := strings.Split(value, ",")[0]
	return normalizeClientIP(first)
}

func normalizeClientIP(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.Trim(value, "\"")
	if value == "" || strings.EqualFold(value, "unknown") {
		return ""
	}

	// RFC 7239 obfuscated identifier: for=_hidden
	if strings.HasPrefix(value, "_") {
		return ""
	}

	if strings.HasPrefix(value, "[") {
		// [IPv6] or [IPv6]:port
		if idx := strings.Index(value, "]"); idx > 1 {
			host := strings.TrimSpace(value[1:idx])
			if host != "" {
				return host
			}
		}
		return ""
	}

	if host, _, err := net.SplitHostPort(value); err == nil {
		return strings.TrimSpace(host)
	}

	return value
}

func (h *hecHandler) authorized(authHeader string) bool {
	if h.cfg.Token == "" {
		return true
	}

	if authHeader == "" {
		return false
	}

	prefix := "splunk "
	tokenCandidate := authHeader
	if strings.HasPrefix(strings.ToLower(authHeader), prefix) {
		tokenCandidate = strings.TrimSpace(authHeader[len(prefix):])
	}

	return tokenCandidate == h.cfg.Token
}

func writeJSON(w http.ResponseWriter, status int, payload HECResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func previewBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if len(body) <= maxBodyLogBytes {
		return string(body)
	}
	return string(bytes.TrimSpace(body[:maxBodyLogBytes])) + "..."
}
