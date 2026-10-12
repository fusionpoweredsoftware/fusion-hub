package ail

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var fence = regexp.MustCompile("(?s)```[ \\t]*(?:json5?|JSON5?)?[ \\t]*\\n(.*?)```")
var trailingComma = regexp.MustCompile(`,(\s*[}\]])`)

// ExtractJSON finds the JSON value in a model reply: a fenced ```json
// block if there is one, otherwise the first balanced {...} or [...].
// Trailing commas are tolerated.
func ExtractJSON(reply string) (any, error) {
	var candidates []string
	for _, m := range fence.FindAllStringSubmatch(reply, -1) {
		candidates = append(candidates, m[1])
	}
	if c := balanced(reply); c != "" {
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return nil, errors.New("no JSON object or array found in the response")
	}
	var firstErr error
	for _, c := range candidates {
		var v any
		err := json.Unmarshal([]byte(trailingComma.ReplaceAllString(strings.TrimSpace(c), "$1")), &v)
		if err == nil {
			return v, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

// balanced returns the first top-level {...} or [...] in s, skipping
// brackets inside strings.
func balanced(s string) string {
	start := strings.IndexAny(s, "{[")
	for start >= 0 {
		depth, inStr, esc := 0, false, false
		for i := start; i < len(s); i++ {
			ch := s[i]
			switch {
			case esc:
				esc = false
			case inStr && ch == '\\':
				esc = true
			case ch == '"':
				inStr = !inStr
			case inStr:
			case ch == '{' || ch == '[':
				depth++
			case ch == '}' || ch == ']':
				depth--
				if depth == 0 {
					return s[start : i+1]
				}
			}
		}
		next := strings.IndexAny(s[start+1:], "{[")
		if next < 0 {
			return ""
		}
		start += next + 1
	}
	return ""
}

// missing lists required keys that are absent, null or empty strings.
func missing(v any, keys []string) []string {
	var out []string
	vars := Vars{}
	if m, ok := v.(map[string]any); ok {
		vars = Vars(m)
	}
	for _, k := range keys {
		if val, ok := vars.Lookup(k); !ok || val == nil || val == "" {
			out = append(out, k)
		}
	}
	return out
}
