package cli

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/chzyer/readline"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/chat"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/commands"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/tts"
)

// newREPL wires a REPL to a scripted stdin that is not a terminal.
func newREPL(t *testing.T, stdin string) (*REPL, *bytes.Buffer, *tts.Recorder) {
	t.Helper()
	paths := config.Paths{Root: t.TempDir()}
	settings, err := config.Load(paths.SettingsFile())
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	rec := &tts.Recorder{}
	reg := commands.NewRegistry()
	env := &commands.Env{
		Out: output.New(out), Settings: settings, Paths: paths,
		Chat: chat.NewManager(paths, settings), Speaker: rec, State: &commands.State{}, Registry: reg,
	}
	r := &REPL{Env: env, Registry: reg, Configure: func(cfg *readline.Config) {
		cfg.Stdin = io.NopCloser(strings.NewReader(stdin))
		cfg.Stdout, cfg.Stderr = io.Discard, io.Discard
		cfg.FuncIsTerminal = func() bool { return false }
	}}
	return r, out, rec
}

// Run executes the initial command, prints an empty line before the first
// prompt, runs each line (errors as plain timestamped lines) and exits at
// the end of a non-terminal stdin.
func TestRunPrintsBlankLineAndExitsAtEOF(t *testing.T) {
	r, out, rec := newREPL(t, "nope\nsay hi\n")
	if err := r.Run(context.Background(), "say first"); err != nil {
		t.Fatal(err)
	}
	if !r.Env.State.Stopped() {
		t.Error("EOF did not stop the REPL")
	}
	if !reflect.DeepEqual(rec.Said, []string{"first", "hi"}) {
		t.Errorf("said %q", rec.Said)
	}
	lines := strings.Split(out.String(), "\n")
	if len(lines) != 3 || lines[0] != "" || !strings.HasSuffix(lines[1], "] - Command not found.") || strings.Contains(lines[1], "ERROR") {
		t.Errorf("output %q", out.String())
	}
}

// The interrupt and EOF prompts are bare newlines, so Ctrl+C and Ctrl+D
// end the line without echoing "^C"/"^D".
func TestPromptConfigIsSilent(t *testing.T) {
	cfg := promptConfig("> ", "")
	if cfg.InterruptPrompt != "\n" || cfg.EOFPrompt != "\n" {
		t.Errorf("prompts %q %q", cfg.InterruptPrompt, cfg.EOFPrompt)
	}
	if err := cfg.Init(); err != nil {
		t.Fatal(err)
	}
	if cfg.InterruptPrompt != "" || cfg.EOFPrompt != "" {
		t.Errorf("after Init: %q %q", cfg.InterruptPrompt, cfg.EOFPrompt)
	}
}
