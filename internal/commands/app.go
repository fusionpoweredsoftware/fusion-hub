package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fusionpoweredsoftware/fusion-hub/examples"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/actors"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/ail"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/chat"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
)

// appConfigKey stores the app selected for app-using-ai.
const appConfigKey = ".current-app-config"

// AppExt is the AIL app file extension.
const AppExt = ".ail.yaml"

type appConfig struct {
	App      string         `json:"app"`
	Settings map[string]any `json:"settings,omitempty"`
	Debug    bool           `json:"debug,omitempty"`
}

func loadAppConfig(env *Env) appConfig {
	var c appConfig
	if v, ok := env.Settings.Get(appConfigKey); ok {
		b, _ := json.Marshal(v)
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func saveAppConfig(env *Env, c appConfig) error {
	b, _ := json.Marshal(c)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	env.Settings.Set(appConfigKey, m)
	return env.Settings.Save()
}

// FindApp resolves an app name: a file path, ~/.fusion-hub/apps/<name>,
// or a built-in example, with or without the .ail.yaml extension.
func FindApp(paths config.Paths, name string) (*ail.Pipeline, error) {
	names := []string{name}
	if !strings.HasSuffix(name, AppExt) {
		names = append(names, name+AppExt)
	}
	for _, n := range names {
		for _, p := range []string{n, filepath.Join(paths.Root, config.Apps, n)} {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return ail.Load(p)
			}
		}
	}
	for _, n := range names {
		if b, err := examples.FS.ReadFile(filepath.Base(n)); err == nil {
			return ail.Parse(b)
		}
	}
	return nil, fmt.Errorf("app not found: %s (looked in the current directory, %s and the built-in examples)", name, filepath.Join(paths.Root, config.Apps))
}

// Runtime binds an AIL pipeline to this environment's chat session.
func (env *Env) Runtime(debug bool) *ail.Runtime {
	book, err := actors.Open(env.Paths.Dir(config.Actors))
	if err != nil {
		env.Out.Warn("actors: %v (using built-in profiles)", err)
		book = actors.Builtin()
	}
	rt := &ail.Runtime{
		Out:     env.Out,
		Speaker: env.Speaker,
		Actors:  book,
		Chat: func(ctx context.Context, prompt string, opts ail.ChatOptions) (string, error) {
			out := opts.Out
			if debug {
				env.Out.Print(output.Gray + "user> " + prompt + output.Reset)
				out = env.Out
			}
			return env.Chat.Send(ctx, env.SessionID, prompt, out, chat.SendOptions{Ephemeral: opts.Ephemeral})
		},
		HistoryLen: func() int { return env.Chat.Session(env.SessionID).Len() },
		Undo: func() error {
			s := env.Chat.Session(env.SessionID)
			return s.Truncate(s.Len() - 2)
		},
		Command: func(ctx context.Context, line string) (string, error) {
			return env.Registry.Dispatch(ctx, env, line)
		},
	}
	return rt
}

func appUsingAI() *Command {
	return &Command{
		ID:            "app-using-ai",
		Name:          "app using AI",
		Description:   "Run the selected AIL app on each line of input.",
		Examples:      "start new app-using-ai english2spanish",
		CaseSensitive: true,
		Start: map[string]StartFunc{
			"": func(ctx context.Context, env *Env, args []string) (string, error) {
				return startApp(ctx, env, args, false, false)
			},
			"new": func(ctx context.Context, env *Env, args []string) (string, error) {
				return startApp(ctx, env, args, true, false)
			},
			"debug": func(ctx context.Context, env *Env, args []string) (string, error) {
				return startApp(ctx, env, args, false, true)
			},
			"old": startOldApp,
		},
		Run: func(ctx context.Context, env *Env, in Input) (string, error) {
			c := loadAppConfig(env)
			if c.App == "" {
				return "", errors.New("No app selected. Start one with: start new app-using-ai <app>")
			}
			p, err := FindApp(env.Paths, c.App)
			if err != nil {
				return "", err
			}
			// Steps may run "start ..." commands; the app keeps its mode.
			mode := env.State.Mode()
			defer env.State.SetMode(mode)
			_, err = p.Run(ctx, env.Runtime(c.Debug), in.Raw, c.Settings)
			return "", err
		},
	}
}

// startApp handles "start [new|debug] app-using-ai [app] [settings-json]".
func startApp(ctx context.Context, env *Env, args []string, isNew, debug bool) (string, error) {
	c := loadAppConfig(env)
	if len(args) > 2 {
		return "", errors.New("Command limited to 2 arguments: the app and its JSON settings.")
	}
	if len(args) >= 1 {
		if c.App != args[0] {
			c.Settings = nil
		}
		c.App = args[0]
	}
	if len(args) == 2 {
		m := map[string]any{}
		if err := json.Unmarshal([]byte(args[1]), &m); err != nil {
			return "", fmt.Errorf("app settings must be JSON: %w", err)
		}
		if c.Settings == nil || isNew {
			c.Settings = map[string]any{}
		}
		for k, v := range m {
			c.Settings[k] = v
		}
	}
	if c.App == "" {
		return "", errors.New("No app selected. Usage: start new app-using-ai <app> ['{\"key\": \"value\"}']")
	}
	p, err := FindApp(env.Paths, c.App)
	if err != nil {
		return "", err
	}
	c.Debug = debug
	if isNew {
		if _, err := newChat(ctx, env, ""); err != nil {
			return "", err
		}
	}
	if err := saveAppConfig(env, c); err != nil {
		return "", err
	}
	name := p.Name
	if name == "" {
		name = c.App
	}
	return "app " + name, nil
}

// startOldApp handles "start old app-using-ai <conversation> [app] [settings]".
func startOldApp(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("Command takes at least 1 argument: the conversation file.")
	}
	if _, err := startOldChat(ctx, env, args[:1]); err != nil {
		return "", err
	}
	return startApp(ctx, env, args[1:], false, false)
}

func multiCommand() *Command {
	return &Command{
		ID:            "multi-command",
		Name:          "multi-command",
		Description:   "Run an AIL app once with the given input, in the current chat.",
		Examples:      "multi-command english2spanish Good morning!",
		CaseSensitive: true,
		Run: func(ctx context.Context, env *Env, in Input) (string, error) {
			if len(in.Args) == 0 {
				return "", errors.New("usage: multi-command <app> [input]")
			}
			p, err := FindApp(env.Paths, in.Args[0])
			if err != nil {
				return "", err
			}
			_, err = p.Run(ctx, env.Runtime(false), RestAfter(in.Raw, 1), nil)
			return "", err
		},
	}
}
