package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/history"
)

type settings map[string]string

func (s settings) String(k string) string { return s[k] }

func TestCanonical(t *testing.T) {
	for in, want := range map[string]string{"": "claude", "Anthropic": "claude", "z.ai": "zai", "OLLAMA": "ollama", "grok": "xai"} {
		if got, err := Canonical(in); err != nil || got != want {
			t.Errorf("Canonical(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := Canonical("gemini"); err == nil || !strings.Contains(err.Error(), "no longer supported") {
		t.Errorf("gemini err = %v", err)
	}
	if _, err := Canonical("nope"); err == nil {
		t.Error("unknown provider accepted")
	}
}

func TestFromSettings(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "env-key")
	cfg, err := FromSettings(settings{"provider": "groq"})
	if err != nil || cfg.APIKey != "env-key" || cfg.Model != Presets["groq"].DefaultModel || cfg.BaseURL != Presets["groq"].BaseURL {
		t.Fatalf("cfg = %#v, %v", cfg, err)
	}
	cfg, _ = FromSettings(settings{"provider": "groq", "api-key": "generic", "groq-api-key": "specific", "model": "m"})
	if cfg.APIKey != "specific" || cfg.Model != "m" {
		t.Fatalf("precedence: %#v", cfg)
	}
	t.Setenv("DEEPSEEK_API_KEY", "")
	if _, err := FromSettings(settings{"provider": "deepseek"}); err == nil {
		t.Error("missing key accepted")
	}
	if cfg, err := FromSettings(settings{"provider": "ollama"}); err != nil || cfg.APIKey != "" {
		t.Errorf("ollama needs no key: %v", err)
	}
}

func TestOpenAICompatibleStream(t *testing.T) {
	var got oaRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("path %s auth %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		for _, part := range []string{"Hel", "lo"} {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", part)
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	p, _ := New(Config{Provider: "openai", BaseURL: srv.URL + "/v1", APIKey: "k", Model: "gpt-test"})
	var deltas []string
	reply, err := p.Stream(context.Background(), Request{System: "sys", Messages: []history.Message{{Role: "user", Content: "hi"}}}, func(d string) { deltas = append(deltas, d) })
	if err != nil || reply != "Hello" || len(deltas) != 2 {
		t.Fatalf("reply %q deltas %v err %v", reply, deltas, err)
	}
	if got.Model != "gpt-test" || !got.Stream || got.Messages[0].Role != "system" || got.Messages[1].Content != "hi" {
		t.Errorf("request = %#v", got)
	}
}

func TestOpenAIErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	p, _ := New(Config{Provider: "xai", BaseURL: srv.URL})
	if _, err := p.Stream(context.Background(), Request{Messages: []history.Message{{Role: "user", Content: "hi"}}}, nil); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestClaudeStream(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") || r.Header.Get("X-Api-Key") != "k" {
			t.Errorf("path %s key %q", r.URL.Path, r.Header.Get("X-Api-Key"))
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Bonjour"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" !"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
			`{"type":"message_stop"}`,
		}
		for _, e := range events {
			var typ struct{ Type string }
			_ = json.Unmarshal([]byte(e), &typ)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ.Type, e)
		}
	}))
	defer srv.Close()
	p, _ := New(Config{Provider: "claude", BaseURL: srv.URL, APIKey: "k", Model: "claude-opus-5-5", MaxTokens: 100})
	msgs := []history.Message{{Role: "assistant", Content: "dropped: must start with user"}, {Role: "user", Content: "a"}, {Role: "user", Content: "b"}, {Role: "assistant", Content: "c"}, {Role: "user", Content: "d"}}
	reply, err := p.Stream(context.Background(), Request{System: "sys", Messages: msgs}, nil)
	if err != nil || reply != "Bonjour !" {
		t.Fatalf("reply %q err %v", reply, err)
	}
	if body["model"] != "claude-opus-5-5" || body["fallbacks"] != "default" {
		t.Errorf("body model/fallbacks = %v / %v", body["model"], body["fallbacks"])
	}
	if m := body["messages"].([]any); len(m) != 3 {
		t.Errorf("messages not merged: %v", m)
	}
}
