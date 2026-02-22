package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type mockForwarder struct {
	lastPath string
	lastBody []byte
	lastMeta ForwardMeta
	err      error
}

func (m *mockForwarder) Forward(_ context.Context, path string, body []byte, meta ForwardMeta) error {
	m.lastPath = path
	m.lastBody = append([]byte(nil), body...)
	m.lastMeta = meta
	return m.err
}

func testConfig() Config {
	return Config{
		ListenAddr:      ":0",
		ForwardURL:      "http://127.0.0.1:18088/services/collector/event",
		Token:           "",
		RequestTimeout:  time.Second,
		ShutdownTimeout: 2 * time.Second,
		MaxBodyBytes:    1024,
		LogLevel:        slog.LevelError,
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCollectSuccess(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok"}`))
	req.Header.Set("Authorization", "Splunk anything")
	req.Header.Set("User-Agent", "HomeAssistant/2026.2")
	req.RemoteAddr = "192.0.2.10:12345"
	req.Host = "example.test"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatalf("expected code 0 body, got %s", w.Body.String())
	}
	if mf.lastPath != "/services/collector/event" {
		t.Fatalf("unexpected forward path: %s", mf.lastPath)
	}
	var got map[string]any
	if err := json.Unmarshal(mf.lastBody, &got); err != nil {
		t.Fatalf("forwarded body is not JSON: %v", err)
	}
	if got["message"] != "ok" {
		t.Fatalf("unexpected forwarded body: %s", string(mf.lastBody))
	}
	if mf.lastMeta.UserAgent != "HomeAssistant/2026.2" {
		t.Fatalf("unexpected forwarded user-agent: %s", mf.lastMeta.UserAgent)
	}
	if mf.lastMeta.AuthHeader != "Splunk anything" {
		t.Fatalf("unexpected auth header: %s", mf.lastMeta.AuthHeader)
	}
	if mf.lastMeta.ClientIP != "192.0.2.10" {
		t.Fatalf("unexpected client ip: %s", mf.lastMeta.ClientIP)
	}
	if mf.lastMeta.Host != "example.test" {
		t.Fatalf("unexpected host: %s", mf.lastMeta.Host)
	}
	if mf.lastMeta.Proto != "http" {
		t.Fatalf("unexpected proto: %s", mf.lastMeta.Proto)
	}
}

func TestCollectUnauthorized(t *testing.T) {
	cfg := testConfig()
	cfg.Token = "topsecret"
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok"}`))
	req.Header.Set("Authorization", "Splunk wrong")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d", w.Code)
	}
	if mf.lastPath != "" {
		t.Fatalf("expected no forwarding, got path %s", mf.lastPath)
	}
}

func TestCollectForwardFailure(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{err: errors.New("boom")}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":9`) {
		t.Fatalf("expected code 9 body, got %s", w.Body.String())
	}
}

func TestCollectMethodNotAllowed(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodGet, "/services/collector/event", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 got %d", w.Code)
	}
}

func TestCollectNoData(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader("   "))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":5`) {
		t.Fatalf("expected code 5 body, got %s", w.Body.String())
	}
	if mf.lastPath != "" {
		t.Fatalf("expected no forwarding for no-data check, got %s", mf.lastPath)
	}
}

func TestCollectInvalidHECPayload(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"message":"not-hec"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":6`) {
		t.Fatalf("expected code 6 body, got %s", w.Body.String())
	}
	if mf.lastPath != "" {
		t.Fatalf("expected no forwarding for invalid payload, got %s", mf.lastPath)
	}
}

func TestCollectClientIPFromForwardedHeader(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok"}`))
	req.Header.Set("Forwarded", `for=198.51.100.7;proto=https, for=127.0.0.1`)
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("X-Real-IP", "10.0.0.5")
	req.RemoteAddr = "127.0.0.1:34567"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if mf.lastMeta.ClientIP != "198.51.100.7" {
		t.Fatalf("unexpected client ip from forwarded header: %s", mf.lastMeta.ClientIP)
	}
}

func TestCollectClientIPFromXForwardedForHeader(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok"}`))
	req.Header.Set("X-Forwarded-For", "198.51.100.8, 127.0.0.1")
	req.Header.Set("X-Real-IP", "10.0.0.5")
	req.RemoteAddr = "127.0.0.1:34567"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if mf.lastMeta.ClientIP != "198.51.100.8" {
		t.Fatalf("unexpected client ip from x-forwarded-for header: %s", mf.lastMeta.ClientIP)
	}
}
