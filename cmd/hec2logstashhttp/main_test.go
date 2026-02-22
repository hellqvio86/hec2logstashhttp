package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersionFlag(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"-version"}, &out)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	text := out.String()
	if !strings.Contains(text, "version=") || !strings.Contains(text, "commit=") || !strings.Contains(text, "date=") {
		t.Fatalf("unexpected version output: %q", text)
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"--does-not-exist"}, &out)
	if code != 2 {
		t.Fatalf("expected exit code 2 for flag parse error, got %d", code)
	}
}
