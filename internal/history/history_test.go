package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyFormatRoundTrip(t *testing.T) {
	legacy := `{"0":[{"role":"user","parts":[{"text":"Only speak in English."}]},{"role":"model","parts":[{"text":"Sure <b>&</b> ok"}]}],"1":[{"role":"user","parts":[{"text":"side"}]}]}`
	path := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Messages) != 2 || c.Messages[0].Role != RoleUser || c.Messages[1].Role != RoleAssistant || c.Messages[1].Content != "Sure <b>&</b> ok" {
		t.Fatalf("messages = %#v", c.Messages)
	}
	if string(c.Extra["1"]) != `[{"role":"user","parts":[{"text":"side"}]}]` {
		t.Errorf("extra channel = %s", c.Extra["1"])
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := c.Save(out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if string(b) != legacy {
		t.Errorf("saved\n%s\nwant\n%s", b, legacy)
	}
	st, _ := os.Stat(out)
	if st.Mode().Perm() != 0o644 {
		t.Errorf("mode = %o", st.Mode().Perm())
	}
	// An empty conversation is {"0":[]}.
	b, _ = (&Conversation{}).Bytes()
	if string(b) != `{"0":[]}` {
		t.Errorf("empty = %s", b)
	}
}

func TestLoadInterimGoFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go.json")
	os.WriteFile(path, []byte(`{"system":"be brief","messages":[{"role":"user","content":"a"},{"role":"assistant","content":"A"}]}`), 0o644)
	c, err := Load(path)
	if err != nil || len(c.Messages) != 2 || c.Messages[1].Role != RoleAssistant || c.Messages[1].Content != "A" || c.Extra != nil {
		t.Fatalf("load = %#v %v", c, err)
	}
	if _, err := Parse([]byte(`not json`)); err == nil {
		t.Error("malformed file accepted")
	}
	if c, err := Parse(nil); err != nil || len(c.Messages) != 0 {
		t.Errorf("empty file: %#v %v", c, err)
	}
}

func TestUndoRecapAndClean(t *testing.T) {
	c := &Conversation{}
	c.Append(RoleUser, "a")
	c.Append("model", "A")
	c.Append(RoleUser, "b")
	c.Append(RoleAssistant, "B")
	if u, r, ok := c.LastExchange(); !ok || u != "b" || r != "B" {
		t.Fatalf("last = %s/%s/%v", u, r, ok)
	}
	// Undo shows the undone input and the reply before it.
	if u, p, ok := c.Undo(); !ok || u != "b" || p != "A" || len(c.Messages) != 2 {
		t.Fatalf("undo = %s/%s/%v %d", u, p, ok, len(c.Messages))
	}
	if _, _, ok := c.Undo(); ok || len(c.Messages) != 2 {
		t.Fatal("undo must not empty the conversation")
	}
	if c.Messages[1].Role != RoleAssistant {
		t.Errorf("model role not converted: %s", c.Messages[1].Role)
	}
	c.Append(RoleAssistant, "")
	c.Append(RoleAssistant, "")
	c.Clean()
	if len(c.Messages) != 2 {
		t.Errorf("clean left %d messages", len(c.Messages))
	}
	if _, _, ok := (&Conversation{}).LastExchange(); ok {
		t.Error("recap of empty conversation")
	}
}

func TestUniquePathAndBackup(t *testing.T) {
	dir := t.TempDir()
	p, _ := UniquePath(dir, "chess.json")
	if filepath.Base(p) != "chess.json" {
		t.Fatalf("p = %s", p)
	}
	c := &Conversation{}
	c.Append(RoleUser, "x")
	if err := c.Save(p); err != nil {
		t.Fatal(err)
	}
	p2, _ := UniquePath(dir, "chess.json")
	if filepath.Base(p2) != "chess (2).json" {
		t.Fatalf("p2 = %s", p2)
	}
	// Names are verbatim: no .json is appended.
	os.WriteFile(filepath.Join(dir, "plain"), nil, 0o644)
	if p3, _ := UniquePath(dir, "plain"); filepath.Base(p3) != "plain (2).json" {
		t.Errorf("p3 = %s", p3)
	}
	for i := 0; i < 2; i++ {
		if err := Backup(p); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{".bak", ".old1", ".old2"} {
		if !Exists(p + f) {
			t.Errorf("missing %s", f)
		}
	}
	if b, _ := os.ReadFile(p + ".old"); string(b) != "3" {
		t.Errorf("counter = %s", b)
	}
	loaded, err := Load(p)
	if err != nil || len(loaded.Messages) != 1 {
		t.Fatalf("load = %#v %v", loaded, err)
	}
}

func TestTooManyCopies(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "same.json"), nil, 0o644)
	for n := 2; n <= 99; n++ {
		os.WriteFile(filepath.Join(dir, "same ("+itoa(n)+").json"), nil, 0o644)
	}
	if _, err := UniquePath(dir, "same.json"); err != ErrTooManyCopies {
		t.Errorf("err = %v", err)
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

func TestNames(t *testing.T) {
	if got := TimestampName(time.Date(2024, 1, 28, 6, 11, 28, 0, time.UTC)); got != "2024-01-28T06-11-28.json" {
		t.Errorf("timestamp = %s", got)
	}
	if got := Hidden("/h/histories/chess.json"); got != "/h/histories/.chess" {
		t.Errorf("hidden = %s", got)
	}
	if got := Hidden("/h/histories/foo"); got != "/h/histories/.foo" {
		t.Errorf("hidden = %s", got)
	}
}
