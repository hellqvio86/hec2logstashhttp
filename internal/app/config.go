package app

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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
	InputsConfig    string
	Inputs          []InputProfile
	RejectUnknown   bool
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	MaxBodyBytes    int64
	LogLevel        slog.Level
}

type InputProfile struct {
	Token                        string `json:"token" yaml:"token"`
	Name                         string `json:"name" yaml:"name"`
	Route                        string `json:"route" yaml:"route"`
	ForwardURL                   string `json:"forward_url" yaml:"forward_url"`
	DataStream                   string `json:"datastream" yaml:"datastream"`
	Namespace                    string `json:"namespace" yaml:"namespace"`
	DefaultSourcetype            string `json:"default_sourcetype" yaml:"default_sourcetype"`
	DefaultSource                string `json:"default_source" yaml:"default_source"`
	AllowEventSourcetypeOverride bool   `json:"allow_event_sourcetype_override" yaml:"allow_event_sourcetype_override"`
	AllowEventSourceOverride     bool   `json:"allow_event_source_override" yaml:"allow_event_source_override"`
}

type inputProfilesFile struct {
	Inputs   []InputProfile `json:"inputs" yaml:"inputs"`
	Fallback struct {
		RejectUnknownTokens *bool `json:"reject_unknown_tokens" yaml:"reject_unknown_tokens"`
	} `json:"fallback" yaml:"fallback"`
}

func LoadConfigFromEnv() Config {
	cfg := Config{
		ListenAddr:      getenv("HEC_LISTEN_ADDR", defaultListenAddr),
		ForwardURL:      getenv("HEC_FORWARD_URL", defaultForwardURL),
		ForwardUA:       parseBool("HEC_FORWARD_UA", false),
		Token:           strings.TrimSpace(os.Getenv("HEC_TOKEN")),
		InputsConfig:    strings.TrimSpace(os.Getenv("HEC_INPUTS_CONFIG")),
		RejectUnknown:   true,
		RequestTimeout:  parseDuration("HEC_REQUEST_TIMEOUT", defaultRequestTimeout),
		ShutdownTimeout: parseDuration("HEC_SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
		MaxBodyBytes:    parseInt64("HEC_MAX_BODY_BYTES", defaultMaxBodyBytes),
		LogLevel:        parseLogLevel(getenv("HEC_LOG_LEVEL", "info")),
	}

	if cfg.InputsConfig == "" {
		return cfg
	}

	inputs, rejectUnknown := loadInputProfiles(cfg.InputsConfig)
	if len(inputs) > 0 {
		cfg.Inputs = inputs
		cfg.RejectUnknown = rejectUnknown
	}

	return cfg
}

func loadInputProfiles(path string) ([]InputProfile, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, true
	}

	var parsed inputProfilesFile
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, true
		}
	} else {
		if err := yaml.Unmarshal(raw, &parsed); err != nil {
			return nil, true
		}
	}

	out := make([]InputProfile, 0, len(parsed.Inputs))
	for _, input := range parsed.Inputs {
		input.Token = strings.TrimSpace(input.Token)
		if input.Token == "" {
			continue
		}
		input.Name = strings.TrimSpace(input.Name)
		input.Route = strings.TrimSpace(input.Route)
		input.ForwardURL = strings.TrimSpace(input.ForwardURL)
		input.DataStream = strings.TrimSpace(input.DataStream)
		input.Namespace = strings.TrimSpace(input.Namespace)
		input.DefaultSourcetype = strings.TrimSpace(input.DefaultSourcetype)
		input.DefaultSource = strings.TrimSpace(input.DefaultSource)
		out = append(out, input)
	}

	rejectUnknown := true
	if parsed.Fallback.RejectUnknownTokens != nil {
		rejectUnknown = *parsed.Fallback.RejectUnknownTokens
	}

	return out, rejectUnknown
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
