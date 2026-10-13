// Package provider talks to language models. One interface covers Claude
// (Anthropic API) and every OpenAI-compatible service; the active one is
// picked with "set provider as <name>".
package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/history"
)

// Request is one chat completion. Generation parameters left zero come
// from the provider's Config.
type Request struct {
	Model     string
	System    string
	Messages  []history.Message
	MaxTokens int
}

// Legacy generation parameters (the original's generation_config).
const (
	DefaultMaxTokens   = 2048
	DefaultTemperature = 0.9
	DefaultTopP        = 1.0
)

// HTTPError is a provider error carrying the HTTP status.
type HTTPError struct {
	Status int
	Msg    string
}

func (e *HTTPError) Error() string { return e.Msg }

// IsRateLimited reports whether err is an HTTP 429 from a provider.
func IsRateLimited(err error) bool {
	var h *HTTPError
	if errors.As(err, &h) && h.Status == http.StatusTooManyRequests {
		return true
	}
	return isAnthropicStatus(err, http.StatusTooManyRequests)
}

// Provider streams a reply, calling onDelta for each text fragment, and
// returns the full reply.
type Provider interface {
	Name() string
	Stream(ctx context.Context, req Request, onDelta func(string)) (string, error)
}

// Kind separates the two wire protocols.
type Kind int

const (
	Anthropic Kind = iota
	OpenAICompatible
)

// Preset describes a supported service.
type Preset struct {
	Name         string
	Kind         Kind
	BaseURL      string
	DefaultModel string
	KeyEnv       string
	KeyOptional  bool
}

// Presets are the services "set provider as ..." accepts.
var Presets = map[string]Preset{
	"claude":     {Name: "claude", Kind: Anthropic, DefaultModel: "claude-opus-5-5", KeyEnv: "ANTHROPIC_API_KEY", KeyOptional: true},
	"openai":     {Name: "openai", Kind: OpenAICompatible, BaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-4o", KeyEnv: "OPENAI_API_KEY"},
	"zai":        {Name: "zai", Kind: OpenAICompatible, BaseURL: "https://api.z.ai/api/paas/v4", DefaultModel: "glm-4.6", KeyEnv: "ZAI_API_KEY"},
	"ollama":     {Name: "ollama", Kind: OpenAICompatible, BaseURL: "http://localhost:11434/v1", DefaultModel: "llama3.2", KeyOptional: true},
	"openrouter": {Name: "openrouter", Kind: OpenAICompatible, BaseURL: "https://openrouter.ai/api/v1", DefaultModel: "openai/gpt-4o-mini", KeyEnv: "OPENROUTER_API_KEY"},
	"groq":       {Name: "groq", Kind: OpenAICompatible, BaseURL: "https://api.groq.com/openai/v1", DefaultModel: "llama-3.3-70b-versatile", KeyEnv: "GROQ_API_KEY"},
	"deepseek":   {Name: "deepseek", Kind: OpenAICompatible, BaseURL: "https://api.deepseek.com/v1", DefaultModel: "deepseek-chat", KeyEnv: "DEEPSEEK_API_KEY"},
	"xai":        {Name: "xai", Kind: OpenAICompatible, BaseURL: "https://api.x.ai/v1", DefaultModel: "grok-4", KeyEnv: "XAI_API_KEY"},
}

var aliases = map[string]string{
	"anthropic": "claude",
	"z.ai":      "zai",
	"grok":      "xai",
}

// DefaultProvider is used when no provider has been set.
const DefaultProvider = "claude"

// Names lists the supported provider names.
func Names() []string {
	var out []string
	for k := range Presets {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Canonical maps a user-typed provider name to its preset name.
func Canonical(name string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return DefaultProvider, nil
	}
	if a, ok := aliases[n]; ok {
		n = a
	}
	if n == "gemini" || n == "vertex" {
		return "", fmt.Errorf("%s is no longer supported; choose one of: %s", name, strings.Join(Names(), ", "))
	}
	if _, ok := Presets[n]; !ok {
		return "", fmt.Errorf("unknown provider %q; choose one of: %s", name, strings.Join(Names(), ", "))
	}
	return n, nil
}

// Config is the resolved provider selection and generation parameters.
type Config struct {
	Provider    string
	Model       string
	APIKey      string
	BaseURL     string
	MaxTokens   int
	Temperature float64
	TopP        float64
}

// Settings is the subset of the settings store the provider reads.
type Settings interface{ String(key string) string }

// FromSettings resolves the provider from the settings keys
// provider, model, base-url, <provider>-api-key, api-key and the
// provider's environment variable, in that order of precedence for keys.
// The generation parameters max-tokens, temperature and top-p default to
// the legacy constants.
func FromSettings(s Settings) (Config, error) {
	name, err := Canonical(s.String("provider"))
	if err != nil {
		return Config{}, err
	}
	p := Presets[name]
	cfg := Config{Provider: name, Model: s.String("model"), BaseURL: s.String("base-url"),
		MaxTokens: DefaultMaxTokens, Temperature: DefaultTemperature, TopP: DefaultTopP}
	if n, err := strconv.Atoi(strings.TrimSpace(s.String("max-tokens"))); err == nil && n > 0 {
		cfg.MaxTokens = n
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(s.String("temperature")), 64); err == nil {
		cfg.Temperature = f
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(s.String("top-p")), 64); err == nil {
		cfg.TopP = f
	}
	if cfg.Model == "" {
		cfg.Model = p.DefaultModel
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = p.BaseURL
	}
	cfg.APIKey = s.String(name + "-api-key")
	if cfg.APIKey == "" {
		cfg.APIKey = s.String("api-key")
	}
	if cfg.APIKey == "" && p.KeyEnv != "" {
		cfg.APIKey = os.Getenv(p.KeyEnv)
	}
	if cfg.APIKey == "" && !p.KeyOptional {
		return cfg, fmt.Errorf("no API key for %s: run \"set %s-api-key as <key>\" or export %s", name, name, p.KeyEnv)
	}
	return cfg, nil
}

// New builds the provider for cfg.
func New(cfg Config) (Provider, error) {
	p, ok := Presets[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
	if p.Kind == Anthropic {
		return newClaude(cfg), nil
	}
	return newOpenAI(cfg), nil
}
