package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// openAI speaks the OpenAI chat-completions protocol, which OpenAI, z.ai,
// Ollama, OpenRouter, Groq, DeepSeek and xAI all serve.
type openAI struct {
	cfg  Config
	http *http.Client
}

func newOpenAI(cfg Config) *openAI { return &openAI{cfg: cfg, http: http.DefaultClient} }

func (o *openAI) Name() string { return o.cfg.Provider }

type oaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaRequest struct {
	Model       string      `json:"model"`
	Messages    []oaMessage `json:"messages"`
	Stream      bool        `json:"stream"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature float64     `json:"temperature"`
	TopP        float64     `json:"top_p"`
}

type oaChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (o *openAI) Stream(ctx context.Context, req Request, onDelta func(string)) (string, error) {
	body := oaRequest{Model: req.Model, Stream: true, MaxTokens: req.MaxTokens, Temperature: o.cfg.Temperature, TopP: o.cfg.TopP}
	if body.Model == "" {
		body.Model = o.cfg.Model
	}
	if body.MaxTokens == 0 {
		body.MaxTokens = o.cfg.MaxTokens
	}
	if body.MaxTokens == 0 {
		body.MaxTokens = DefaultMaxTokens
	}
	if req.System != "" {
		body.Messages = append(body.Messages, oaMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		body.Messages = append(body.Messages, oaMessage{Role: m.Role, Content: m.Content})
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(o.cfg.BaseURL, "/") + "/chat/completions"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "text/event-stream")
	if o.cfg.APIKey != "" {
		hreq.Header.Set("Authorization", "Bearer "+o.cfg.APIKey)
	}
	resp, err := o.http.Do(hreq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", &HTTPError{Status: resp.StatusCode, Msg: fmt.Sprintf("%s: %s: %s", o.cfg.Provider, resp.Status, strings.TrimSpace(string(msg)))}
	}

	var out strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk oaChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return out.String(), fmt.Errorf("%s: bad stream chunk: %w", o.cfg.Provider, err)
		}
		if chunk.Error != nil {
			return out.String(), fmt.Errorf("%s: %s", o.cfg.Provider, chunk.Error.Message)
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				out.WriteString(c.Delta.Content)
				if onDelta != nil {
					onDelta(c.Delta.Content)
				}
			}
		}
	}
	return out.String(), sc.Err()
}
