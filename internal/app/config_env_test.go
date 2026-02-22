package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigFromEnvLegacyMode(t *testing.T) {
	t.Setenv("HEC_LISTEN_ADDR", ":9090")
	t.Setenv("HEC_FORWARD_URL", "http://127.0.0.1:19000/services/collector/event")
	t.Setenv("HEC_FORWARD_UA", "true")
	t.Setenv("HEC_TOKEN", "legacy-token")
	t.Setenv("HEC_REQUEST_TIMEOUT", "9s")
	t.Setenv("HEC_SHUTDOWN_TIMEOUT", "11s")
	t.Setenv("HEC_MAX_BODY_BYTES", "4096")
	t.Setenv("HEC_LOG_LEVEL", "warn")
	t.Setenv("HEC_INPUTS_CONFIG", "")

	cfg := LoadConfigFromEnv()
	if cfg.ListenAddr != ":9090" {
		t.Fatalf("unexpected listen addr: %s", cfg.ListenAddr)
	}
	if cfg.ForwardURL != "http://127.0.0.1:19000/services/collector/event" {
		t.Fatalf("unexpected forward url: %s", cfg.ForwardURL)
	}
	if !cfg.ForwardUA {
		t.Fatalf("expected forward ua true")
	}
	if cfg.Token != "legacy-token" {
		t.Fatalf("unexpected token: %s", cfg.Token)
	}
	if cfg.RequestTimeout != 9*time.Second {
		t.Fatalf("unexpected request timeout: %s", cfg.RequestTimeout)
	}
	if cfg.ShutdownTimeout != 11*time.Second {
		t.Fatalf("unexpected shutdown timeout: %s", cfg.ShutdownTimeout)
	}
	if cfg.MaxBodyBytes != 4096 {
		t.Fatalf("unexpected max body bytes: %d", cfg.MaxBodyBytes)
	}
	if cfg.LogLevel != slog.LevelWarn {
		t.Fatalf("unexpected log level: %v", cfg.LogLevel)
	}
}

func TestLoadConfigFromEnvInputProfiles(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "inputs.yml")
	content := `
inputs:
  - token: "token-a"
    name: "homeassistant"
    route: "default"
    forward_url: "http://127.0.0.1:18088/services/collector/event"
    datastream: "logs-homeassistant"
fallback:
  reject_unknown_tokens: false
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("HEC_INPUTS_CONFIG", configPath)
	cfg := LoadConfigFromEnv()
	if len(cfg.Inputs) != 1 {
		t.Fatalf("expected one input, got %d", len(cfg.Inputs))
	}
	if cfg.RejectUnknown {
		t.Fatalf("expected reject unknown false")
	}
}

func TestParseEnvFallbacks(t *testing.T) {
	t.Setenv("HEC_REQUEST_TIMEOUT", "invalid")
	t.Setenv("HEC_SHUTDOWN_TIMEOUT", "0s")
	t.Setenv("HEC_MAX_BODY_BYTES", "-1")
	t.Setenv("HEC_FORWARD_UA", "not-bool")

	cfg := LoadConfigFromEnv()
	if cfg.RequestTimeout != defaultRequestTimeout {
		t.Fatalf("expected request timeout fallback: %s", cfg.RequestTimeout)
	}
	if cfg.ShutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("expected shutdown timeout fallback: %s", cfg.ShutdownTimeout)
	}
	if cfg.MaxBodyBytes != defaultMaxBodyBytes {
		t.Fatalf("expected max body bytes fallback: %d", cfg.MaxBodyBytes)
	}
	if cfg.ForwardUA {
		t.Fatalf("expected forward ua fallback false")
	}
}

func TestParseLogLevelCases(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"":        slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLogLevel(in); got != want {
			t.Fatalf("parseLogLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestGetenvFallback(t *testing.T) {
	t.Setenv("TEST_EMPTY", "")
	if got := getenv("TEST_EMPTY", "fallback"); got != "fallback" {
		t.Fatalf("unexpected fallback value: %s", got)
	}
}

func TestNewServerLifecycle(t *testing.T) {
	cfg := Config{
		ListenAddr:      "127.0.0.1:0",
		ForwardURL:      "http://127.0.0.1:18088/services/collector/event",
		RequestTimeout:  time.Second,
		ShutdownTimeout: 2 * time.Second,
		MaxBodyBytes:    1024,
		LogLevel:        slog.LevelError,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(cfg, logger)

	done := make(chan error, 1)
	go func() {
		done <- srv.ListenAndServe()
	}()

	time.Sleep(30 * time.Millisecond)
	_ = srv.Shutdown(context.Background())

	select {
	case err := <-done:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("unexpected server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}
