package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPaths(t *testing.T) {
	t.Setenv("FUSION_HUB_HOME", "")
	p, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if p.Root != filepath.Join(home, ".fusion-hub", "fusion-hub") {
		t.Errorf("root = %s", p.Root)
	}
	if p.SettingsFile() != filepath.Join(p.Root, ".settings") || p.CLIHistoryFile() != filepath.Join(p.Root, ".cli-history") || p.ModeHistoryFile() != filepath.Join(p.Root, ".cmd-mode-history") {
		t.Errorf("files: %s %s %s", p.SettingsFile(), p.CLIHistoryFile(), p.ModeHistoryFile())
	}
	t.Setenv("FUSION_HUB_HOME", "/tmp/x")
	if p, _ := DefaultPaths(); p.Root != "/tmp/x" {
		t.Errorf("override root = %s", p.Root)
	}
}

func TestExpand(t *testing.T) {
	p := Paths{Root: t.TempDir()}
	for in, want := range map[string]string{
		"~~~/a.json":   filepath.Join(p.Root, Histories, "a.json"),
		"~~/settings":  filepath.Join(p.Root, "settings"),
		"chess.json":   filepath.Join(p.Root, Histories, "chess.json"),
		"./local.json": "./local.json",
		".hidden.json": ".hidden.json",
	} {
		if got := p.Expand(in, Histories); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
	if got := p.Expand("t", Templates); got != filepath.Join(p.Root, Templates, "t") {
		t.Errorf("template = %s", got)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".settings")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Set("provider", "claude")
	s.Set(".hidden", map[string]any{"a": 1})
	s.Set("current-input-history-file-path", nil)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.ContainsAny(string(b), "\n ") {
		t.Errorf("settings not compact: %q", b)
	}
	again, err := Load(path)
	if err != nil || again.String("provider") != "claude" || again.String(".hidden") != `{"a":1}` || again.String("current-input-history-file-path") != "" {
		t.Fatalf("%v %v", again.Snapshot(), err)
	}
	if v, ok := again.Get("current-input-history-file-path"); !ok || v != nil {
		t.Errorf("null value lost: %v %v", v, ok)
	}
	if keys := again.Keys(); len(keys) != 2 || keys[0] != "current-input-history-file-path" || keys[1] != "provider" {
		t.Errorf("keys %v", keys)
	}
}

func TestLoadMigratesSettingsJSON(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "fusion-hub")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(parent, "settings.json"), []byte(`{"model": "old"}`), 0o600)
	path := filepath.Join(root, ".settings")
	s, err := Load(path)
	if err != nil || s.String("model") != "old" {
		t.Fatalf("parent settings.json not read: %v %v", s.Snapshot(), err)
	}
	// The root's settings.json wins over the parent's.
	os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"model": "newer"}`), 0o600)
	if s, _ := Load(path); s.String("model") != "newer" {
		t.Errorf("root settings.json not preferred: %v", s.Snapshot())
	}
	// Once .settings exists it is the only file read.
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if s, _ := Load(path); s.String("model") != "old" {
		t.Errorf(".settings not preferred: %v", s.Snapshot())
	}
	if _, err := os.Stat(path); err != nil {
		t.Error(".settings not written")
	}
}
