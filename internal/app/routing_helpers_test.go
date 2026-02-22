package app

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplyInputRoutingBatch(t *testing.T) {
	in := []byte(`{"event":"a","sourcetype":"st-a"}{"event":"b"}`)
	normalized, ok := normalizeHECPayload(in)
	if !ok {
		t.Fatalf("expected valid payload")
	}

	out := applyInputRouting(normalized, resolvedInput{
		Name:                         "homeassistant",
		Route:                        "default",
		DataStream:                   "logs-homeassistant",
		Namespace:                    "prod",
		DefaultSourcetype:            "homeassistant:event",
		DefaultSource:                "homeassistant",
		AllowEventSourcetypeOverride: false,
		AllowEventSourceOverride:     false,
	})

	var arr []map[string]any
	if err := json.Unmarshal(out, &arr); err != nil {
		t.Fatalf("unexpected json: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("unexpected event count: %d", len(arr))
	}
	for _, event := range arr {
		if event["hec_datastream"] != "logs-homeassistant" {
			t.Fatalf("missing datastream: %#v", event)
		}
		if event["hec_source"] != "homeassistant" {
			t.Fatalf("expected default source: %#v", event)
		}
		if event["hec_sourcetype"] != "homeassistant:event" {
			t.Fatalf("expected forced sourcetype: %#v", event)
		}
	}
}

func TestApplyInputRoutingInvalidJSONPassThrough(t *testing.T) {
	in := []byte(`not-json`)
	out := applyInputRouting(in, resolvedInput{Name: "a"})
	if string(out) != string(in) {
		t.Fatalf("expected passthrough for invalid json")
	}
}

func TestPreviewBodyTruncates(t *testing.T) {
	in := []byte(strings.Repeat("a", maxBodyLogBytes+20))
	got := previewBody(in)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated preview to end with ellipsis")
	}
}

func TestRequestProtoPriority(t *testing.T) {
	req := httptest.NewRequest("POST", "/services/collector/event", nil)
	req.Header.Set("X-Forwarded-Proto", "HTTPS")
	if got := requestProto(req); got != "https" {
		t.Fatalf("unexpected proto: %s", got)
	}
}

func TestNormalizeClientIPCases(t *testing.T) {
	cases := map[string]string{
		`"198.51.100.9"`:     "198.51.100.9",
		"_hidden":            "",
		"[2001:db8::1]:1234": "2001:db8::1",
	}
	for in, want := range cases {
		if got := normalizeClientIP(in); got != want {
			t.Fatalf("normalizeClientIP(%q) = %q, want %q", in, got, want)
		}
	}
}
