package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		name        string
		defaultPath string
		requestPath string
		want        string
	}{
		{"root falls back", "/services/collector/event", "/", "/services/collector/event"},
		{"use request path", "/services/collector/event", "/services/collector", "/services/collector"},
		{"empty fallback", "", "", "/services/collector/event"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := normalizePath(tc.defaultPath, tc.requestPath)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestForwarderForward(t *testing.T) {
	var gotPath, gotAuth, gotBody, gotUserAgent string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotUserAgent = r.Header.Get("User-Agent")
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := Config{
		ForwardURL:     backend.URL + "/services/collector/event",
		RequestTimeout: time.Second,
		ForwardUA:      true,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := NewHTTPForwarder(cfg, logger)

	err := f.Forward(context.Background(), "/services/collector/event", []byte(`{"event":"hello"}`), "Splunk token", "HomeAssistant/2026.2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/services/collector/event" {
		t.Fatalf("unexpected path %s", gotPath)
	}
	if gotAuth != "Splunk token" {
		t.Fatalf("unexpected auth %s", gotAuth)
	}
	if gotBody != `{"event":"hello"}` {
		t.Fatalf("unexpected body %s", gotBody)
	}
	if gotUserAgent != "HomeAssistant/2026.2" {
		t.Fatalf("unexpected user-agent %s", gotUserAgent)
	}
}

func TestForwarderForwardUserAgentDisabled(t *testing.T) {
	var gotUserAgent string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := Config{
		ForwardURL:     backend.URL + "/services/collector/event",
		RequestTimeout: time.Second,
		ForwardUA:      false,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := NewHTTPForwarder(cfg, logger)

	err := f.Forward(context.Background(), "/services/collector/event", []byte(`{"event":"hello"}`), "", "HomeAssistant/2026.2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotUserAgent == "HomeAssistant/2026.2" {
		t.Fatalf("expected incoming user-agent not to be forwarded, got %s", gotUserAgent)
	}
}
