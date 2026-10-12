package history

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUndoAndLastExchange(t *testing.T) {
	c := &Conversation{}
	c.Append(RoleUser, "a")
	c.Append(RoleAssistant, "A")
	c.Append(RoleUser, "b")
	c.Append(RoleAssistant, "B")
	if u, a, _ := c.LastExchange(); u != "b" || a != "B" {
		t.Fatalf("last = %s/%s", u, a)
	}
	if u, a, ok := c.Undo(); !ok || u != "a" || a != "A" || len(c.Messages) != 2 {
		t.Fatalf("undo = %s/%s/%v %d", u, a, ok, len(c.Messages))
	}
	c.Undo()
	if _, _, ok := c.Undo(); ok {
		t.Fatal("undo on empty conversation should report nothing")
	}
}

func TestUniquePathAndBackup(t *testing.T) {
	dir := t.TempDir()
	p, _ := UniquePath(dir, "chess.json")
	if filepath.Base(p) != "chess.json" {
		t.Fatalf("p = %s", p)
	}
	c := &Conversation{System: "s"}
	if err := c.Save(p); err != nil {
		t.Fatal(err)
	}
	p2, _ := UniquePath(dir, "chess")
	if filepath.Base(p2) != "chess (2).json" {
		t.Fatalf("p2 = %s", p2)
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
	if err != nil || loaded.System != "s" {
		t.Fatalf("load = %#v %v", loaded, err)
	}
}
