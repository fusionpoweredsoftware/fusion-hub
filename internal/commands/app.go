package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	fusionhub "github.com/fusionpoweredsoftware/fusion-hub"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/actors"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/ail"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/ailjs"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/chat"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/history"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
)

// app-using-ai and multi-command run AIL apps. An app is a .app.ail.js file
// (the original format, run unchanged by internal/ailjs) or a .ail.yaml
// pipeline (internal/ail). Apps are looked up the way the original did:
// a bare name lives in <root>/multi-command, ./x is relative to the working
// directory; the examples shipped in the binary are copied there on first
// use so they can be read and edited.

// appConfigKey is the original's setup.settings[".current-app-config"]:
// one entry per session id ({appFilePath, appSettings, isDebugging}) plus
// startedSessionId and sessionId.
const appConfigKey = ".current-app-config"

// AppExt is appended to an app name, as the original always did.
const AppExt = ".app.ail.js"

// YAMLExt marks a declarative app.
const YAMLExt = ".ail.yaml"

// MultiCommandDir is the context directory bare app names resolve in.
const MultiCommandDir = "multi-command"

type sessionAppConfig struct {
	AppFilePath string         `json:"appFilePath"`
	AppSettings map[string]any `json:"appSettings"`
	IsDebugging bool           `json:"isDebugging"`
}

func loadAppConfigs(env *Env) map[string]any {
	if v, ok := env.Settings.Get(appConfigKey); ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return map[string]any{}
}

func sessionConfig(cfgs map[string]any, sid string) sessionAppConfig {
	var c sessionAppConfig
	if v, ok := cfgs[sid]; ok {
		b, _ := json.Marshal(v)
		_ = json.Unmarshal(b, &c)
	}
	if c.AppSettings == nil {
		c.AppSettings = map[string]any{}
	}
	return c
}

func saveAppConfigs(env *Env, cfgs map[string]any) error {
	env.Settings.Set(appConfigKey, cfgs)
	return env.Settings.Save()
}

func setSessionConfig(cfgs map[string]any, sid string, c sessionAppConfig) {
	var m map[string]any
	b, _ := json.Marshal(c)
	_ = json.Unmarshal(b, &m)
	if c.AppFilePath == "" {
		m["appFilePath"] = nil
	}
	cfgs[sid] = m
}

// sessionKey is the original sessionId as a config key ("0" by default).
func sessionKey(v any) string {
	switch c := v.(type) {
	case nil:
		return "0"
	case string:
		if c == "" {
			return "0"
		}
		return c
	case float64:
		return fmt.Sprintf("%v", c)
	}
	return fmt.Sprint(v)
}

// chatID maps the app session id to the chat session ("" is the default).
func chatID(sid string) string {
	if sid == "0" {
		return ""
	}
	return sid
}

var (
	enginesMu sync.Mutex
	engines   = map[*Env]*ailjs.Engine{}
	running   = map[string]bool{}
)

// appEngine is the JS runtime for this environment, created on first use.
func appEngine(env *Env) (*ailjs.Engine, error) {
	enginesMu.Lock()
	defer enginesMu.Unlock()
	if e, ok := engines[env]; ok {
		return e, nil
	}
	e, err := ailjs.New(&appHost{env: env}, fusionhub.Legacy)
	if err != nil {
		return nil, err
	}
	engines[env] = e
	return e, nil
}

// appHost connects the JS runtime to this environment.
type appHost struct{ env *Env }

func (h *appHost) Out() *output.Writer      { return h.env.Out }
func (h *appHost) Paths() config.Paths      { return h.env.Paths }
func (h *appHost) Settings() map[string]any { return h.env.Settings.Snapshot() }
func (h *appHost) Mode() string             { return h.env.State.Mode() }
func (h *appHost) SetMode(m string)         { h.env.State.SetMode(m) }

func (h *appHost) StartChat(ctx context.Context, sid string, isNew bool) error {
	if isNew {
		return h.env.Chat.New(ctx, sid, "")
	}
	return h.env.Chat.Start(ctx, sid)
}

func (h *appHost) History(sid string) []ailjs.Message {
	conv := h.env.Chat.Session(sid).Snapshot()
	out := make([]ailjs.Message, len(conv.Messages))
	for i, m := range conv.Messages {
		role := m.Role
		if role == history.RoleAssistant {
			role = "model"
		}
		out[i] = ailjs.Message{Role: role, Text: m.Content}
	}
	return out
}

func toHistory(msgs []ailjs.Message) []history.Message {
	out := make([]history.Message, len(msgs))
	for i, m := range msgs {
		role := m.Role
		if role == "model" {
			role = history.RoleAssistant
		}
		out[i] = history.Message{Role: role, Content: m.Text}
	}
	return out
}

func (h *appHost) Chat(ctx context.Context, _ string, hist []ailjs.Message, text string, stream io.Writer) (string, error) {
	return h.env.Chat.Complete(ctx, toHistory(hist), text, stream)
}

func (h *appHost) Persist(sid string, channels map[string][]ailjs.Message) error {
	var extra map[string]json.RawMessage
	for k, ch := range channels {
		if k == "0" {
			continue
		}
		turns := make([]map[string]any, len(ch))
		for i, m := range ch {
			turns[i] = map[string]any{"role": m.Role, "parts": []map[string]any{{"text": m.Text}}}
		}
		b, err := json.Marshal(turns)
		if err != nil {
			return err
		}
		if extra == nil {
			extra = map[string]json.RawMessage{}
		}
		extra[k] = b
	}
	return h.env.Chat.Replace(sid, toHistory(channels["0"]), extra)
}

// Execute runs a block that is not a chat-with-ai message. A plain
// "start <command>" from an app is silent unless logStart is set (the
// original's isCommandStartLogged); typed starts and every other command go
// through the normal dispatch.
func (h *appHost) Execute(ctx context.Context, line string, logStart bool) error {
	f := strings.Fields(line)
	if len(f) == 2 && strings.EqualFold(f[0], "start") && !logStart {
		if c := h.env.Registry.Get(f[1]); c != nil && c.Startable() {
			if _, err := c.Start[""](ctx, h.env, nil); err != nil {
				if errors.Is(err, ErrNotStarted) {
					return nil
				}
				return err
			}
			h.env.State.SetMode(c.ID)
			return nil
		}
	}
	_, err := h.env.Registry.Dispatch(ctx, h.env, line)
	return err
}

func appUsingAI() *Command {
	return &Command{
		ID:            "app-using-ai",
		Name:          "app using AI",
		Description:   "Send a message to the AI, filtered. Returns the AI's response.",
		Examples:      "app-using-ai myFirst.app.ail.js",
		CaseSensitive: true,
		Start: map[string]StartFunc{
			"":      startApp,
			"debug": startAppDebug,
			"new":   startAppNew,
			"old":   startAppOld,
		},
		Run: runApp,
	}
}

// parseAppSettings reads the JSON5 settings argument.
func parseAppSettings(env *Env, s string) (map[string]any, error) {
	if s == "" {
		return map[string]any{}, nil
	}
	e, err := appEngine(env)
	if err != nil {
		return nil, err
	}
	v, err := e.JSON5(s)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("app settings must be a JSON object")
	}
	return m, nil
}

func appFileName(name string) string {
	if name == "" || strings.HasSuffix(name, YAMLExt) {
		return name
	}
	return name + AppExt
}

// startApp is "start app-using-ai [app] [settings]": the saved entry is
// updated (saved settings win over the typed ones, as Object.assign did)
// and the chat is resumed.
func startApp(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) > 2 {
		return "", errors.New("Command limited to 3 arguments.")
	}
	return startAppWith(ctx, env, args, false)
}

// startAppOld is "start old app-using-ai <conversation> [app] [settings]".
func startAppOld(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) == 0 {
		return "", errOneArgument
	}
	if len(args) > 3 {
		return "", errors.New("Command limited to 3 arguments.")
	}
	return startAppWith(ctx, env, args[1:], true, args[0])
}

func startAppWith(ctx context.Context, env *Env, args []string, old bool, oldFile ...string) (string, error) {
	cfgs := loadAppConfigs(env)
	settings := map[string]any{}
	if len(args) > 1 {
		var err error
		if settings, err = parseAppSettings(env, args[1]); err != nil {
			return "", err
		}
	}
	sid := sessionKey(cfgs["startedSessionId"])
	if v, ok := settings["sessionId"]; ok {
		sid = sessionKey(v)
	}
	entry := sessionConfig(cfgs, sid)
	if len(args) > 0 && args[0] != "" {
		entry.AppFilePath = appFileName(args[0])
	}
	for k, v := range entry.AppSettings {
		settings[k] = v
	}
	entry.AppSettings = settings
	setSessionConfig(cfgs, sid, entry)
	if err := saveAppConfigs(env, cfgs); err != nil {
		return "", err
	}
	var err error
	if old {
		err = env.Chat.Old(ctx, chatID(sid), oldFile[0])
	} else {
		err = env.Chat.Start(ctx, chatID(sid))
	}
	if err != nil {
		return "", err
	}
	cfgs["sessionId"] = sid
	if err := saveAppConfigs(env, cfgs); err != nil {
		return "", err
	}
	forgetHistory(env, chatID(sid))
	return "", nil
}

// startAppDebug is "start debug app-using-ai [app] [settings]": typed
// settings replace the saved ones and the app logs its generated module and
// streams the model's replies until a "start new".
func startAppDebug(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) > 2 {
		return "", errors.New("Command takes no arguments.")
	}
	cfgs := loadAppConfigs(env)
	var settings map[string]any
	if len(args) > 1 {
		var err error
		if settings, err = parseAppSettings(env, args[1]); err != nil {
			return "", err
		}
	}
	sid := sessionKey(cfgs["startedSessionId"])
	if settings != nil {
		if v, ok := settings["sessionId"]; ok {
			sid = sessionKey(v)
		}
	}
	entry := sessionConfig(cfgs, sid)
	if len(args) > 0 && args[0] != "" {
		entry.AppFilePath = appFileName(args[0])
	}
	if settings != nil {
		entry.AppSettings = settings
	}
	entry.IsDebugging = true
	setSessionConfig(cfgs, sid, entry)
	if err := saveAppConfigs(env, cfgs); err != nil {
		return "", err
	}
	if err := env.Chat.Start(ctx, chatID(sid)); err != nil {
		return "", err
	}
	forgetHistory(env, chatID(sid))
	return "", nil
}

// startAppNew is "start new app-using-ai [app] [settings] [conversation]":
// a fresh entry and a new chat, saved later under conversation when given.
func startAppNew(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) > 3 {
		return "", errors.New("Command limited to 3 arguments.")
	}
	cfgs := loadAppConfigs(env)
	settings := map[string]any{}
	if len(args) > 1 {
		var err error
		if settings, err = parseAppSettings(env, args[1]); err != nil {
			return "", err
		}
	}
	sid := "0"
	if v, ok := settings["sessionId"]; ok {
		sid = sessionKey(v)
	}
	entry := sessionAppConfig{AppSettings: settings}
	if len(args) > 0 && args[0] != "" {
		entry.AppFilePath = appFileName(args[0])
	}
	if v, ok := settings["isDebugging"].(bool); ok {
		entry.IsDebugging = v
	}
	setSessionConfig(cfgs, sid, entry)
	if err := saveAppConfigs(env, cfgs); err != nil {
		return "", err
	}
	name := ""
	if len(args) > 2 {
		name = args[2]
	}
	if err := env.Chat.New(ctx, chatID(sid), name); err != nil {
		return "", err
	}
	cfgs["startedSessionId"] = sid
	if err := saveAppConfigs(env, cfgs); err != nil {
		return "", err
	}
	forgetHistory(env, chatID(sid))
	return "", nil
}

func forgetHistory(env *Env, id string) {
	enginesMu.Lock()
	e := engines[env]
	enginesMu.Unlock()
	if e != nil {
		e.ForgetHistory(id)
	}
}

// runApp is the app-using-ai command itself: run the selected app on the
// line. As in the original, the blocks run in CLI mode, the REPL is left in
// app-using-ai mode afterwards, and an error ends the run with a logged
// message rather than failing the command.
func runApp(ctx context.Context, env *Env, in Input) (string, error) {
	if err := runSelectedApp(ctx, env, in); err != nil {
		env.Out.Error("%v", err)
	}
	return "", nil
}

func runSelectedApp(ctx context.Context, env *Env, in Input) error {
	cfgs := loadAppConfigs(env)
	sid := sessionKey(cfgs["startedSessionId"])
	enginesMu.Lock()
	if running[sid] {
		enginesMu.Unlock()
		env.Out.Log("App already running.")
		return nil
	}
	running[sid] = true
	enginesMu.Unlock()
	defer func() {
		enginesMu.Lock()
		delete(running, sid)
		enginesMu.Unlock()
	}()
	entry := sessionConfig(cfgs, sid)
	env.State.SetMode("")
	defer env.State.SetMode("app-using-ai")
	if entry.AppFilePath == "" {
		return errors.New("No app selected. Usage: start new app-using-ai <app> ['{\"key\": \"value\"}']")
	}
	if strings.HasSuffix(entry.AppFilePath, YAMLExt) {
		_, err := runYAML(ctx, env, entry.AppFilePath, in.Raw, entry.AppSettings, entry.IsDebugging)
		return err
	}
	e, err := appEngine(env)
	if err != nil {
		return err
	}
	if err := installExample(env, entry.AppFilePath); err != nil {
		return err
	}
	return e.RunApp(ctx, entry.AppFilePath, in.Raw, entry.AppSettings, chatID(sid), entry.IsDebugging)
}

// installExample copies an example shipped in the binary to where the app
// name resolves (<root>/multi-command for bare names) when nothing is there
// yet, so "start new app-using-ai actor" works out of the box and the user
// gets an editable copy.
func installExample(env *Env, appFilePath string) error {
	dest := ailjs.ExpandFilePath(env.Paths, appFilePath, MultiCommandDir)
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	rel := filepath.ToSlash(filepath.Clean(appFilePath))
	for _, candidate := range []string{rel, "example/" + filepath.Base(rel), "example/just-testing/" + filepath.Base(rel)} {
		b, err := fusionhub.Legacy.ReadFile(candidate)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, b, 0o644); err != nil {
			return err
		}
		env.Out.Log("Copied the built-in example to: %s", dest)
		return nil
	}
	return nil
}

func multiCommand() *Command {
	return &Command{
		ID:            "multi-command",
		Name:          "multi-command",
		Description:   "Run multiple commands from a file.",
		Examples:      "multi-command <file.txt> [parameters]",
		CaseSensitive: true,
		Run: func(ctx context.Context, env *Env, in Input) (string, error) {
			e, err := appEngine(env)
			if err != nil {
				return "", err
			}
			args, err := e.Args(in.Raw)
			if err != nil {
				return "", err
			}
			if len(args) == 0 || args[0] == "" {
				env.Out.Log("multi-command <file.txt> [parameters]")
				return "", nil
			}
			if strings.HasSuffix(args[0], YAMLExt) {
				return runYAML(ctx, env, args[0], RestAfter(in.Raw, 1), nil, false)
			}
			if err := installExample(env, args[0]); err != nil {
				return "", err
			}
			return "", e.MultiCommand(ctx, args, strings.TrimSpace(in.Raw))
		},
	}
}

// --- declarative (.ail.yaml) apps ---

// FindYAML resolves a .ail.yaml app: a path, or a file in <root>/apps.
func FindYAML(paths config.Paths, name string) (*ail.Pipeline, error) {
	for _, p := range []string{name, filepath.Join(paths.Root, config.Apps, name)} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return ail.Load(p)
		}
	}
	return nil, fmt.Errorf("app not found: %s (looked in the current directory and %s)", name, filepath.Join(paths.Root, config.Apps))
}

func runYAML(ctx context.Context, env *Env, name, input string, settings map[string]any, debug bool) (string, error) {
	p, err := FindYAML(env.Paths, name)
	if err != nil {
		return "", err
	}
	_, err = p.Run(ctx, env.Runtime(debug), input, settings)
	return "", err
}

// Runtime binds a YAML pipeline to this environment's chat session.
func (env *Env) Runtime(debug bool) *ail.Runtime {
	book, err := actors.Open(env.Paths.Dir(config.Actors))
	if err != nil {
		env.Out.Warn("actors: %v (using built-in profiles)", err)
		book = actors.Builtin()
	}
	return &ail.Runtime{
		Out:     env.Out,
		Speaker: env.Speaker,
		Actors:  book,
		Chat: func(ctx context.Context, prompt string, opts ail.ChatOptions) (string, error) {
			out := opts.Out
			if debug {
				env.Out.Print(output.FgGray + "user> " + prompt + output.Reset)
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
}
