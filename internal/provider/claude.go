package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/history"
)

// fallbackModels accept the server-side refusal fallback ("default" form).
var fallbackModels = map[string]bool{
	"claude-fable-5-1":  true,
	"claude-opus-5-5":   true,
	"claude-opus-5":     true,
	"claude-sonnet-5-5": true,
}

type claude struct {
	cfg    Config
	client anthropic.Client
}

func newClaude(cfg Config) *claude {
	var opts []option.RequestOption
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &claude{cfg: cfg, client: anthropic.NewClient(opts...)}
}

func (c *claude) Name() string { return "claude" }

func (c *claude) Stream(ctx context.Context, req Request, onDelta func(string)) (string, error) {
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.cfg.MaxTokens
	}
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		Messages:  toBetaMessages(req.Messages),
	}
	if req.System != "" {
		params.System = []anthropic.BetaTextBlockParam{{Text: req.System}}
	}
	if fallbackModels[model] {
		// On a safety decline the API reruns the request on a fallback model.
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: "default"}
	}

	stream := c.client.Beta.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	var out strings.Builder
	var stop string
	for stream.Next() {
		switch ev := stream.Current().AsAny().(type) {
		case anthropic.BetaRawContentBlockDeltaEvent:
			if d, ok := ev.Delta.AsAny().(anthropic.BetaTextDelta); ok {
				out.WriteString(d.Text)
				if onDelta != nil {
					onDelta(d.Text)
				}
			}
		case anthropic.BetaRawMessageDeltaEvent:
			stop = string(ev.Delta.StopReason)
		}
	}
	if err := stream.Err(); err != nil {
		return out.String(), err
	}
	if stop == "refusal" && out.Len() == 0 {
		return "", fmt.Errorf("the model declined to answer")
	}
	return out.String(), nil
}

// toBetaMessages converts history to API messages, merging consecutive
// turns of the same role and making sure the first turn is the user's.
func toBetaMessages(msgs []history.Message) []anthropic.BetaMessageParam {
	var out []anthropic.BetaMessageParam
	var role string
	var buf []string
	flush := func() {
		if len(buf) == 0 {
			return
		}
		block := anthropic.NewBetaTextBlock(strings.Join(buf, "\n\n"))
		if role == history.RoleAssistant {
			out = append(out, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{block}})
		} else {
			out = append(out, anthropic.NewBetaUserMessage(block))
		}
		buf = nil
	}
	for _, m := range msgs {
		r := m.Role
		if r != history.RoleAssistant {
			r = history.RoleUser
		}
		if len(out) == 0 && len(buf) == 0 && r == history.RoleAssistant {
			continue
		}
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		if r != role {
			flush()
			role = r
		}
		buf = append(buf, m.Content)
	}
	flush()
	return out
}
