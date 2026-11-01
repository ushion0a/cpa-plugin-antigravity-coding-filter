package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// Identity prompt from CLIProxyAPI v7.2.93's antigravity executor.
const antigravitySystemPrompt = "You are Antigravity, a powerful agentic AI coding assistant designed by the Google Deepmind team working on Advanced Agentic Coding.You are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question.**Absolute paths only****Proactiveness**"

func addAntigravitySystemPrompt(body []byte, sourceFormat string) ([]byte, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil || root == nil {
		return nil, false
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, false
	}

	if system, exists := root["system"]; exists {
		next, changed := prependSystemPrompt(system)
		if !changed {
			return nil, false
		}
		root["system"] = next
	} else {
		switch sourceFormat {
		case "claude", "anthropic":
			root["system"] = antigravitySystemPrompt
		case "openai", "chat-completions":
			messages, ok := root["messages"].([]any)
			if root["messages"] != nil && !ok {
				return nil, false
			}
			for _, item := range messages {
				message, ok := item.(map[string]any)
				if ok && (message["role"] == "system" || message["role"] == "developer") &&
					strings.Contains(strings.ToLower(collectText(message["content"])), "antigravity") {
					return nil, false
				}
			}
			root["messages"] = append([]any{map[string]any{
				"role": "system", "content": antigravitySystemPrompt,
			}}, messages...)
		case "openai-response", "responses":
			next, changed := prependSystemPrompt(root["instructions"])
			if !changed {
				return nil, false
			}
			root["instructions"] = next
		case "gemini":
			key := "system_instruction"
			if _, exists := root["systemInstruction"]; exists {
				key = "systemInstruction"
			}
			system, ok := root[key].(map[string]any)
			if root[key] != nil && !ok {
				return nil, false
			}
			if system == nil {
				system = make(map[string]any)
			}
			if strings.Contains(strings.ToLower(collectText(system)), "antigravity") {
				return nil, false
			}
			parts, ok := system["parts"].([]any)
			if system["parts"] != nil && !ok {
				return nil, false
			}
			system["parts"] = append([]any{map[string]any{"text": antigravitySystemPrompt}}, parts...)
			root[key] = system
		default:
			return nil, false
		}
	}

	raw, err := json.Marshal(root)
	if err != nil {
		return nil, false
	}
	return raw, true
}

func prependSystemPrompt(value any) (any, bool) {
	if strings.Contains(strings.ToLower(collectText(value)), "antigravity") {
		return value, false
	}
	switch typed := value.(type) {
	case nil:
		return antigravitySystemPrompt, true
	case string:
		if typed == "" {
			return antigravitySystemPrompt, true
		}
		return antigravitySystemPrompt + "\n\n" + typed, true
	case []any:
		return append([]any{map[string]any{"type": "text", "text": antigravitySystemPrompt}}, typed...), true
	default:
		return value, false
	}
}
