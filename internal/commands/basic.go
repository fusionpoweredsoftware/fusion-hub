package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/provider"
)

func help() *Command {
	return &Command{
		ID:          "help",
		Name:        "help",
		Description: "List the names, descriptions and examples of commands.",
		Examples:    "help [command]",
		Run: func(_ context.Context, env *Env, in Input) (string, error) {
			var cmds []*Command
			for _, c := range env.Registry.All() {
				if len(in.Args) == 0 || c.ID == in.Args[0] {
					cmds = append(cmds, c)
				}
			}
			if len(cmds) == 0 {
				return "", ErrNotFound
			}
			w := [3]int{len("COMMAND"), len("DESCRIPTION"), len("EXAMPLES")}
			for _, c := range cmds {
				w[0] = max(w[0], len(c.ID))
				w[1] = max(w[1], len(c.Description))
			}
			row := func(a, b, c string) string {
				return fmt.Sprintf("%-*s  |  %-*s  |  %s", w[0], a, w[1], b, c)
			}
			env.Out.Print("")
			env.Out.Print(row("COMMAND", "DESCRIPTION", "EXAMPLES"))
			env.Out.Print(strings.Repeat("-", w[0]+2) + "|" + strings.Repeat("-", w[1]+4) + "|" + strings.Repeat("-", 30))
			for _, c := range cmds {
				env.Out.Print(row(c.ID, c.Description, c.Examples))
			}
			env.Out.Print("")
			env.Out.Print("Ctrl+C switches between CLI mode and the started command's mode; press it twice quickly to exit.")
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
		Description:   "Set variables for commands to use (provider, model, API keys...), or list them.",
		Examples:      "set (<key> as <val>|json(+)? <json>|nill <key>)",
		CaseSensitive: true,
		Run:           runSet,
	}
}

func runSet(_ context.Context, env *Env, in Input) (string, error) {
	s := env.Settings
	switch {
	case len(in.Args) == 0:
		for _, k := range s.Keys() {
			env.Out.Print(strings.ToUpper(k) + " AS " + mask(k, s.String(k)))
		}
		if len(s.Keys()) == 0 {
			env.Out.Print("Nothing set. Try: set provider as claude")
		}
		return "", nil
	case in.Args[0] == "json" && len(in.Args) == 1:
		b, _ := json.MarshalIndent(s.Snapshot(), "", "  ")
		env.Out.Print("JSON:\n\n" + string(b))
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
			return "", errors.New("No variable specified.")
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

func mask(key, val string) string {
	if !strings.HasSuffix(key, "api-key") || len(val) <= 8 {
		if strings.HasSuffix(key, "api-key") {
			return "****"
		}
		return val
	}
	return "****" + val[len(val)-4:]
}

func start() *Command {
	return &Command{
		ID:            "start",
		Name:          "start",
		Description:   "Start a session with a startable command (chat-with-ai, app-using-ai) and make it the command mode.",
		Examples:      "start (new|old|last|recap|...)? chat-with-ai",
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
		var types []string
		for t := range target.Start {
			if t != "" {
				types = append(types, t)
			}
		}
		sort.Strings(types)
		return "", fmt.Errorf("Unable to process command: Not a valid start type. %s accepts: %s", target.ID, strings.Join(types, ", "))
	}
	msg, err := fn(ctx, env, rest)
	if err != nil {
		return "", err
	}
	env.State.SetMode(target.ID)
	if msg == "" {
		msg = target.Name
		if startType != "" {
			msg = startType + " " + msg
		}
	}
	msg = strings.ReplaceAll(msg, "-", " ")
	env.Out.Log("%s started.", strings.ToUpper(msg[:1])+msg[1:])
	return "", nil
}
