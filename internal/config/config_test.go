package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpand(t *testing.T) {
	p := Paths{Root: t.TempDir()}
	home, _ := os.UserHomeDir()
	for in, want := range map[string]string{
		"~~~/a.json":   filepath.Join(p.Root, Histories, "a.json"),
		"~~/settings":  filepath.Join(p.Root, "settings"),
		"chess.json":   filepath.Join(p.Root, Histories, "chess.json"),
		"./local.json": "./local.json",
		"/abs/x.json":  "/abs/x.json",
		"~/x":          filepath.Join(home, "x"),
	} {
		if in == "/abs/x.json" || in == "~/x" {
			continue // would create directories outside the temp dir
		}
		if got := p.Expand(in, Histories); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Set("provider", "claude")
	s.Set(".hidden", map[string]any{"a": 1})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil || again.String("provider") != "claude" || again.String(".hidden") != `{"a":1}` {
		t.Fatalf("%v %v", again.Snapshot(), err)
	}
	if keys := again.Keys(); len(keys) != 1 || keys[0] != "provider" {
		t.Errorf("keys %v", keys)
	}
}
