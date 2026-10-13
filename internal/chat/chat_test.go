package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/history"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/provider"
)

type fixture struct {
	m   *Manager
	sp  *provider.Scripted
	log *bytes.Buffer
}

func newFixture(t *testing.T, replies ...string) *fixture {
	t.Helper()
	paths := config.Paths{Root: t.TempDir()}
	m := NewManager(paths, config.NewMemory())
	sp := &provider.Scripted{Replies: replies}
	m.Provider = func() (provider.Provider, error) { return sp, nil }
	m.Now = func() time.Time { return time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC) }
	log := &bytes.Buffer{}
	m.Console = output.New(log)
	return &fixture{m: m, sp: sp, log: log}
}

func (f *fixture) histories(name string) string {
	return filepath.Join(f.m.Paths.Root, config.Histories, name)
}

func (f *fixture) logged(t *testing.T, want ...string) {
	t.Helper()
	got := f.log.String()
	for _, w := range want {
		if !strings.Contains(got, "] - "+w+"\n") {
			t.Errorf("log lacks %q:\n%s", w, got)
		}
	}
}

func TestSendStartsWithInstructionsOnce(t *testing.T) {
	f := newFixture(t, "ok", "Hello there", "Fine")
	ctx := context.Background()
	var out bytes.Buffer
	reply, err := f.m.Send(ctx, "", "hi", &out, SendOptions{})
	if err != nil || reply != "Hello there" || out.String() != "Hello there\n\n" {
		t.Fatalf("reply %q out %q err %v", reply, out.String(), err)
	}
	f.logged(t, "Initializing chat with AI...")
	if first := f.sp.Requests[0].Messages; len(first) != 1 || first[0].Content != DefaultInstructions || first[0].Role != history.RoleUser {
		t.Errorf("instructions not the first user turn: %#v", first)
	}
	if f.sp.Requests[0].System != "" || f.sp.Requests[0].MaxTokens != 0 {
		t.Errorf("request carries system/max tokens: %#v", f.sp.Requests[0])
	}
	b, _ := os.ReadFile(f.m.CurrentFile(""))
	want := `{"0":[{"role":"user","parts":[{"text":"` + DefaultInstructions + `"}]},{"role":"model","parts":[{"text":"ok"}]},{"role":"user","parts":[{"text":"hi"}]},{"role":"model","parts":[{"text":"Hello there"}]}]}`
	if string(b) != want {
		t.Errorf("file\n%s\nwant\n%s", b, want)
	}
	f.log.Reset()
	f.m.Send(ctx, "", "again", nil, SendOptions{})
	if f.log.Len() != 0 {
		t.Errorf("second message logged: %s", f.log.String())
	}
	// A restart resumes the saved conversation and says so once.
	m2 := NewManager(f.m.Paths, f.m.Settings)
	m2.Console = output.New(f.log)
	if err := m2.Start(ctx, ""); err != nil {
		t.Fatal(err)
	}
	f.logged(t, "Resuming chat with AI...")
	if m2.Session("").Len() != 6 {
		t.Errorf("resumed %d messages", m2.Session("").Len())
	}
	if n := strings.Count(f.log.String(), "chat with AI..."); n != 1 {
		t.Errorf("start logged %d times", n)
	}
}

func TestInstructionsSettingAndSessionFiles(t *testing.T) {
	f := newFixture(t, "oui")
	f.m.Settings.Set("instructions", "Parle français.")
	if err := f.m.Start(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	if f.sp.Last() != "Parle français." {
		t.Errorf("sent %q", f.sp.Last())
	}
	if !strings.HasSuffix(f.m.CurrentFile("s1"), ".ai-chat-history.json.s1") || !strings.HasSuffix(f.m.CurrentFile("a/b"), ".ai-chat-history.json.a_b") {
		t.Errorf("session files: %s %s", f.m.CurrentFile("s1"), f.m.CurrentFile("a/b"))
	}
	if f.m.CurrentFile("0") != f.m.CurrentFile("") || filepath.Base(f.m.CurrentFile("")) != ".ai-chat-history.json" {
		t.Errorf("default file: %s", f.m.CurrentFile("0"))
	}
	if v, _ := f.m.Settings.Get(SaveAsKey); v != nil {
		t.Errorf("%s = %v, want null", SaveAsKey, v)
	}
}

func TestEmptyReplyAndBangRetry(t *testing.T) {
	f := newFixture(t, "ok", "", "yes", "")
	ctx := context.Background()
	var out bytes.Buffer
	reply, err := f.m.Send(ctx, "", "hi!", &out, SendOptions{})
	if err != nil || reply != "yes" || out.String() != "yes\n\n\n\n" {
		t.Fatalf("reply %q out %q err %v", reply, out.String(), err)
	}
	if f.sp.Last() != "hi !" {
		t.Errorf("retried with %q", f.sp.Last())
	}
	if c := f.m.Session("").Snapshot(); c.Messages[2].Content != "hi !" {
		t.Errorf("history kept %q", c.Messages[2].Content)
	}
	out.Reset()
	reply, err = f.m.Send(ctx, "", "hi", &out, SendOptions{})
	if err != nil || reply != "" || out.String() != "(No response)\n\n" {
		t.Fatalf("reply %q out %q err %v", reply, out.String(), err)
	}
	if f.m.Session("").Len() != 4 {
		t.Errorf("history changed on empty reply: %d", f.m.Session("").Len())
	}
}

// flaky fails the first n requests with err, then answers "late".
type flaky struct {
	n   int
	err error
}

func (p *flaky) Name() string { return "flaky" }
func (p *flaky) Stream(_ context.Context, _ provider.Request, onDelta func(string)) (string, error) {
	if p.n > 0 {
		p.n--
		return "", p.err
	}
	if onDelta != nil {
		onDelta("late")
	}
	return "late", nil
}

func TestProviderErrorsRetryOnce(t *testing.T) {
	f := newFixture(t)
	fp := &flaky{n: 1, err: errors.New("boom")}
	f.m.Provider = func() (provider.Provider, error) { return fp, nil }
	ctx := context.Background()
	var out bytes.Buffer
	if reply, err := f.m.Send(ctx, "", "q", &out, SendOptions{}); err != nil || reply != "late" {
		t.Fatalf("retry: %q %v", reply, err)
	}
	fp.n = 2
	if _, err := f.m.Send(ctx, "", "q2", nil, SendOptions{}); err == nil || err.Error() != "boom" {
		t.Fatalf("second failure not returned: %v", err)
	}
	if f.m.Session("").Len() != 4 {
		t.Errorf("history changed on error: %d", f.m.Session("").Len())
	}
	fp.n, fp.err = 2, &provider.HTTPError{Status: http.StatusTooManyRequests, Msg: "openai: 429 Too Many Requests"}
	if _, err := f.m.Send(ctx, "", "q3", nil, SendOptions{}); err == nil || err.Error() != BusyText {
		t.Fatalf("429 err = %v", err)
	}
	f.m.Provider = func() (provider.Provider, error) { return nil, errors.New("no API key") }
	if _, err := f.m.Send(ctx, "", "q4", nil, SendOptions{}); err == nil {
		t.Error("provider error swallowed")
	}
}

func TestEphemeralLeavesHistory(t *testing.T) {
	f := newFixture(t, "ok", "x")
	if _, err := f.m.Send(context.Background(), "s1", "q", nil, SendOptions{Ephemeral: true}); err != nil {
		t.Fatal(err)
	}
	if f.m.Session("s1").Len() != 2 {
		t.Errorf("history = %d, want just the instruction exchange", f.m.Session("s1").Len())
	}
}

func TestNewArchivesWithAIName(t *testing.T) {
	f := newFixture(t, "ok", "Chess openings are...", `How about "chess-openings.json"?`, "ok2", `"chess-openings.json"`, "ok3")
	ctx := context.Background()
	f.m.Send(ctx, "", "tell me about chess", nil, SendOptions{})
	os.WriteFile(f.m.Paths.ModeHistoryFile(), []byte("tell me about chess\n"), 0o644)
	f.log.Reset()
	if err := f.m.New(ctx, "", ""); err != nil {
		t.Fatal(err)
	}
	f.logged(t, "Finalizing previous chat with AI...", "Previous conversation saved as: chess-openings.json", "Starting new chat with AI...")
	if name := f.sp.Requests[2]; !strings.Contains(name.Messages[len(name.Messages)-1].Content, ".JSON filename") || len(name.Messages) != 5 {
		t.Errorf("filename request = %#v", name.Messages)
	}
	c, err := history.Load(f.histories("chess-openings.json"))
	if err != nil || len(c.Messages) != 4 {
		t.Fatalf("archive %#v %v", c, err)
	}
	if b, _ := os.ReadFile(f.histories(".chess-openings")); string(b) != "tell me about chess\n" {
		t.Errorf("mode history not renamed: %q", b)
	}
	if f.m.Session("").Len() != 2 || f.sp.Last() != DefaultInstructions {
		t.Errorf("new chat: %d messages, last request %q", f.m.Session("").Len(), f.sp.Last())
	}
	// The instruction-only conversation is archived too, under a unique name.
	f.log.Reset()
	if err := f.m.New(ctx, "", ""); err != nil {
		t.Fatal(err)
	}
	f.logged(t, "Previous conversation saved as: chess-openings (2).json")
	if !history.Exists(f.histories("chess-openings (2).json")) {
		t.Error("unique archive missing")
	}
}

func TestNewFallsBackToTimestamp(t *testing.T) {
	f := newFixture(t, "ok", "answer", "I can't name files", "ok2")
	f.m.Send(context.Background(), "", "q", nil, SendOptions{})
	if err := f.m.New(context.Background(), "", ""); err != nil {
		t.Fatal(err)
	}
	if !history.Exists(f.histories("2024-01-02T03-04-05.json")) {
		t.Error("timestamp archive missing")
	}
	f.logged(t, "Previous conversation saved as: 2024-01-02T03-04-05.json")
}

func TestTooManyCopiesWarns(t *testing.T) {
	f := newFixture(t, "ok", "a", `"same.json"`, "ok2")
	dir := f.m.Paths.Dir(config.Histories)
	os.WriteFile(filepath.Join(dir, "same.json"), nil, 0o644)
	for n := 2; n <= 99; n++ {
		os.WriteFile(filepath.Join(dir, "same ("+itoa(n)+").json"), nil, 0o644)
	}
	f.m.Send(context.Background(), "", "q", nil, SendOptions{})
	if err := f.m.New(context.Background(), "", ""); err != nil {
		t.Fatal(err)
	}
	f.logged(t, "Previous conversation saved as: 2024-01-02T03-04-05.json")
	if !strings.Contains(f.log.String(), tooManyCopies) {
		t.Errorf("warning missing:\n%s", f.log.String())
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

func TestNewWithNameSavesBack(t *testing.T) {
	f := newFixture(t, "ok", "a1", "ok2")
	ctx := context.Background()
	if err := f.m.New(ctx, "", "foo"); err != nil {
		t.Fatal(err)
	}
	f.logged(t, "Starting new chat with AI...", "Current conversation will be saved as: foo")
	if f.m.Settings.String(SaveAsKey) != "foo" {
		t.Errorf("%s = %q", SaveAsKey, f.m.Settings.String(SaveAsKey))
	}
	f.m.Send(ctx, "", "q1", nil, SendOptions{})
	n := len(f.sp.Requests)
	if err := f.m.New(ctx, "", ""); err != nil {
		t.Fatal(err)
	}
	// No naming request: the file name was given.
	if len(f.sp.Requests) != n+1 {
		t.Errorf("%d requests during new, want 1", len(f.sp.Requests)-n)
	}
	f.logged(t, "Previous conversation saved as: foo")
	c, err := history.Load(f.histories("foo"))
	if err != nil || len(c.Messages) != 4 {
		t.Fatalf("archive %#v %v", c, err)
	}
	if v, _ := f.m.Settings.Get(SaveAsKey); v != nil {
		t.Errorf("%s = %v after new", SaveAsKey, v)
	}
}

func TestOldResumesAndSavesBack(t *testing.T) {
	f := newFixture(t, "a2", "ok2")
	ctx := context.Background()
	old := &history.Conversation{}
	old.Append(history.RoleUser, "q1")
	old.Append(history.RoleAssistant, "a1")
	if err := old.Save(f.histories("x.json")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(f.histories(".x"), []byte("q1\n"), 0o644)
	if err := f.m.Old(ctx, "", "nope"); err == nil || err.Error() != "conversation not found: nope" {
		t.Fatalf("missing: %v", err)
	}
	if err := f.m.Old(ctx, "", "x.json"); err != nil {
		t.Fatal(err)
	}
	f.logged(t, "Starting old chat with AI...")
	if u, a, _ := f.m.Recap(""); u != "q1" || a != "a1" {
		t.Errorf("recap %s/%s", u, a)
	}
	for _, s := range []string{".bak", ".old1", ".old"} {
		if !history.Exists(f.histories("x.json" + s)) {
			t.Errorf("missing %s", s)
		}
	}
	if f.m.Exists("") {
		t.Error("live file written before the first message")
	}
	if b, _ := os.ReadFile(f.m.Paths.ModeHistoryFile()); string(b) != "q1\n" {
		t.Errorf("mode history not restored: %q", b)
	}
	if f.m.Settings.String(SaveAsKey) != f.histories("x.json") {
		t.Errorf("%s = %q", SaveAsKey, f.m.Settings.String(SaveAsKey))
	}
	if len(f.sp.Requests) != 0 {
		t.Errorf("resuming sent %d requests", len(f.sp.Requests))
	}
	f.m.Send(ctx, "", "q2", nil, SendOptions{})
	if strings.Contains(f.log.String(), "Resuming") || strings.Contains(f.log.String(), "Initializing") {
		t.Errorf("start logged again: %s", f.log.String())
	}
	if err := f.m.New(ctx, "", ""); err != nil {
		t.Fatal(err)
	}
	f.logged(t, "Previous conversation saved as: x.json")
	c, _ := history.Load(f.histories("x.json"))
	if len(c.Messages) != 4 || c.Messages[3].Content != "a2" {
		t.Errorf("saved back %#v", c.Messages)
	}
}

func TestOldPathForms(t *testing.T) {
	f := newFixture(t)
	c := &history.Conversation{}
	c.Append(history.RoleUser, "q")
	c.Append(history.RoleAssistant, "a")
	abs := filepath.Join(t.TempDir(), "elsewhere")
	c.Save(abs)
	if err := f.m.Old(context.Background(), "", abs); err != nil {
		t.Fatal(err)
	}
	if f.m.Settings.String(SaveAsKey) != abs {
		t.Errorf("abs path setting = %q", f.m.Settings.String(SaveAsKey))
	}
	// Legacy and interim files resume alike.
	os.WriteFile(f.histories("interim"), []byte(`{"system":"s","messages":[{"role":"user","content":"u"},{"role":"assistant","content":"m"}]}`), 0o644)
	if err := f.m.Old(context.Background(), "", "interim"); err != nil {
		t.Fatal(err)
	}
	if u, a, _ := f.m.Recap(""); u != "u" || a != "m" {
		t.Errorf("recap %s/%s", u, a)
	}
}

func TestTemplated(t *testing.T) {
	f := newFixture(t, "ok", "a2")
	ctx := context.Background()
	src := &history.Conversation{}
	src.Append(history.RoleUser, "Speak French")
	src.Append(history.RoleAssistant, "Oui")
	src.Save(f.histories("french"))
	if err := f.m.Templated(ctx, "", "missing"); err == nil || err.Error() != "Template not found" {
		t.Fatalf("missing: %v", err)
	}
	if err := f.m.Templated(ctx, "", "french"); err != nil {
		t.Fatal(err)
	}
	f.logged(t, "Starting template chat with AI...")
	tmpl := filepath.Join(f.m.Paths.Root, config.Templates, "french")
	for _, s := range []string{"", ".bak", ".old1"} {
		if !history.Exists(tmpl + s) {
			t.Errorf("missing template copy %q", s)
		}
	}
	if f.m.Session("").Len() != 2 || f.m.Settings.String(SaveAsKey) != "" {
		t.Errorf("len %d setting %q", f.m.Session("").Len(), f.m.Settings.String(SaveAsKey))
	}
	f.m.Send(ctx, "", "q", nil, SendOptions{})
	// The template is refreshed from histories on every start; a missing
	// source is reported but the template copy still works.
	os.Remove(f.histories("french"))
	f.log.Reset()
	if err := f.m.Templated(ctx, "", "french"); err != nil {
		t.Fatal(err)
	}
	f.logged(t, "Previous conversation saved as: 2024-01-02T03-04-05.json")
	if !strings.Contains(f.log.String(), "Template not found") {
		t.Errorf("missing source not reported:\n%s", f.log.String())
	}
	if f.m.Session("").Len() != 2 {
		t.Errorf("template copy not resumed: %d", f.m.Session("").Len())
	}
}

func TestNewTemplateCopiesLiveFile(t *testing.T) {
	f := newFixture(t, "ok", "Bonjour")
	ctx := context.Background()
	if _, err := f.m.NewTemplate("", "french"); err == nil {
		t.Error("template made from a conversation that was never saved")
	}
	f.m.Send(ctx, "", "Speak French", nil, SendOptions{})
	dest, err := f.m.NewTemplate("", "french")
	if err != nil || dest != filepath.Join(f.m.Paths.Root, config.Templates, "french") {
		t.Fatalf("dest %s %v", dest, err)
	}
	f.logged(t, "Template created: "+dest)
	live, _ := os.ReadFile(f.m.CurrentFile(""))
	tmpl, _ := os.ReadFile(dest)
	if !bytes.Equal(live, tmpl) {
		t.Error("template differs from the live file")
	}
}

func TestUndoAndRecap(t *testing.T) {
	f := newFixture(t, "ok", "a1", "a2")
	ctx := context.Background()
	if _, _, ok := f.m.Recap(""); ok {
		t.Error("recap of nothing")
	}
	if _, _, ok, _ := f.m.Undo(""); ok {
		t.Error("undo of nothing")
	}
	f.m.Send(ctx, "", "q1", nil, SendOptions{})
	f.m.Send(ctx, "", "q2", nil, SendOptions{})
	if u, a, ok := f.m.Recap(""); !ok || u != "q2" || a != "a2" {
		t.Errorf("recap %s/%s/%v", u, a, ok)
	}
	if u, p, ok, err := f.m.Undo(""); !ok || err != nil || u != "q2" || p != "a1" {
		t.Errorf("undo %s/%s/%v %v", u, p, ok, err)
	}
	if c, _ := history.Load(f.m.CurrentFile("")); len(c.Messages) != 4 {
		t.Errorf("undo not saved: %d", len(c.Messages))
	}
	if u, p, ok, _ := f.m.Undo(""); !ok || u != "q1" || p != "ok" {
		t.Errorf("undo %s/%s/%v", u, p, ok)
	}
	if _, _, ok, _ := f.m.Undo(""); ok || f.m.Session("").Len() != 2 {
		t.Error("undo emptied the conversation")
	}
}

func TestCompleteAndReplace(t *testing.T) {
	f := newFixture(t, "stateless")
	ctx := context.Background()
	msgs := []history.Message{{Role: history.RoleUser, Content: "a"}, {Role: history.RoleAssistant, Content: "b"}, {Role: history.RoleAssistant, Content: ""}}
	var out bytes.Buffer
	reply, err := f.m.Complete(ctx, msgs, "c", &out)
	if err != nil || reply != "stateless" || out.String() != "stateless\n\n" {
		t.Fatalf("reply %q out %q err %v", reply, out.String(), err)
	}
	if got := f.sp.Requests[0].Messages; len(got) != 3 || got[2].Content != "c" {
		t.Errorf("request messages = %#v (empty assistant turn not cleaned)", got)
	}
	if f.m.Exists("") || f.m.Session("").Len() != 0 {
		t.Error("Complete touched the session")
	}
	extra := map[string]json.RawMessage{"1": json.RawMessage(`[{"role":"user","parts":[{"text":"side"}]}]`)}
	if err := f.m.Replace("", msgs[:2], extra); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f.m.CurrentFile(""))
	want := `{"0":[{"role":"user","parts":[{"text":"a"}]},{"role":"model","parts":[{"text":"b"}]}],"1":[{"role":"user","parts":[{"text":"side"}]}]}`
	if string(b) != want {
		t.Errorf("file\n%s\nwant\n%s", b, want)
	}
	if f.m.Session("").Len() != 2 {
		t.Errorf("session len %d", f.m.Session("").Len())
	}
}

func TestExtractFilename(t *testing.T) {
	for in, want := range map[string]string{
		`Sure: "chess talk.json"`: "chess talk.json",
		"name: notes.json.":       "notes.json",
		"../../etc/x.json":        "x.json",
		"none":                    "",
		`".hidden.json"`:          ".hidden.json",
		"'quoted'.json":           "quoted.json",
	} {
		if got := ExtractFilename(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}
