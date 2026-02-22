package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadInputProfilesYAMLDefaultsRejectUnknown(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "inputs.yml")
	content := `
inputs:
  - token: "token-a"
    name: "a"
    route: "default"
    forward_url: "http://127.0.0.1:18088/services/collector/event"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	inputs, rejectUnknown := loadInputProfiles(configPath)
	if len(inputs) != 1 {
		t.Fatalf("expected one input, got %d", len(inputs))
	}
	if !rejectUnknown {
		t.Fatalf("expected rejectUnknown true by default")
	}
}

func TestLoadInputProfilesJSONWithFallbackOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "inputs.json")
	content := `{
  "inputs": [
    {
      "token": "token-a",
      "name": "a",
      "route": "default",
      "forward_url": "http://127.0.0.1:18088/services/collector/event"
    }
  ],
  "fallback": {
    "reject_unknown_tokens": false
  }
}`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	inputs, rejectUnknown := loadInputProfiles(configPath)
	if len(inputs) != 1 {
		t.Fatalf("expected one input, got %d", len(inputs))
	}
	if rejectUnknown {
		t.Fatalf("expected rejectUnknown false from fallback override")
	}
}
