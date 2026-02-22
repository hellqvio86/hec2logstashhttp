package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	hecEventKey      = "event"
	hecFieldsKey     = "fields"
	hecHostKey       = "host"
	hecIndexKey      = "index"
	hecSourceKey     = "source"
	hecSourcetypeKey = "sourcetype"
	hecTimeKey       = "time"
)

func normalizeHECPayload(body []byte) ([]byte, bool) {
	objects, ok := decodeJSONObjectStream(body)
	if !ok || len(objects) == 0 {
		return nil, false
	}

	converted := make([]map[string]any, 0, len(objects))
	for _, obj := range objects {
		entry, ok := convertHECObject(obj)
		if !ok {
			return nil, false
		}
		converted = append(converted, entry)
	}

	if len(converted) == 1 {
		out, err := json.Marshal(converted[0])
		if err != nil {
			return nil, false
		}
		return out, true
	}

	out, err := json.Marshal(converted)
	if err != nil {
		return nil, false
	}
	return out, true
}

func decodeJSONObjectStream(body []byte) ([]map[string]any, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var objects []map[string]any
	for {
		var value any
		if err := dec.Decode(&value); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, false
		}

		obj, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		objects = append(objects, obj)
	}

	return objects, true
}

func convertHECObject(in map[string]any) (map[string]any, bool) {
	rawEvent, ok := in[hecEventKey]
	if !ok {
		return nil, false
	}

	out := make(map[string]any)
	switch event := rawEvent.(type) {
	case map[string]any:
		for k, v := range event {
			out[k] = v
		}
	default:
		out["message"] = event
	}

	addIfPresent(out, "hec_host", in, hecHostKey)
	addIfPresent(out, "hec_source", in, hecSourceKey)
	addIfPresent(out, "hec_sourcetype", in, hecSourcetypeKey)
	addIfPresent(out, "hec_index", in, hecIndexKey)

	if ts, ok := parseHECTime(in[hecTimeKey]); ok {
		out["@timestamp"] = ts
	}

	if fields, ok := in[hecFieldsKey].(map[string]any); ok {
		for k, v := range fields {
			if _, exists := out[k]; !exists {
				out[k] = v
			}
		}
	}

	return out, true
}

func addIfPresent(out map[string]any, outKey string, in map[string]any, inKey string) {
	if v, ok := in[inKey]; ok {
		out[outKey] = v
	}
}

func parseHECTime(v any) (string, bool) {
	switch tv := v.(type) {
	case json.Number:
		if f, err := tv.Float64(); err == nil {
			return epochSecondsToRFC3339Nano(f), true
		}
	case float64:
		return epochSecondsToRFC3339Nano(tv), true
	case int64:
		return epochSecondsToRFC3339Nano(float64(tv)), true
	case int:
		return epochSecondsToRFC3339Nano(float64(tv)), true
	}
	return "", false
}

func epochSecondsToRFC3339Nano(seconds float64) string {
	nanos := int64(seconds * float64(time.Second))
	return time.Unix(0, nanos).UTC().Format(time.RFC3339Nano)
}

func applyInputRouting(body []byte, input resolvedInput) []byte {
	if len(body) == 0 {
		return body
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	switch typed := payload.(type) {
	case map[string]any:
		applyInputToEvent(typed, input)
	case []any:
		for _, item := range typed {
			event, ok := item.(map[string]any)
			if !ok {
				continue
			}
			applyInputToEvent(event, input)
		}
	default:
		return body
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return out
}

func applyInputToEvent(event map[string]any, input resolvedInput) {
	if tokenName := strings.TrimSpace(input.Name); tokenName != "" {
		event["hec_token_name"] = tokenName
	}
	if route := strings.TrimSpace(input.Route); route != "" {
		event["hec_route"] = route
	}
	if datastream := strings.TrimSpace(input.DataStream); datastream != "" {
		event["hec_datastream"] = datastream
	}
	if namespace := strings.TrimSpace(input.Namespace); namespace != "" {
		event["hec_namespace"] = namespace
	}

	defaultSourcetype := strings.TrimSpace(input.DefaultSourcetype)
	if defaultSourcetype != "" {
		current := strings.TrimSpace(asString(event["hec_sourcetype"]))
		if !input.AllowEventSourcetypeOverride || current == "" {
			event["hec_sourcetype"] = defaultSourcetype
		}
	}

	defaultSource := strings.TrimSpace(input.DefaultSource)
	if defaultSource != "" {
		current := strings.TrimSpace(asString(event["hec_source"]))
		if !input.AllowEventSourceOverride || current == "" {
			event["hec_source"] = defaultSource
		}
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
