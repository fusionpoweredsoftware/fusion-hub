package ail

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fusionpoweredsoftware/fusion-hub/examples"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/actors"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/tts"
)

// fakeChat replays replies and records prompts and the history length.
type fakeChat struct {
	replies []string
	prompts []string
	history int
	undone  int
}

func (f *fakeChat) runtime(out *bytes.Buffer) *Runtime {
	return &Runtime{
		Out: output.New(out),
		Now: func() time.Time { return time.Date(2024, 1, 28, 6, 11, 28, 170e6, time.UTC) },
		Chat: func(_ context.Context, prompt string, opts ChatOptions) (string, error) {
			f.prompts = append(f.prompts, prompt)
			if len(f.replies) == 0 {
				return "", os.ErrClosed
			}
			r := f.replies[0]
			f.replies = f.replies[1:]
			if !opts.Ephemeral {
				f.history += 2
			}
			return r, nil
		},
		HistoryLen: func() int { return f.history },
		Undo:       func() error { f.history -= 2; f.undone++; return nil },
	}
}

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		"Here you go:\n```json\n{\"a\": [1, 2,],}\n```\nthanks":      `{"a":[1,2]}`,
		`Sure! {"a": "has } brace", "b": {"c": true}} trailing text`: `{"a":"has } brace","b":{"c":true}}`,
		`[1, 2, 3]`: `[1,2,3]`,
	}
	for in, want := range cases {
		v, err := ExtractJSON(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if b, _ := json.Marshal(v); string(b) != want {
			t.Errorf("%q = %s, want %s", in, b, want)
		}
	}
	if _, err := ExtractJSON("no json here"); err == nil {
		t.Error("expected error")
	}
	if _, err := ExtractJSON(`{"a": }`); err == nil {
		t.Error("expected malformed error")
	}
}

func TestTemplates(t *testing.T) {
	v := Vars{"name": "Ada", "list": []any{"x", map[string]any{"k": "deep"}}, "n": 3, "empty": ""}
	got, err := v.Render("Hi ${name}, ${list.1.k}, ${list|len}, ${n}, ${missing}|${empty | default:none}|${name|upper}|${list|json}")
	if err != nil || got != `Hi Ada, deep, 2, 3, |none|ADA|["x",{"k":"deep"}]` {
		t.Fatalf("got %q %v", got, err)
	}
	if val, _ := v.Value("${list}"); len(val.([]any)) != 2 {
		t.Errorf("Value kept type? %#v", val)
	}
	if _, err := v.Render("${name|bogus}"); err == nil {
		t.Error("unknown filter accepted")
	}
	for expr, want := range map[string]bool{"${name}": true, "${missing}": false, "${name} == Ada": true, "${name} != 'Ada'": false, "${n} == 3": true, "${empty}": false} {
		if got, _ := v.Condition(expr); got != want {
			t.Errorf("Condition(%q) = %v", expr, got)
		}
	}
	if err := v.Assign("list.1.k", "new"); err != nil || v["list"].([]any)[1].(map[string]any)["k"] != "new" {
		t.Errorf("assign: %v %#v", err, v["list"])
	}
	if err := v.Assign("name.x", 1); err == nil {
		t.Error("assign into string accepted")
	}
}

func TestParseValidation(t *testing.T) {
	bad := map[string]string{
		"no steps":       "name: x\n",
		"two actions":    "steps:\n  - print: a\n    say: b\n",
		"no action":      "steps:\n  - id: x\n",
		"bad output":     "steps:\n  - prompt: hi\n    output: xml\n",
		"unknown field":  "steps:\n  - print: a\n    colour: red\n",
		"inputs on text": "steps:\n  - prompt: hi\n    inputs: [a]\n",
		"empty loop":     "steps:\n  - foreach: ${x}\n",
		"nested print":   "steps:\n  - print: a\n    steps:\n      - print: b\n",
	}
	for name, src := range bad {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestExamplesParse(t *testing.T) {
	files, _ := fs.Glob(examples.FS, "*.ail.yaml")
	if len(files) < 6 {
		t.Fatalf("only %d examples", len(files))
	}
	for _, f := range files {
		b, _ := examples.FS.ReadFile(f)
		if _, err := Parse(b); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func load(t *testing.T, name string) *Pipeline {
	t.Helper()
	b, err := examples.FS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEnglishToSpanishRetriesMalformedJSON(t *testing.T) {
	var out bytes.Buffer
	f := &fakeChat{replies: []string{
		"I think it is {\"translationToSpanish\": ",
		"```json\n{\"english\": \"Good morning\", \"translationToSpanish\": \"Buenos días\", \"isEnglishToSpanishTranslation\": true}\n```",
	}}
	vars, err := load(t, "english2spanish.ail.yaml").Run(context.Background(), f.runtime(&out), "Good morning", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "Buenos días" {
		t.Errorf("output %q", out.String())
	}
	if !strings.Contains(f.prompts[0], `"Good morning"`) || !strings.Contains(f.prompts[1], "malformed JSON") {
		t.Errorf("prompts %q", f.prompts)
	}
	if f.history != 0 {
		t.Errorf("ephemeral step changed history: %d", f.history)
	}
	if vars["translation"].(map[string]any)["english"] != "Good morning" {
		t.Errorf("vars %#v", vars)
	}
}

func TestEnglishToSpanishFails(t *testing.T) {
	f := &fakeChat{replies: []string{`{"translationToSpanish": "x", "isEnglishToSpanishTranslation": false}`}}
	_, err := load(t, "english2spanish.ail.yaml").Run(context.Background(), f.runtime(&bytes.Buffer{}), "hi", nil)
	if err == nil || err.Error() != "Unable to translate." {
		t.Fatalf("err = %v", err)
	}
}

func TestSpokenChatSplitsParagraphs(t *testing.T) {
	var out bytes.Buffer
	f := &fakeChat{replies: []string{"First paragraph.\n\nSecond one!"}}
	rt := f.runtime(&out)
	rec := &tts.Recorder{}
	rt.Speaker = rec
	if _, err := load(t, "spoken-chat-with-ai.ail.yaml").Run(context.Background(), rt, "tell me", nil); err != nil {
		t.Fatal(err)
	}
	if len(rec.Said) != 2 || rec.Said[1] != "Second one!" {
		t.Errorf("said %q", rec.Said)
	}
	if out.String() != "First paragraph.\nSecond one!\n" {
		t.Errorf("out %q", out.String())
	}
}

func TestUIDMDStepWithInputs(t *testing.T) {
	src := `
variables:
  topic: whales
steps:
  - id: answer
    output: uidmd
    inputs: [topic, settings.style]
    require: [fact]
    prompt: Tell me a fact.
  - print: ${answer.fact}
`
	p, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	f := &fakeChat{replies: []string{"no blocks", "*** plaintext (20240128T061128170Z:fact)\nOrcas are dolphins.\n(20240128T061128170Z:fact)"}}
	if _, err := p.Run(context.Background(), f.runtime(&out), "", map[string]any{"style": "terse"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "Orcas are dolphins." {
		t.Errorf("out %q", out.String())
	}
	for _, want := range []string{"*** plaintext (20240128T061128170Z:topic)\nwhales", "(20240128T061128170Z:style)", "UIDMD notation"} {
		if !strings.Contains(f.prompts[0], want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if !strings.Contains(f.prompts[1], "missing these UIDMD code blocks: fact") {
		t.Errorf("retry prompt %q", f.prompts[1])
	}
}

func TestLoopsSetAndSave(t *testing.T) {
	file := filepath.Join(t.TempDir(), "out", "game.json")
	src := `
variables:
  game:
    rooms: [{name: hall}, {name: cellar}]
steps:
  - foreach: ${game.rooms}
    as: room
    id: visited
    steps:
      - set:
          room.index: ${index}
  - repeat: 10
    until: ${index} == 2
    steps:
      - set: {count: "${index}"}
  - save: ` + file + `
    value: ${game}
  - print: rooms=${visited|len} count=${count}
  - stop: true
  - print: unreachable
`
	p, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := p.Run(context.Background(), (&fakeChat{}).runtime(&out), "", nil); err != nil {
		t.Fatal(err)
	}
	if out.String() != "rooms=2 count=2\n" {
		t.Errorf("out %q", out.String())
	}
	b, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(b), `"index": 1`) {
		t.Errorf("saved %s %v", b, err)
	}
}

func TestCommandStep(t *testing.T) {
	p, _ := Parse([]byte("steps:\n  - id: r\n    command: say ${input}\n  - print: got ${r}\n"))
	var out bytes.Buffer
	rt := (&fakeChat{}).runtime(&out)
	rt.Command = func(_ context.Context, line string) (string, error) { return "ran " + line, nil }
	if _, err := p.Run(context.Background(), rt, "hi", nil); err != nil || out.String() != "got ran say hi\n" {
		t.Fatalf("%q %v", out.String(), err)
	}
}

func actorReply(text string) string {
	return "*** plaintext (20240128T061128170Z:response)\n" + text + "\n(20240128T061128170Z:response)"
}

func TestActorRetriesAndSpeaks(t *testing.T) {
	var out bytes.Buffer
	f := &fakeChat{replies: []string{"I am an AI.", "*** plaintext (20240128T061128170Z:response)\n你好\n(20240128T061128170Z:response)", actorReply("Imagination is more important than knowledge.\n\n(more)")}}
	rt := f.runtime(&out)
	rec := &tts.Recorder{}
	rt.Speaker, rt.Actors = rec, actors.Builtin()
	vars, err := load(t, "actor.ail.yaml").Run(context.Background(), rt, "What matters?", map[string]any{"id": "einstein", "voice": "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if f.undone != 2 || !strings.Contains(f.prompts[2], "NO CHINESE OR JAPANESE") {
		t.Errorf("undone %d, last prompt %q", f.undone, f.prompts[2])
	}
	if !strings.Contains(f.prompts[0], "*** plaintext (20240128T061128170Z:person)\nAlbert Einstein") {
		t.Errorf("prompt %q", f.prompts[0])
	}
	if len(rec.Said) != 1 || rec.Said[0] != "(male) Imagination is more important than knowledge." {
		t.Errorf("said %q", rec.Said)
	}
	if !strings.HasPrefix(vars["reply"].(string), "Imagination") || !strings.Contains(out.String(), "Albert Einstein>") {
		t.Errorf("reply %v out %q", vars["reply"], out.String())
	}
}

func TestReActorTakesTurns(t *testing.T) {
	var out bytes.Buffer
	f := &fakeChat{replies: []string{actorReply("Relativity!"), actorReply("Flight!")}}
	rt := f.runtime(&out)
	rt.Actors = actors.Builtin()
	vars, err := load(t, "re-actor.ail.yaml").Run(context.Background(), rt, "travel", nil)
	if err != nil {
		t.Fatal(err)
	}
	conv := vars["discussion"].(string)
	if !strings.Contains(conv, "Albert Einstein said: Relativity!") || !strings.Contains(conv, "Amelia Earhart said: Flight!") {
		t.Errorf("conversation %q", conv)
	}
	if !strings.Contains(f.prompts[1], "Albert Einstein said: Relativity!") {
		t.Errorf("second actor did not see the first: %q", f.prompts[1])
	}
}

func TestAdventureDesignWalksTheTree(t *testing.T) {
	dir := t.TempDir()
	f := &fakeChat{}
	f.replies = []string{
		"rough draft", "story",
		`{"title": "T", "gameBackgroundStory": "B", "chapters": [{"chapterTitle": "C1", "chapterText": "x"}]}`,
		`{"chapterObjective": "O", "objectives": [{"objectiveDescription": "find key", "pointsGained": 1}]}`,
		`{"newRooms": [{"roomName": "Hall", "outgoingConnectedRooms": []}]}`,
		`{"puzzles": [{"puzzleName": "P"}]}`,
		`{"items": [{"name": "key"}]}`,
		`{"characters": []}`,
	}
	var out bytes.Buffer
	file := filepath.Join(dir, "game.json")
	if _, err := load(t, "adventure-design.ail.yaml").Run(context.Background(), f.runtime(&out), "", map[string]any{"theme": "pirate", "file": file}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(file)
	var game map[string]any
	if err := json.Unmarshal(b, &game); err != nil {
		t.Fatal(err)
	}
	room := game["chapters"].([]any)[0].(map[string]any)["details"].(map[string]any)["objectives"].([]any)[0].(map[string]any)["newRooms"].([]any)[0].(map[string]any)
	if room["items"].([]any)[0].(map[string]any)["name"] != "key" {
		t.Errorf("room = %#v", room)
	}
	if !strings.Contains(f.prompts[0], "pirate themed") || !strings.Contains(f.prompts[0], "3 chapters") {
		t.Errorf("goal not rendered: %q", f.prompts[0][:200])
	}
}
