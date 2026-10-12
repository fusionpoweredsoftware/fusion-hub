package commands

import (
	"context"
	"errors"
	"strings"
)

func say() *Command {
	return &Command{
		ID:            "say",
		Name:          "say",
		Description:   "Convert text to speech. \"sync:\" waits for playback; \"(male-voice)\" picks a male voice.",
		Examples:      "say sync: (male-voice) hello there world!",
		CaseSensitive: true,
		Run: func(ctx context.Context, env *Env, in Input) (string, error) {
			text, sync, male := ParseSay(in.Raw)
			if text == "" {
				return "", errors.New("Nothing to say.")
			}
			if env.Speaker == nil {
				return "", errors.New("text-to-speech is not available")
			}
			return "", env.Speaker.Say(ctx, text, male, sync)
		},
	}
}

// ParseSay strips the "sync: " and "(male-voice) " prefixes.
func ParseSay(raw string) (text string, sync, male bool) {
	text = strings.TrimSpace(raw)
	if rest, ok := cutPrefixFold(text, "sync:"); ok {
		sync, text = true, strings.TrimSpace(rest)
	}
	if rest, ok := cutPrefixFold(text, "(male-voice)"); ok {
		male, text = true, strings.TrimSpace(rest)
	}
	return text, sync, male
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}
