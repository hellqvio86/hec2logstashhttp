package app

import (
	"encoding/json"
	"testing"
)

func TestNormalizeHECPayloadSingleEnvelope(t *testing.T) {
	in := []byte(`{"time":1730000000.25,"host":"edge01","event":"hello","fields":{"env":"prod"}}`)
	got, ok := normalizeHECPayload(in)
	if !ok {
		t.Fatalf("expected payload to be valid HEC")
	}

	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatalf("expected JSON object, got error %v", err)
	}

	if obj["message"] != "hello" {
		t.Fatalf("expected message field, got %#v", obj)
	}
	if obj["hec_host"] != "edge01" {
		t.Fatalf("expected hec_host, got %#v", obj)
	}
	if obj["env"] != "prod" {
		t.Fatalf("expected fields to be copied, got %#v", obj)
	}
	if _, ok := obj["@timestamp"].(string); !ok {
		t.Fatalf("expected @timestamp to be set, got %#v", obj)
	}
}

func TestNormalizeHECPayloadSingleEnvelopeWithObjectEvent(t *testing.T) {
	in := []byte(`{"event":{"message":"hello","severity":"info"},"source":"ha"}`)
	got, ok := normalizeHECPayload(in)
	if !ok {
		t.Fatalf("expected payload to be valid HEC")
	}

	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatalf("expected JSON object, got error %v", err)
	}

	if obj["message"] != "hello" {
		t.Fatalf("expected message from event object, got %#v", obj)
	}
	if obj["severity"] != "info" {
		t.Fatalf("expected severity from event object, got %#v", obj)
	}
	if obj["hec_source"] != "ha" {
		t.Fatalf("expected hec_source, got %#v", obj)
	}
}

func TestNormalizeHECPayloadBatch(t *testing.T) {
	in := []byte(`{"event":"one"}{"event":"two","fields":{"site":"lab"}}`)
	got, ok := normalizeHECPayload(in)
	if !ok {
		t.Fatalf("expected payload to be valid HEC")
	}

	var arr []map[string]any
	if err := json.Unmarshal(got, &arr); err != nil {
		t.Fatalf("expected JSON array, got error %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 events got %d", len(arr))
	}
	if arr[0]["message"] != "one" {
		t.Fatalf("unexpected first event %#v", arr[0])
	}
	if arr[1]["message"] != "two" {
		t.Fatalf("unexpected second event %#v", arr[1])
	}
	if arr[1]["site"] != "lab" {
		t.Fatalf("expected fields in second event %#v", arr[1])
	}
}

func TestNormalizeHECPayloadRejectsNonHEC(t *testing.T) {
	in := []byte(`{"message":"already-logstash"}`)
	got, ok := normalizeHECPayload(in)
	if ok {
		t.Fatalf("expected invalid HEC payload, got normalized %s", string(got))
	}
}
