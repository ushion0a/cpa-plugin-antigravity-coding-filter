package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestInterceptAddsFallbackWithoutSystemKeyword(t *testing.T) {
	tests := []struct {
		name   string
		format string
		body   string
		system string
	}{
		{"string", "claude", `{"system":"Keep replies concise.","messages":[]}`, "system"},
		{"blocks", "claude", `{"system":[{"type":"text","text":"Keep replies concise.","cache_control":{"type":"ephemeral"}}],"messages":[]}`, "system"},
		{"empty string", "claude", `{"system":"","messages":[]}`, "system"},
		{"empty blocks", "claude", `{"system":[],"messages":[]}`, "system"},
		{"missing", "claude", `{"messages":[]}`, "system"},
		{"null", "claude", `{"system":null,"messages":[]}`, "system"},
		{"chat", "openai", `{"messages":[{"role":"system","content":"Keep replies concise."},{"role":"user","content":"hello"}]}`, "messages"},
		{"user identity is not system identity", "openai", `{"messages":[{"role":"user","content":"Antigravity"}]}`, "messages"},
		{"responses", "openai-response", `{"instructions":"Keep replies concise.","input":"hello"}`, "instructions"},
		{"gemini", "gemini", `{"system_instruction":{"parts":[{"text":"Keep replies concise."}]},"contents":[]}`, "system_instruction"},
	}
	for _, mode := range []filterMode{filterModeBlock, filterModeRewrite} {
		for _, tt := range tests {
			t.Run(string(mode)+"/"+tt.name, func(t *testing.T) {
				// Given
				defer restoreDefaultFilterConfig(t)
				applyFilterConfig(filterConfig{Mode: mode, UseDefaultKeywords: true})
				var original map[string]any
				mustUnmarshalJSON(t, []byte(tt.body), &original)

				// When
				body := interceptBody(t, tt.format, "antigravity", []byte(tt.body))

				// Then
				var result map[string]any
				mustUnmarshalJSON(t, body, &result)
				if strings.Count(collectText(result[tt.system]), antigravitySystemPrompt) != 1 {
					t.Fatalf("body = %s, want exactly one shipped fallback", body)
				}
				if strings.Contains(tt.body, "Keep replies concise.") &&
					!strings.Contains(collectText(result[tt.system]), "Keep replies concise.") {
					t.Fatalf("body = %s, original instruction lost", body)
				}
				if tt.system == "messages" {
					messages := result["messages"].([]any)
					if !reflect.DeepEqual(messages[1:], original["messages"]) {
						t.Fatalf("original messages changed: %s", body)
					}
				}
				delete(original, tt.system)
				delete(result, tt.system)
				if !reflect.DeepEqual(result, original) {
					t.Fatalf("non-system fields changed: got %#v, want %#v", result, original)
				}
			})
		}
	}
}

func TestInterceptFallbackPreservesAlreadyIdentifiedAndInvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		format string
		body   string
	}{
		{"identified string", "claude", `{"system":"You are ANTIGRAVITY."}`},
		{"identified blocks", "claude", `{"system":[{"type":"text","text":"You are Antigravity."}]}`},
		{"identified chat", "openai", `{"messages":[{"role":"developer","content":"You are Antigravity."}]}`},
		{"identified responses", "openai-response", `{"instructions":"You are Antigravity."}`},
		{"identified gemini", "gemini", `{"systemInstruction":{"parts":[{"text":"You are Antigravity."}]}}`},
		{"invalid json", "claude", `{`},
		{"non-object", "claude", `[]`},
		{"invalid system", "claude", `{"system":42}`},
		{"unknown format", "other", `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			defer restoreDefaultFilterConfig(t)
			applyFilterConfig(defaultFilterConfig())

			// When
			body := interceptBody(t, tt.format, "antigravity", []byte(tt.body))

			// Then
			if len(body) != 0 {
				t.Fatalf("body = %s, want original preserved", body)
			}
		})
	}
}

func TestInterceptFallbackKeepsSourceBlocksAndIsIdempotent(t *testing.T) {
	// Given
	defer restoreDefaultFilterConfig(t)
	applyFilterConfig(defaultFilterConfig())
	original := []byte(`{"system":[{"type":"text","text":"custom-rule","cache_control":{"type":"ephemeral"}}]}`)

	// When
	body := interceptBody(t, "claude", "antigravity", original)

	// Then
	var result struct {
		System []json.RawMessage `json:"system"`
	}
	mustUnmarshalJSON(t, body, &result)
	if len(result.System) != 2 {
		t.Fatalf("system = %s, want fallback plus original block", body)
	}
	var block map[string]any
	mustUnmarshalJSON(t, result.System[1], &block)
	if !reflect.DeepEqual(block, map[string]any{
		"type": "text", "text": "custom-rule", "cache_control": map[string]any{"type": "ephemeral"},
	}) {
		t.Fatalf("original block changed: %#v", block)
	}
	if repeated := interceptBody(t, "claude", "antigravity", body); len(repeated) != 0 {
		t.Fatalf("second interception changed body: %s", repeated)
	}
}

func TestInterceptFallbackDoesNotChangeOtherDestinationsOrMatchedBlockMode(t *testing.T) {
	for _, tt := range []struct {
		destination string
		body        string
	}{
		{"gemini", `{"system":"custom-rule"}`},
		{"", `{"system":"custom-rule"}`},
		{"antigravity", `{"system":"You are Codex."}`},
	} {
		t.Run(tt.destination+tt.body, func(t *testing.T) {
			// Given
			defer restoreDefaultFilterConfig(t)
			applyFilterConfig(defaultFilterConfig())

			// When
			body := interceptBody(t, "claude", tt.destination, []byte(tt.body))

			// Then
			if len(body) != 0 {
				t.Fatalf("body = %s, want unchanged request", body)
			}
		})
	}
}

func interceptBody(t *testing.T, source, destination string, body []byte) []byte {
	t.Helper()
	request, err := json.Marshal(map[string]any{
		"SourceFormat": source, "ToFormat": destination, "Body": body,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, code := handlePluginCall("request.intercept_after", request)
	var envelope struct {
		OK     bool                  `json:"ok"`
		Result struct{ Body []byte } `json:"result"`
	}
	mustUnmarshalJSON(t, raw, &envelope)
	if code != 0 || !envelope.OK {
		t.Fatalf("interceptor failed: code=%d, response=%s", code, raw)
	}
	return envelope.Result.Body
}
