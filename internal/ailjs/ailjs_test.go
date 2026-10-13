package ailjs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fusionhub "github.com/fusionpoweredsoftware/fusion-hub"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
)

// fakeHost scripts the model: reply(prompt) returns the text the "AI" says.
type fakeHost struct {
	out      *bytes.Buffer
	paths    config.Paths
	mode     string
	history  map[string][]Message
	channels map[string]map[string][]Message
	prompts  []string
	histLens []int
	execs    []string
	starts   []string
	reply    func(prompt string, n int) string
}

func newFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".fusion-hub", "fusion-hub")
	return &fakeHost{out: &bytes.Buffer{}, paths: config.Paths{Root: root}, history: map[string][]Message{}, channels: map[string]map[string][]Message{}}
}

func (h *fakeHost) Out() *output.Writer      { return output.New(h.out) }
func (h *fakeHost) Paths() config.Paths      { return h.paths }
func (h *fakeHost) Settings() map[string]any { return map[string]any{"provider": "scripted"} }
func (h *fakeHost) Mode() string             { return h.mode }
func (h *fakeHost) SetMode(m string)         { h.mode = m }
func (h *fakeHost) StartChat(_ context.Context, sid string, isNew bool) error {
	kind := "resume"
	if isNew {
		kind = "new"
		h.history[sid] = nil
	}
	h.starts = append(h.starts, kind)
	return nil
}
func (h *fakeHost) History(sid string) []Message { return h.history[sid] }
func (h *fakeHost) Chat(_ context.Context, _ string, history []Message, text string, stream io.Writer) (string, error) {
	h.prompts = append(h.prompts, text)
	h.histLens = append(h.histLens, len(history))
	r := h.reply(text, len(h.prompts))
	_, _ = io.WriteString(stream, r+"\n\n")
	return r, nil
}
func (h *fakeHost) Persist(sid string, channels map[string][]Message) error {
	h.history[sid] = channels["0"]
	h.channels[sid] = channels
	return nil
}
func (h *fakeHost) Execute(_ context.Context, line string, _ bool) error {
	h.execs = append(h.execs, line)
	if f := strings.Fields(line); len(f) >= 2 && f[0] == "start" {
		h.mode = f[len(f)-1]
	}
	return nil
}

// installExample copies a shipped example to <root>/multi-command, the
// directory the original resolved bare app names in.
func installExample(t *testing.T, h *fakeHost, name string) string {
	t.Helper()
	b, err := fusionhub.Legacy.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(h.paths.Root, "multi-command", filepath.Base(name))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Base(name)
}

func newEngine(t *testing.T, h *fakeHost) *Engine {
	t.Helper()
	e, err := New(h, fusionhub.Legacy)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestTransform(t *testing.T) {
	src := `import cli, { output, properties as cliProperties } from 'fusion-hub/cli';
import json5 from "json5"
import { a } from 'x';
const m = await import("fusion-hub/paths");
export const x = 1;
export async function process(d) { return d }
export function plain() {}
export const { chat } = m;
export default { x }
`
	got := transform(src)
	for _, want := range []string{
		`const __m0 = await __import("fusion-hub/cli"); const cli = __m0.default; const { output, properties : cliProperties } = __m0;`,
		`const __m1 = await __import("json5"); const json5 = __m1.default;`,
		`const __m2 = await __import("x"); const { a } = __m2;`,
		`await __import("fusion-hub/paths")`,
		"\nconst x = 1;", "\nasync function process(d)", "\nfunction plain()", "const { chat } = m;",
		"__exports.default = { x }",
		"__exports.x = x;", "__exports.process = process;", "__exports.plain = plain;", "__exports.chat = chat;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("transform output lacks %q:\n%s", want, got)
		}
	}
}

func TestActorExample(t *testing.T) {
	h := newFakeHost(t)
	h.reply = func(prompt string, n int) string {
		if !strings.Contains(prompt, "Provided code blocks:") || !strings.Contains(prompt, "(") {
			t.Errorf("prompt %d has no UIDMD inputs:\n%s", n, prompt)
		}
		// Answer with the response block using the uuid from the prompt.
		i := strings.Index(prompt, "***plaintext (")
		uid := prompt[i+len("***plaintext (") : i+len("***plaintext (")+strings.Index(prompt[i+len("***plaintext ("):], ":")]
		return "Sure.\n\n***plaintext (" + uid + ":response)\nTime is relative, my friend.\n(" + uid + ":response)\n"
	}
	e := newEngine(t, h)
	app := installExample(t, h, "example/actor.app.ail.js")
	if err := e.RunApp(context.Background(), app, "What is time?", map[string]any{}, "", false); err != nil {
		t.Fatalf("RunApp: %v\n%s", err, h.out)
	}
	out := h.out.String()
	want := "\n" + "\x1b[34m" + "Albert Einstein>" + "\x1b[0m" + " Time is relative, my friend.\n"
	if !strings.Contains(out, want) {
		t.Errorf("output %q lacks %q", out, want)
	}
	if len(h.prompts) != 1 {
		t.Errorf("prompts = %d, want 1", len(h.prompts))
	}
	p := h.prompts[0]
	for _, s := range []string{"I have provided you with the person, biosignature, rules, and input code blocks using UIDMD notation", ":person)\nAlbert Einstein\n(", ":input)\nWhat is time?\n(", "1. Never use non-English characters."} {
		if !strings.Contains(p, s) {
			t.Errorf("prompt lacks %q:\n%s", s, p)
		}
	}
	// The UIDMD explanation exchange was unshifted into the history, then the
	// real exchange appended.
	hist := h.history[""]
	if len(hist) != 4 || !strings.Contains(hist[0].Text, "uidmd_explanation") || hist[3].Role != "model" {
		t.Errorf("history = %+v", hist)
	}
	// STOP left command mode, then "start app-using-ai" was dispatched.
	if len(h.execs) != 1 || h.execs[0] != "start app-using-ai" {
		t.Errorf("execs = %q", h.execs)
	}
	if _, err := os.Stat(filepath.Join(h.paths.Root, "actors", "actors.json")); err != nil {
		t.Errorf("actors.json not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.paths.Root, "multi-command", ".actor.app.ail.mjs")); err != nil {
		t.Errorf("debuggable module not written: %v", err)
	}
}

func TestEnglish2SpanishExampleAsWritten(t *testing.T) {
	// The original declares the output as UIDMD, so a JSON reply never
	// satisfies isInvalid(): six requests, then "Unable to translate.".
	h := newFakeHost(t)
	h.reply = func(string, int) string {
		return "```json\n{\"english\": \"Hi\", \"translationToSpanish\": \"Hola\", \"isEnglishToSpanishTranslation\": true}\n```"
	}
	e := newEngine(t, h)
	app := installExample(t, h, "example/english2spanish.app.ail.js")
	if err := e.RunApp(context.Background(), app, "Hi", nil, "", false); err != nil {
		t.Fatalf("RunApp: %v\n%s", err, h.out)
	}
	if len(h.prompts) != 6 {
		t.Errorf("prompts = %d, want 6", len(h.prompts))
	}
	if !strings.Contains(h.out.String(), "Unable to translate.\n") {
		t.Errorf("output: %q", h.out)
	}
	if h.execs[0] != "start chat-with-ai" {
		t.Errorf("execs = %q", h.execs)
	}
}

func TestSpokenChatExample(t *testing.T) {
	h := newFakeHost(t)
	h.reply = func(string, int) string { return "Hello *there*!\n\nSecond paragraph." }
	e := newEngine(t, h)
	app := installExample(t, h, "example/spoken-chat-with-ai.app.ail.js")
	if err := e.RunApp(context.Background(), app, "hi", nil, "", false); err != nil {
		t.Fatalf("RunApp: %v\n%s", err, h.out)
	}
	if want := "Hello there!\n\nSecond paragraph.\n\n"; !strings.Contains(h.out.String(), want) {
		t.Errorf("output %q lacks %q", h.out, want)
	}
	if want := []string{"say sync: Hello there!", "say sync: Second paragraph.", "start app-using-ai"}; strings.Join(h.execs, "|") != strings.Join(want, "|") {
		t.Errorf("execs = %q", h.execs)
	}
	if h.prompts[0] != "hi" {
		t.Errorf("prompt = %q", h.prompts[0])
	}
}

func TestSayMoreExampleAndDebugStreaming(t *testing.T) {
	h := newFakeHost(t)
	e := newEngine(t, h)
	app := installExample(t, h, "example/say-more.app.ail.js")
	if err := e.RunApp(context.Background(), app, "sync: (male-voice) hello", nil, "", true); err != nil {
		t.Fatalf("RunApp: %v\n%s", err, h.out)
	}
	if h.execs[0] != "say sync: (male-voice) hello" {
		t.Errorf("execs = %q", h.execs)
	}
	// Debug mode logs the generated module.
	if !strings.Contains(h.out.String(), "export const run = async (input, app, aiChatSettings)") {
		t.Errorf("debug output lacks the generated module:\n%s", h.out)
	}
}

func TestMultiCommandDefaults(t *testing.T) {
	// From the REPL, multi-command gives the app no app.input, so the
	// original's first block starts a new chat and returns nothing; the
	// script then fails on response.replace, exactly as it did in node.
	h := newFakeHost(t)
	h.reply = func(string, int) string { return "ok" }
	e := newEngine(t, h)
	app := installExample(t, h, "example/spoken-chat-with-ai.app.ail.js")
	err := e.MultiCommand(context.Background(), []string{app, "hi", "there"}, app+" hi there")
	if err == nil || !strings.Contains(err.Error(), "Cannot read property 'replace'") {
		t.Fatalf("MultiCommand: %v\n%s", err, h.out)
	}
	out := h.out.String()
	if !strings.Contains(out, "Creating history file as: ~~~/"+app) {
		t.Errorf("no history log:\n%s", out)
	}
	if !strings.Contains(out, "export const run = async (input, app, aiChatSettings)") {
		t.Errorf("generated module not logged:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(h.paths.Root, "multi-command-history")); err != nil {
		t.Error(err)
	}
	if len(h.prompts) != 0 || strings.Join(h.starts, ",") != "new" {
		t.Errorf("prompts = %q starts = %q", h.prompts, h.starts)
	}
}

func TestFileNotFound(t *testing.T) {
	h := newFakeHost(t)
	e := newEngine(t, h)
	err := e.RunApp(context.Background(), "nope.app.ail.js", "x", nil, "", false)
	if err == nil || !strings.Contains(err.Error(), "ENOENT") {
		t.Errorf("err = %v", err)
	}
}

func uidOf(prompt string) string {
	i := strings.Index(prompt, "***plaintext (")
	if i < 0 {
		return ""
	}
	rest := prompt[i+len("***plaintext ("):]
	return rest[:strings.Index(rest, ":")]
}

func TestOtfaTextExample(t *testing.T) {
	h := newFakeHost(t)
	turn := 0
	h.reply = func(prompt string, n int) string {
		switch {
		case strings.HasPrefix(prompt, "Verify the inventory contents"):
			if turn == 1 {
				return `["lantern"]`
			}
			return `[]`
		case strings.HasPrefix(prompt, "Are you giving the player options"):
			return "No"
		case strings.Contains(prompt, "My command is: start new game"):
			return "You wake up in a damp cellar. A lantern flickers on a hook. Stairs lead up into darkness. The air smells of rust."
		case strings.Contains(prompt, "My command is: take lantern"):
			return "You lift the lantern from its hook. Its warm light pushes the shadows back. The stairs look less forbidding now. Something glints near the top step.\n+"
		}
		t.Errorf("unexpected prompt %d: %q", n, prompt)
		return ""
	}
	e := newEngine(t, h)
	app := installExample(t, h, "example/just-testing/otfa-text.app.ail.js")
	settings := map[string]any{"theme": "dungeon"}
	if err := e.RunApp(context.Background(), app, "start new game", settings, "", false); err != nil {
		t.Fatalf("RunApp: %v\n%s", err, h.out)
	}
	if !strings.Contains(h.prompts[0], "Respond like I'm playing a dungeon themed text adventure.") || strings.Contains(h.prompts[0], "At the end of your response provide") {
		t.Errorf("start prompt: %q", h.prompts[0])
	}
	if want := "You wake up in a damp cellar. A lantern flickers on a hook. Stairs lead up into darkness. The air smells of rust.\n0\n"; !strings.Contains(h.out.String(), want) {
		t.Errorf("output %q lacks %q", h.out, want)
	}
	if len(h.prompts) != 3 {
		t.Errorf("prompts after start = %d (%q)", len(h.prompts), h.prompts)
	}
	// The file is written by each chat call, so it holds the inventory check
	// too; the app then pops it from the history it keeps between lines.
	if len(h.history[""]) != 4 {
		t.Errorf("history = %+v", h.history[""])
	}
	turn = 1
	h.out.Reset()
	if err := e.RunApp(context.Background(), app, "take lantern", settings, "", false); err != nil {
		t.Fatalf("RunApp: %v\n%s", err, h.out)
	}
	if !strings.Contains(h.prompts[3], "At the end of your response provide a positive, neutral, or negative on its own line") {
		t.Errorf("turn prompt: %q", h.prompts[3])
	}
	if want := "Something glints near the top step.\n\n+\n"; !strings.Contains(h.out.String(), want) {
		t.Errorf("output %q lacks %q", h.out, want)
	}
	state, err := os.ReadFile(filepath.Join(h.paths.Root, "otfa", "current-otfa.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), `"inventory":["lantern"]`) {
		t.Errorf("game state: %s", state)
	}
	// The second line's main prompt saw only the first turn (2 messages),
	// not the popped verification exchanges.
	if want := []int{0, 2, 2, 2, 4, 4}; fmt.Sprint(h.histLens) != fmt.Sprint(want) {
		t.Errorf("history lengths seen by the model = %v, want %v", h.histLens, want)
	}
	// An app-using-ai start reloads the history from the file.
	e.ForgetHistory("")
	h.out.Reset()
	turn = 1
	if err := e.RunApp(context.Background(), app, "take lantern", settings, "", false); err != nil {
		t.Fatalf("RunApp: %v\n%s", err, h.out)
	}
	if got := h.histLens[6]; got != 6 {
		t.Errorf("after ForgetHistory the model saw %d messages, want 6", got)
	}
}

func TestReActorExample(t *testing.T) {
	h := newFakeHost(t)
	h.reply = func(prompt string, n int) string {
		uid := uidOf(prompt)
		who := "Einstein"
		if strings.Contains(prompt, ":person)\nAmelia Earhart\n(") {
			who = "Earhart"
		}
		return "***plaintext (" + uid + ":response)\n" + who + " speaks.\n(" + uid + ":response)"
	}
	e := newEngine(t, h)
	app := installExample(t, h, "example/just-testing/re-actor.app.ail.js")
	settings := map[string]any{"ids": []any{"EINSTEIN", "EARHART"}, "conversationLength": 1}
	if err := e.RunApp(context.Background(), app, "the nature of flight", settings, "", false); err != nil {
		t.Fatalf("RunApp: %v\n%s", err, h.out)
	}
	out := h.out.String()
	for _, want := range []string{"\x1b[34mAlbert Einstein>\x1b[0m Einstein speaks.", "\x1b[34mAmelia Earhart>\x1b[0m Earhart speaks."} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}
	if len(h.prompts) != 2 {
		t.Errorf("prompts = %d", len(h.prompts))
	}
	// Each turn is popped and a synthetic "conversation so far" pair spliced
	// in, so the model saw the explanation, the conversation so far and the
	// request; the file (written before the pop) holds all three pairs.
	hist := h.history[""]
	// (The pair is spliced in before the speaker's line is appended to the
	// conversation, so Earhart was shown the topic alone, as in node.)
	if len(hist) != 6 || !strings.HasSuffix(hist[2].Text, "This is the conversation so far:\n\nToday we are discussing: the nature of flight") || !strings.Contains(hist[4].Text, ":person)\nAmelia Earhart\n(") {
		for i, m := range hist {
			t.Logf("%d %s: %.80q", i, m.Role, m.Text)
		}
		t.Errorf("history has %d messages", len(hist))
	}
}
