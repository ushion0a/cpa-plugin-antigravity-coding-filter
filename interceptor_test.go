package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestHandlePluginCallRequestInterceptAfterRewritesOnlyAntigravityInRewriteMode(t *testing.T) {
	defer restoreDefaultFilterConfig(t)

	tests := []struct {
		name          string
		mode          filterMode
		toFormat      string
		wantRewritten bool
	}{
		{
			name:          "rewrites selected Antigravity destination",
			mode:          filterModeRewrite,
			toFormat:      "antigravity",
			wantRewritten: true,
		},
		{
			name:     "passes selected Gemini destination",
			mode:     filterModeRewrite,
			toFormat: "gemini",
		},
		{
			name:     "passes Antigravity destination in block mode",
			mode:     filterModeBlock,
			toFormat: "antigravity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			applyFilterConfig(filterConfig{Mode: tt.mode, UseDefaultKeywords: true})
			request := requestInterceptAfterRequestJSON(t, tt.toFormat, `{"system":"You are Codex.","messages":[]}`)

			// When
			raw, code := handlePluginCall("request.intercept_after", request)

			// Then
			if code != 0 {
				t.Fatalf("code = %d, want 0; body=%s", code, raw)
			}
			var envelope struct {
				OK     bool `json:"ok"`
				Result struct {
					Body string `json:"Body"`
				} `json:"result"`
			}
			mustUnmarshalJSON(t, raw, &envelope)
			if !envelope.OK {
				t.Fatalf("ok = false, want true")
			}
			if !tt.wantRewritten {
				if envelope.Result.Body != "" {
					t.Fatalf("Body = %q, want empty body to preserve original request", envelope.Result.Body)
				}
				return
			}

			body, err := base64.StdEncoding.DecodeString(envelope.Result.Body)
			if err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if !containsSystemText(t, body, "You are Antigravity.") {
				t.Fatalf("body = %s, want rewritten system", body)
			}
		})
	}
}

func requestInterceptAfterRequestJSON(t *testing.T, toFormat, body string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"SourceFormat":   "openai",
		"ToFormat":       toFormat,
		"Model":          "gemini-3.1-flash-lite",
		"RequestedModel": "gemini-3.1-flash-lite",
		"Body":           []byte(body),
	})
	if err != nil {
		t.Fatalf("marshal request intercept after request: %v", err)
	}
	return raw
}
