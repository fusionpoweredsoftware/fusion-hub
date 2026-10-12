package commands

import (
	"bytes"
	"context"
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
	rec := &tts.Recorder{}
	reg := NewRegistry()
	return &harness{
		env: &Env{Out: output.New(out), Settings: settings, Paths: paths, Chat: m, Speaker: rec, State: &State{}, Registry: reg},
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
	if !strings.Contains(out, "ZAI-API-KEY AS ****abcd") || strings.Contains(out, "1234567890") {
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
	h := newHarness(t, "Hi Ada!", "Fine.")
	out := h.run(t, "start chat-with-ai")
	if !strings.Contains(out, "Chat with AI started.") || h.env.State.Mode() != "chat-with-ai" {
		t.Fatalf("out %q mode %q", out, h.env.State.Mode())
	}
	// In command mode the whole line goes to the command.
	if out := h.run(t, "help me, I'm Ada"); out != "Hi Ada!\n" {
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
	if !strings.Contains(out, "help me, I'm Ada") || !strings.Contains(out, "Last chat with AI started.") {
		t.Errorf("undo %q", out)
	}
}

func TestStartErrors(t *testing.T) {
	h := newHarness(t)
	for line, want := range map[string]string{
		"start":                    "No command supplied",
		"start help":               "Not a valid command mode",
		"start bogus chat-with-ai": "Not a valid start type",
		"nope":                     "Command not found",
		"start old chat-with-ai":   "1 argument",
	} {
		_, err := h.reg.Execute(context.Background(), h.env, line)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", line, err)
		}
	}
}

func TestNewTemplatedAndTemplated(t *testing.T) {
	h := newHarness(t, "Bonjour", "name.json")
	h.run(t, "chat-with-ai Speak French")
	h.run(t, "start new-templated chat-with-ai french")
	h.env.State.SetMode("")
	if _, err := os.Stat(filepath.Join(h.env.Paths.Root, config.Templates, "french.json")); err != nil {
		t.Fatal(err)
	}
	h.run(t, "start new chat-with-ai")
	h.env.State.SetMode("")
	h.run(t, "start templated chat-with-ai french")
	if h.env.Chat.Session("").Len() != 2 {
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
	h := newHarness(t, `{"translationToSpanish": "Hola", "isEnglishToSpanishTranslation": true}`)
	out := h.run(t, "start new app-using-ai english2spanish")
	if !strings.Contains(out, "App English to Spanish started.") || h.env.State.Mode() != "app-using-ai" {
		t.Fatalf("out %q", out)
	}
	if out := h.run(t, "Hello"); out != "Hola\n" {
		t.Errorf("app out %q", out)
	}
	if h.env.Chat.Session("").Len() != 0 {
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
