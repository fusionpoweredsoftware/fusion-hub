package chat

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/history"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/provider"
)

func newManager(t *testing.T, replies ...string) (*Manager, *provider.Scripted) {
	t.Helper()
	paths := config.Paths{Root: t.TempDir()}
	m := NewManager(paths, config.NewMemory())
	sp := &provider.Scripted{Replies: replies}
	m.Provider = func() (provider.Provider, error) { return sp, nil }
	m.Now = func() time.Time { return time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC) }
	return m, sp
}

func TestSendPersistsAndStreams(t *testing.T) {
	m, sp := newManager(t, "Hello there")
	var out bytes.Buffer
	reply, err := m.Send(context.Background(), "", "hi", &out, SendOptions{})
	if err != nil || reply != "Hello there" || out.String() != "Hello there\n" {
		t.Fatalf("reply %q out %q err %v", reply, out.String(), err)
	}
	if sp.Requests[0].System != DefaultInstructions {
		t.Errorf("system = %q", sp.Requests[0].System)
	}
	c, err := history.Load(m.CurrentFile(""))
	if err != nil || len(c.Messages) != 2 {
		t.Fatalf("saved %#v %v", c, err)
	}
	// A second manager (restart) resumes the same conversation.
	m2 := NewManager(m.Paths, m.Settings)
	if m2.Session("").Len() != 2 {
		t.Error("conversation not resumed")
	}
}

func TestEphemeralAndErrorsLeaveHistory(t *testing.T) {
	m, _ := newManager(t, "x")
	if _, err := m.Send(context.Background(), "s1", "q", nil, SendOptions{Ephemeral: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Send(context.Background(), "s1", "q", nil, SendOptions{}); err == nil {
		t.Fatal("expected error when provider has no replies")
	}
	if m.Session("s1").Len() != 0 {
		t.Errorf("history changed: %d", m.Session("s1").Len())
	}
	if !strings.HasSuffix(m.CurrentFile("a/b"), ".ai-chat-history.a_b.json") {
		t.Errorf("unsafe id in file name: %s", m.CurrentFile("a/b"))
	}
}

func TestNewArchivesWithAIName(t *testing.T) {
	m, sp := newManager(t, "Chess openings are...", `How about "chess-openings.json"?`)
	ctx := context.Background()
	m.Send(ctx, "", "tell me about chess", nil, SendOptions{})
	archived, err := m.New(ctx, "", "")
	if err != nil || filepath.Base(archived) != "chess-openings.json" {
		t.Fatalf("archived %q %v", archived, err)
	}
	if !strings.Contains(sp.Last(), ".JSON filename") {
		t.Errorf("filename prompt = %q", sp.Last())
	}
	if m.Session("").Len() != 0 || history.Exists(m.CurrentFile("")) {
		t.Error("new chat not empty")
	}
	// Empty conversations are not archived.
	if a, _ := m.New(ctx, "", ""); a != "" {
		t.Errorf("archived empty chat: %s", a)
	}
}

func TestNewFallsBackToTimestamp(t *testing.T) {
	m, _ := newManager(t, "answer", "I can't name files")
	m.Send(context.Background(), "", "q", nil, SendOptions{})
	archived, _ := m.New(context.Background(), "", "")
	if filepath.Base(archived) != "2024-01-02T03-04-05.json" {
		t.Errorf("archived %s", archived)
	}
}

func TestOldResumesAndSavesBack(t *testing.T) {
	m, _ := newManager(t, "a1", "no name", "a2")
	ctx := context.Background()
	m.Send(ctx, "", "q1", nil, SendOptions{})
	archived, _ := m.New(ctx, "", "first")
	if archived == "" {
		t.Fatal("first chat not archived")
	}
	// The model gave no file name, so it was archived by timestamp.
	if _, err := m.Old(ctx, "", filepath.Base(archived), false); err != nil {
		t.Fatal(err)
	}
	if u, a, _ := m.Recap(""); u != "q1" || a != "a1" {
		t.Errorf("recap %s/%s", u, a)
	}
	if !history.Exists(archived + ".bak") {
		t.Error("no backup made")
	}
	m.Send(ctx, "", "q2", nil, SendOptions{})
	// "new" now saves back to the file it came from.
	again, _ := m.New(ctx, "", "")
	if again != archived {
		t.Errorf("saved to %s, want %s", again, archived)
	}
	c, _ := history.Load(archived)
	if len(c.Messages) != 4 {
		t.Errorf("saved %d messages", len(c.Messages))
	}
	if _, err := m.Old(ctx, "", "nope", false); err == nil {
		t.Error("missing conversation accepted")
	}
}

func TestUndoAndTemplates(t *testing.T) {
	m, _ := newManager(t, "a1", "a2", "name.json")
	ctx := context.Background()
	m.Send(ctx, "", "q1", nil, SendOptions{})
	m.Send(ctx, "", "q2", nil, SendOptions{})
	if u, a, ok, _ := m.Undo(""); !ok || u != "q1" || a != "a1" {
		t.Fatalf("undo %s/%s/%v", u, a, ok)
	}
	path, err := m.SaveTemplate("", "tutor")
	if err != nil || filepath.Base(filepath.Dir(path)) != config.Templates {
		t.Fatalf("template %s %v", path, err)
	}
	if _, err := m.Old(ctx, "", "tutor", true); err != nil {
		t.Fatal(err)
	}
	if m.Session("").Len() != 2 {
		t.Errorf("template not loaded")
	}
	if _, err := m.Old(ctx, "", "missing", true); err == nil || !strings.Contains(err.Error(), "template not found") {
		t.Errorf("err %v", err)
	}
}

func TestExtractFilename(t *testing.T) {
	for in, want := range map[string]string{`Sure: "chess talk.json"`: "chess talk.json", "name: notes.json.": "notes.json", "../../etc/x.json": "x.json", "none": "", `".hidden.json"`: ""} {
		if got := ExtractFilename(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}
