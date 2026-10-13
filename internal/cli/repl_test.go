package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chzyer/readline"
)

// History is saved to and loaded from each mode's file, across switches
// and across separate readline instances (i.e. separate sessions).
func TestHistoryPersists(t *testing.T) {
	dir := t.TempDir()
	cliFile := filepath.Join(dir, ".cli-history")
	modeFile := filepath.Join(dir, ".cmd-mode-history")
	open := func() *readline.Instance {
		cfg := promptConfig("> ", cliFile)
		cfg.Stdin = io.NopCloser(strings.NewReader(""))
		cfg.Stdout, cfg.Stderr = io.Discard, io.Discard
		rl, err := readline.NewEx(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return rl
	}
	switchTo := func(rl *readline.Instance, prompt, file string) {
		cfg := rl.Config.Clone() // as Run does
		cfg.Prompt = prompt
		cfg.HistoryFile = file
		rl.SetConfig(cfg)
	}
	// Instances are not closed: readline's Close races with its own input
	// goroutine. History is written to the file as each line is saved.

	rl := open()
	if err := rl.SaveHistory("help"); err != nil {
		t.Fatal(err)
	}
	switchTo(rl, "chat-with-ai> ", modeFile)
	if err := rl.SaveHistory("hello there"); err != nil {
		t.Fatal(err)
	}
	switchTo(rl, "> ", cliFile)
	if err := rl.SaveHistory("set"); err != nil {
		t.Fatal(err)
	}

	for file, want := range map[string]string{cliFile: "help\nset\n", modeFile: "hello there\n"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != want {
			t.Errorf("%s = %q, want %q", filepath.Base(file), b, want)
		}
	}

	// A new session appends to the existing history instead of replacing it.
	rl = open()
	if err := rl.SaveHistory("exit"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(cliFile); string(b) != "help\nset\nexit\n" {
		t.Errorf("after reopen: %q", b)
	}
}
