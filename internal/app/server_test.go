package app

import (
	"context"
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
	lastAuth string
	err      error
}

func (m *mockForwarder) Forward(_ context.Context, path string, body []byte, authHeader string) error {
	m.lastPath = path
	m.lastBody = append([]byte(nil), body...)
	m.lastAuth = authHeader
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
