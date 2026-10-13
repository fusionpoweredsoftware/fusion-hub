package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/provider"
)

// The help table: fixed column widths and separators.
var (
	helpCols = [3]int{13, 75, 34}
	helpSep  = "  |  "
	helpRule = "--|--"
)

// helpRow pads the three cells to the table's column widths.
func helpRow(id, description, examples string) string {
	return fmt.Sprintf("%-*s%s%-*s%s%-*s", helpCols[0], id, helpSep, helpCols[1], description, helpSep, helpCols[2], examples)
}

func help() *Command {
	return &Command{
		ID:          "help",
		Name:        "help",
		Description: "List the names, descriptions, examples, and input attributes of commands.",
		Examples:    "help",
		Run: func(_ context.Context, env *Env, in Input) (string, error) {
			found := false
			for _, c := range env.Registry.All() {
				if len(in.Args) > 0 && c.ID != in.Args[0] {
					continue
				}
				if !found {
					env.Out.Print("")
					env.Out.Print(helpRow("COMMAND", "DESCRIPTION", "EXAMPLES"))
					env.Out.Print(strings.Repeat("-", helpCols[0]) + helpRule + strings.Repeat("-", helpCols[1]) + helpRule + strings.Repeat("-", helpCols[2]))
					env.Out.Print(helpRow("", "", ""))
				}
				env.Out.Print(helpRow(c.ID, c.Description, c.Examples))
				env.Out.Print(helpRow("", "", ""))
				found = true
			}
			if !found {
				env.Out.Print("Command not found.")
			}
			return "", nil
		},
	}
}

func exit() *Command {
	return &Command{
		ID:          "exit",
		Name:        "exit",
		Description: "Exit the application.",
		Examples:    "exit",
		Run: func(_ context.Context, env *Env, _ Input) (string, error) {
			env.State.Stop()
			return "", nil
		},
	}
}

func set() *Command {
	return &Command{
		ID:            "set",
		Name:          "set",
		Description:   "Set a variables for commands to use, or see what is currently set.",
		Examples:      "set (<key> as <val>|json(+)? <json>|nill <key>)",
		CaseSensitive: true,
		Run:           runSet,
	}
}

func runSet(_ context.Context, env *Env, in Input) (string, error) {
	s := env.Settings
	switch {
	case len(in.Args) == 0:
		env.Out.Print("<KEY> AS <VALUE>:\n\n" + settingsListing(s))
		return "", nil
	case in.Args[0] == "json" && len(in.Args) == 1:
		env.Out.Print("JSON:\n\n" + compactJSON(s.Snapshot()) + "\n")
		return "", nil
	case in.Args[0] == "json" || in.Args[0] == "json+":
		m := map[string]any{}
		if err := json.Unmarshal([]byte(RestAfter(in.Raw, 1)), &m); err != nil {
			return "", fmt.Errorf("invalid JSON: %w", err)
		}
		if in.Args[0] == "json" {
			s.Replace(m)
		} else {
			s.Merge(m)
		}
		return "", s.Save()
	case in.Args[0] == "nill":
		if len(in.Args) < 2 {
			env.Out.Print("No variable specified.")
			return "", nil
		}
		s.Delete(in.Args[1])
		env.Out.Print(in.Args[1] + " deleted.")
		return "", s.Save()
	}

	pairs, err := parsePairs(in.Args)
	if err != nil {
		return "", err
	}
	for _, kv := range pairs {
		key, val := kv[0], kv[1]
		if key == "provider" {
			name, err := provider.Canonical(val)
			if err != nil {
				return "", err
			}
			val = name
			p := provider.Presets[name]
			env.Out.Print(fmt.Sprintf("Provider set to %s (default model %s).", name, p.DefaultModel))
		}
		s.Set(key, val)
	}
	return "", s.Save()
}

// settingsListing renders the public settings as "key AS value" lines the
// way the original derived them from the settings' JSON: the braces are
// dropped, `":"` becomes ` AS `, commas become newlines and the remaining
// quotes are removed (so non-string values appear as JSON). Keys are in
// sorted order and API keys are masked.
func settingsListing(s *config.Store) string {
	var b strings.Builder
	b.WriteString("{")
	for i, k := range s.Keys() {
		if i > 0 {
			b.WriteString(",")
		}
		v, _ := s.Get(k)
		if strings.HasSuffix(k, "api-key") {
			v = mask(k, s.String(k))
		}
		b.WriteString(compactJSON(k) + ":" + compactJSON(v))
	}
	b.WriteString("}")
	text := strings.TrimSuffix(strings.TrimPrefix(b.String(), "{"), "}")
	text = strings.ReplaceAll(text, `":"`, " AS ")
	text = strings.ReplaceAll(text, ",", "\n")
	return strings.ReplaceAll(text, `"`, "")
}

// compactJSON is JSON.stringify: compact and without HTML escaping.
func compactJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "null"
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// parsePairs reads "k1 as v1 k2 as multi word v2" into pairs.
func parsePairs(args []string) ([][2]string, error) {
	var asAt []int
	for i, a := range args {
		if strings.EqualFold(a, "as") && i > 0 {
			asAt = append(asAt, i)
		}
	}
	if len(asAt) == 0 {
		return nil, errors.New("usage: set <key> as <value>")
	}
	var out [][2]string
	for n, i := range asAt {
		end := len(args)
		if n+1 < len(asAt) {
			end = asAt[n+1] - 1
		}
		if i+1 > end {
			return nil, fmt.Errorf("no value given for %s", args[i-1])
		}
		out = append(out, [2]string{args[i-1], strings.Join(args[i+1:end], " ")})
	}
	if asAt[0] != 1 {
		return nil, errors.New("usage: set <key> as <value>")
	}
	return out, nil
}

// mask hides an API key, keeping its last four characters when it is long
// enough for that to give nothing away.
func mask(key, val string) string {
	if !strings.HasSuffix(key, "api-key") {
		return val
	}
	if len(val) <= 8 {
		return "****"
	}
	return "****" + val[len(val)-4:]
}

func start() *Command {
	return &Command{
		ID:            "start",
		Name:          "start",
		Description:   "Start a session with startable commands like chat-with-ai.",
		Examples:      "start (new|old|last)? chat-with-ai",
		CaseSensitive: true,
		Run:           runStart,
	}
}

func runStart(ctx context.Context, env *Env, in Input) (string, error) {
	args := in.Args
	if len(args) == 0 {
		return "", errors.New("Unable to process command: No command supplied.")
	}
	var target *Command
	var startType string
	var rest []string
	if c := env.Registry.Get(args[0]); c != nil && c.Startable() {
		target, rest = c, args[1:]
	} else if len(args) > 1 {
		if c := env.Registry.Get(args[1]); c != nil && c.Startable() {
			target, startType, rest = c, strings.ToLower(args[0]), args[2:]
		}
	}
	if target == nil {
		return "", errors.New("Unable to process command: Not a valid command mode to start.")
	}
	fn, ok := target.Start[startType]
	if !ok {
		return "", errors.New("Unable to process command: Not a valid start type.")
	}
	result, err := fn(ctx, env, rest)
	if errors.Is(err, ErrNotStarted) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	env.State.SetMode(target.ID)
	var msg string
	switch {
	case result != "":
		msg = startedName(result)
	case startType != "":
		msg = startedName(startType) + " " + target.Name
	default:
		msg = startedName(target.Name)
	}
	env.Out.Log("%s started.", msg)
	return "", nil
}

// startedName capitalises the first letter and turns dashes into spaces.
func startedName(s string) string {
	if s == "" {
		return s
	}
	return strings.ReplaceAll(strings.ToUpper(s[:1])+s[1:], "-", " ")
}
