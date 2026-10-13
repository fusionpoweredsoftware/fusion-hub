package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/chat"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/provider"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/tts"
)

type harness struct {
	env *Env
	reg *Registry
	out *bytes.Buffer
	sp  *provider.Scripted
	rec *tts.Recorder
}

func newHarness(t *testing.T, replies ...string) *harness {
	t.Helper()
	paths := config.Paths{Root: t.TempDir()}
	settings, err := config.Load(paths.SettingsFile())
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	sp := &provider.Scripted{Replies: replies}
	m := chat.NewManager(paths, settings)
	m.Provider = func() (provider.Provider, error) { return sp, nil }
	w := output.New(out)
	m.Console = w
	rec := &tts.Recorder{}
	reg := NewRegistry()
	return &harness{
		env: &Env{Out: w, Settings: settings, Paths: paths, Chat: m, Speaker: rec, State: &State{}, Registry: reg},
		reg: reg, out: out, sp: sp, rec: rec,
	}
}

func (h *harness) run(t *testing.T, line string) string {
	t.Helper()
	h.out.Reset()
	if _, err := h.reg.Execute(context.Background(), h.env, line); err != nil {
		t.Fatalf("%q: %v", line, err)
	}
	return h.out.String()
}

func TestArgs(t *testing.T) {
	got := Args(`start new  app-using-ai actor '{"id": "EINSTEIN"}' 'it\'s'`)
	want := []string{"start", "new", "app-using-ai", "actor", `{"id": "EINSTEIN"}`, "it's"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Args = %q", got)
	}
	if RestAfter("multi-command app  hello  world", 1) != "app  hello  world" {
		t.Error("RestAfter")
	}
}

func TestSet(t *testing.T) {
	h := newHarness(t)
	h.run(t, "set model as claude-sonnet-5-5 instructions as Be very brief.")
	if h.env.Settings.String("model") != "claude-sonnet-5-5" || h.env.Settings.String("instructions") != "Be very brief." {
		t.Fatalf("settings %#v", h.env.Settings.Snapshot())
	}
	if out := h.run(t, "set provider as Z.AI"); !strings.Contains(out, "Provider set to zai") || h.env.Settings.String("provider") != "zai" {
		t.Errorf("provider: %q", out)
	}
	if _, err := h.reg.Execute(context.Background(), h.env, "set provider as gemini"); err == nil {
		t.Error("gemini accepted")
	}
	h.run(t, "set zai-api-key as sk-1234567890abcd")
	out := h.run(t, "set")
	if !strings.Contains(out, "zai-api-key AS ****abcd") || strings.Contains(out, "1234567890") {
		t.Errorf("listing leaks key: %q", out)
	}
	h.run(t, `set json+ {"tts": "system", "n": 2}`)
	h.run(t, "set nill model")
	if _, ok := h.env.Settings.Get("model"); ok || h.env.Settings.String("tts") != "system" {
		t.Error("json+/nill")
	}
	// Persisted to disk.
	again, _ := config.Load(h.env.Paths.SettingsFile())
	if again.String("provider") != "zai" {
		t.Error("settings not saved")
	}
}

func TestStartChatModeAndSticky(t *testing.T) {
	h := newHarness(t, "ok", "Hi Ada!", "Fine.")
	out := h.run(t, "start chat-with-ai")
	if !strings.Contains(out, "Initializing chat with AI...") || !strings.Contains(out, "Chat with AI started.") || h.env.State.Mode() != "chat-with-ai" {
		t.Fatalf("out %q mode %q", out, h.env.State.Mode())
	}
	// In command mode the whole line goes to the command.
	if out := h.run(t, "help me, I'm Ada"); out != "Hi Ada!\n\n" {
		t.Errorf("chat out %q", out)
	}
	if h.sp.Last() != "help me, I'm Ada" {
		t.Errorf("sent %q", h.sp.Last())
	}
	h.env.State.Toggle()
	if h.env.State.Mode() != "" {
		t.Fatal("toggle to CLI")
	}
	h.run(t, "chat-with-ai how are you")
	h.env.State.Toggle()
	if h.env.State.Mode() != "chat-with-ai" {
		t.Error("toggle back to last mode")
	}
	h.env.State.Toggle()
	out = h.run(t, "start recap chat-with-ai")
	if !strings.Contains(out, "user-chat-input>  how are you") || !strings.Contains(out, "Fine.") {
		t.Errorf("recap %q", out)
	}
	// Still in chat mode, a "start" line would be sent to the AI.
	h.env.State.SetMode("")
	out = h.run(t, "start last chat-with-ai")
	if !strings.Contains(out, "user-chat-input>  how are you") || !strings.Contains(out, "Hi Ada!") || !strings.Contains(out, "Last chat with AI started.") {
		t.Errorf("undo %q", out)
	}
}

func TestStartErrors(t *testing.T) {
	h := newHarness(t)
	for line, want := range map[string]string{
		"start":                    "Unable to process command: No command supplied.",
		"start help":               "Unable to process command: Not a valid command mode to start.",
		"start new":                "Unable to process command: Not a valid command mode to start.",
		"start bogus chat-with-ai": "Unable to process command: Not a valid start type.",
		"nope":                     "Command not found.",
		"start old chat-with-ai":   "1 argument",
	} {
		_, err := h.reg.Execute(context.Background(), h.env, line)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", line, err)
		}
	}
}

// A startable command's start modes are logged as "<Type> <name> started."
// (dashes in the type become spaces), the plain start as "<Name> started."
// and a custom result as "<Result> started."; ErrNotStarted changes
// nothing and prints nothing.
func TestStartLogsAndNotStarted(t *testing.T) {
	h := newHarness(t)
	h.reg.Add(&Command{
		ID: "demo-mode", Name: "demo mode",
		Start: map[string]StartFunc{
			"":              func(context.Context, *Env, []string) (string, error) { return "", nil },
			"new-templated": func(context.Context, *Env, []string) (string, error) { return "", nil },
			"custom":        func(context.Context, *Env, []string) (string, error) { return "the demo-app", nil },
			"empty":         func(context.Context, *Env, []string) (string, error) { return "", ErrNotStarted },
		},
		Run: func(context.Context, *Env, Input) (string, error) { return "", nil },
	})
	for line, want := range map[string]string{
		"start demo-mode":               "] - Demo mode started.\n",
		"start new-templated demo-mode": "] - New templated demo mode started.\n",
		"start custom demo-mode":        "] - The demo app started.\n",
	} {
		h.env.State.SetMode("")
		if out := h.run(t, line); !strings.HasSuffix(out, want) || h.env.State.Mode() != "demo-mode" {
			t.Errorf("%q: out %q mode %q", line, out, h.env.State.Mode())
		}
	}
	h.env.State.SetMode("")
	if out := h.run(t, "start empty demo-mode"); out != "" || h.env.State.Mode() != "" {
		t.Errorf("not started: out %q mode %q", out, h.env.State.Mode())
	}
}

// The help table has fixed 13/75/34 columns, a dashed rule, a spacer row
// after the header and after every command, commands in id order and no
// footer; an unknown command is a plain "Command not found." line.
func TestHelpLayout(t *testing.T) {
	h := newHarness(t)
	sep, rule := "  |  ", "--|--"
	spacer := strings.Repeat(" ", 13) + sep + strings.Repeat(" ", 75) + sep + strings.Repeat(" ", 34)
	want := []string{
		"",
		"COMMAND      " + sep + "DESCRIPTION" + strings.Repeat(" ", 64) + sep + "EXAMPLES" + strings.Repeat(" ", 26),
		strings.Repeat("-", 13) + rule + strings.Repeat("-", 75) + rule + strings.Repeat("-", 34),
		spacer,
	}
	ids := []string{"app-using-ai", "chat-with-ai", "exit", "help", "multi-command", "say", "set", "start"}
	for _, id := range ids {
		c := h.reg.Get(id)
		want = append(want, fmt.Sprintf("%-13s%s%-75s%s%-34s", c.ID, sep, c.Description, sep, c.Examples), spacer)
	}
	if got := strings.Split(h.run(t, "help"), "\n"); !reflect.DeepEqual(got[:len(got)-1], want) || got[len(got)-1] != "" {
		t.Errorf("help =\n%q\nwant\n%q", got, want)
	}
	for id, c := range map[string][2]string{
		"help":  {"List the names, descriptions, examples, and input attributes of commands.", "help"},
		"exit":  {"Exit the application.", "exit"},
		"say":   {"Converts text to speech.", "say sync: (male-voice) hello there world!"},
		"set":   {"Set a variables for commands to use, or see what is currently set.", "set (<key> as <val>|json(+)? <json>|nill <key>)"},
		"start": {"Start a session with startable commands like chat-with-ai.", "start (new|old|last)? chat-with-ai"},
	} {
		if cmd := h.reg.Get(id); cmd.Description != c[0] || cmd.Examples != c[1] {
			t.Errorf("%s row: %q / %q", id, cmd.Description, cmd.Examples)
		}
	}
	// One command (help is case-insensitive); the rows keep their padding.
	one := h.run(t, "help EXIT")
	if !strings.HasPrefix(one, "\n"+want[1]+"\n"+want[2]+"\n"+spacer+"\n"+"exit         "+sep) || strings.Count(one, "\n") != 6 {
		t.Errorf("help exit = %q", one)
	}
	if out := h.run(t, "help nothing"); out != "Command not found.\n" {
		t.Errorf("help nothing = %q", out)
	}
	// The command word ends at a tab too.
	if out := h.run(t, "help\tsay"); !strings.Contains(out, "say          "+sep) {
		t.Errorf("help<tab>say = %q", out)
	}
}

// The bare listing is the settings' JSON with braces dropped, `":"` turned
// into ` AS `, commas into newlines and quotes removed; `set json` is the
// compact JSON followed by a blank line; nill prints plain lines.
func TestSetOutputFormat(t *testing.T) {
	h := newHarness(t)
	if out := h.run(t, "set"); out != "<KEY> AS <VALUE>:\n\n\n" {
		t.Errorf("empty listing = %q", out)
	}
	h.run(t, `set json {"model": "m-1", "n": 2, "flag": true, ".hidden": "x", "nested": {"a": "b, c"}, "list": ["x", "y"], "my-api-key": "sk-1234567890abcd"}`)
	want := "<KEY> AS <VALUE>:\n\n" +
		"flag:true\n" +
		"list:[x\ny]\n" +
		"model AS m-1\n" +
		"my-api-key AS ****abcd\n" +
		"n:2\n" +
		"nested:{a AS b\n c}\n"
	if out := h.run(t, "set"); out != want {
		t.Errorf("listing =\n%q\nwant\n%q", out, want)
	}
	if out := h.run(t, "set json"); !strings.HasPrefix(out, "JSON:\n\n{\".hidden\":\"x\",\"flag\":true,") || !strings.HasSuffix(out, "}\n\n") || strings.Contains(out, "\n  ") {
		t.Errorf("set json = %q", out)
	}
	if out := h.run(t, "set nill"); out != "No variable specified.\n" {
		t.Errorf("set nill = %q", out)
	}
	if out := h.run(t, "set nill model"); out != "model deleted.\n" {
		t.Errorf("set nill model = %q", out)
	}
	if _, ok := h.env.Settings.Get("model"); ok {
		t.Error("model not deleted")
	}
	if _, err := h.reg.Execute(context.Background(), h.env, "set json {oops"); err == nil || !strings.HasPrefix(err.Error(), "invalid JSON: ") {
		t.Errorf("bad json: %v", err)
	}
}

// In lock mode a line for any other command is silently ignored, in CLI
// mode and in command mode alike.
func TestLockMode(t *testing.T) {
	h := newHarness(t)
	h.env.State.Lock("say")
	h.env.State.SetMode("")
	for _, line := range []string{"help", "nope", "SET model as x"} {
		if out := h.run(t, line); out != "" {
			t.Errorf("%q: %q", line, out)
		}
	}
	if _, ok := h.env.Settings.Get("model"); ok {
		t.Error("set ran under lock")
	}
	h.run(t, "say hello")
	h.env.State.SetMode("set")
	h.run(t, "model as y")
	if _, ok := h.env.Settings.Get("model"); ok {
		t.Error("set ran in command mode under lock")
	}
	h.env.State.SetMode("say")
	h.run(t, "world")
	if !reflect.DeepEqual(h.rec.Said, []string{"hello", "world"}) {
		t.Errorf("said %q", h.rec.Said)
	}
}

func TestNewTemplatedAndTemplated(t *testing.T) {
	h := newHarness(t, "ok", "Bonjour", "name.json", "ok2")
	h.run(t, "chat-with-ai Speak French")
	h.run(t, "start new-templated chat-with-ai french")
	h.env.State.SetMode("")
	if _, err := os.Stat(filepath.Join(h.env.Paths.Root, config.Templates, "french")); err != nil {
		t.Fatal(err)
	}
	h.run(t, "start new chat-with-ai")
	h.env.State.SetMode("")
	h.run(t, "start templated chat-with-ai french")
	if h.env.Chat.Session("").Len() != 4 {
		t.Error("template not loaded")
	}
}

func TestSay(t *testing.T) {
	h := newHarness(t)
	h.run(t, "say sync: (male-voice) Hello There")
	h.run(t, "say plain")
	if !reflect.DeepEqual(h.rec.Said, []string{"(male) Hello There", "plain"}) {
		t.Errorf("said %q", h.rec.Said)
	}
}

func TestAppUsingAI(t *testing.T) {
	h := newHarness(t, "ok", `{"translationToSpanish": "Hola", "isEnglishToSpanishTranslation": true}`)
	out := h.run(t, "start new app-using-ai english2spanish")
	if !strings.Contains(out, "App English to Spanish started.") || h.env.State.Mode() != "app-using-ai" {
		t.Fatalf("out %q", out)
	}
	if out := h.run(t, "Hello"); out != "Hola\n" {
		t.Errorf("app out %q", out)
	}
	// Only the instruction exchange is in history: the translation was ephemeral.
	if h.env.Chat.Session("").Len() != 2 {
		t.Error("ephemeral translation kept in history")
	}
}

func TestAppSettingsAndUserApps(t *testing.T) {
	h := newHarness(t)
	dir := filepath.Join(h.env.Paths.Root, config.Apps)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "greet.ail.yaml"), []byte("name: Greeter\nsteps:\n  - print: ${settings.greeting}, ${input}!\n  - command: say ${input}\n"), 0o644)
	h.run(t, `start app-using-ai greet '{"greeting": "Howdy"}'`)
	if out := h.run(t, "Ada"); out != "Howdy, Ada!\n" {
		t.Errorf("out %q", out)
	}
	if len(h.rec.Said) != 1 || h.rec.Said[0] != "Ada" {
		t.Errorf("command step ran in app mode? said %q", h.rec.Said)
	}
	h.env.State.SetMode("")
	if out := h.run(t, "multi-command greet Bob"); out != ", Bob!\n" {
		t.Errorf("multi-command out %q", out)
	}
	if _, err := h.reg.Execute(context.Background(), h.env, "start app-using-ai missing-app"); err == nil {
		t.Error("missing app accepted")
	}
}

func TestExitAndHelp(t *testing.T) {
	h := newHarness(t)
	out := h.run(t, "help")
	for _, id := range []string{"chat-with-ai", "app-using-ai", "say", "set", "start", "multi-command", "exit"} {
		if !strings.Contains(out, id) {
			t.Errorf("help lacks %s", id)
		}
	}
	h.run(t, "EXIT")
	if !h.env.State.Stopped() {
		t.Error("exit")
	}
}
