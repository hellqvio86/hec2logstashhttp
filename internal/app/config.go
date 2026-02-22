package app

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddr      = ":8088"
	defaultForwardURL      = "http://127.0.0.1:18088/services/collector/event"
	defaultRequestTimeout  = 5 * time.Second
	defaultShutdownTimeout = 10 * time.Second
	defaultMaxBodyBytes    = int64(1 << 20)
)

type Config struct {
	ListenAddr      string
	ForwardURL      string
	ForwardUA       bool
	Token           string
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	MaxBodyBytes    int64
	LogLevel        slog.Level
}

func LoadConfigFromEnv() Config {
	return Config{
		ListenAddr:      getenv("HEC_LISTEN_ADDR", defaultListenAddr),
		ForwardURL:      getenv("HEC_FORWARD_URL", defaultForwardURL),
		ForwardUA:       parseBool("HEC_FORWARD_UA", false),
		Token:           strings.TrimSpace(os.Getenv("HEC_TOKEN")),
		RequestTimeout:  parseDuration("HEC_REQUEST_TIMEOUT", defaultRequestTimeout),
		ShutdownTimeout: parseDuration("HEC_SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
		MaxBodyBytes:    parseInt64("HEC_MAX_BODY_BYTES", defaultMaxBodyBytes),
		LogLevel:        parseLogLevel(getenv("HEC_LOG_LEVEL", "info")),
	}
}

func getenv(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}

func parseDuration(key string, fallback time.Duration) time.Duration {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}

	d, err := time.ParseDuration(val)
	if err != nil || d <= 0 {
		return fallback
	}

	return d
}

func parseInt64(key string, fallback int64) int64 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}

	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil || n <= 0 {
		return fallback
	}

	return n
}

func parseBool(key string, fallback bool) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}

	b, err := strconv.ParseBool(val)
	if err != nil {
		return fallback
	}

	return b
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
