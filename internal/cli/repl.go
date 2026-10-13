// Package cli is the interactive REPL: a prompt showing the sticky command
// mode, separate input histories for CLI mode and command mode, and Ctrl+C
// switching between the two (twice quickly exits).
package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/chzyer/readline"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/commands"
)

// DoubleInterrupt is how close two Ctrl+C presses must be to exit.
const DoubleInterrupt = 350 * time.Millisecond

// REPL runs commands typed at the terminal.
type REPL struct {
	Env      *commands.Env
	Registry *commands.Registry
	// Configure, when set, adjusts the readline configuration before the
	// prompt opens (tests use it to supply their own streams).
	Configure func(*readline.Config)
}

// Run executes initial (if any) and then reads lines until exit.
//
// Ctrl+C ends the current line silently and switches between CLI mode and
// the last command mode; pressed twice within DoubleInterrupt it exits, as
// does Ctrl+C in lock mode. Ctrl+D on a terminal does the same; end of
// input on a non-terminal stdin exits.
func (r *REPL) Run(ctx context.Context, initial string) error {
	if initial != "" {
		r.exec(ctx, initial)
	}
	cfg := promptConfig("> ", r.Env.Paths.CLIHistoryFile())
	if r.Configure != nil {
		r.Configure(cfg)
	}
	rl, err := readline.NewEx(cfg)
	if err != nil {
		return err
	}
	defer rl.Close()
	isTerminal := rl.Config.FuncIsTerminal

	// An empty line before the first prompt, as the original.
	r.Env.Out.Print("")

	var lastInterrupt time.Time
	mode := "\x00" // force the first prompt/history setup
	for !r.Env.State.Stopped() {
		if m := r.Env.State.Mode(); m != mode {
			mode = m
			// CLI mode and command mode keep separate input histories.
			path := r.Env.Paths.CLIHistoryFile()
			if mode != "" {
				path = r.Env.Paths.ModeHistoryFile()
			}
			// A clone keeps the terminal settings but drops the loaded
			// history, so SetConfig opens and loads the new file
			// (SetHistoryPath would leave an empty, unsaved history).
			cfg := rl.Config.Clone()
			cfg.Prompt = mode + "> "
			cfg.HistoryFile = path
			rl.SetConfig(cfg)
		}
		line, err := rl.Readline()
		switch {
		case errors.Is(err, readline.ErrInterrupt), errors.Is(err, io.EOF) && isTerminal():
			// readline has already ended the line (the empty line after
			// the switch); the next prompt follows on a fresh line.
			if r.Env.State.Locked() != "" || time.Since(lastInterrupt) < DoubleInterrupt {
				r.Env.State.Stop()
				continue
			}
			lastInterrupt = time.Now()
			r.Env.State.Toggle()
		case errors.Is(err, io.EOF):
			r.Env.State.Stop()
		case err != nil:
			return err
		default:
			r.exec(ctx, line)
		}
	}
	return nil
}

// promptConfig is the initial readline config. The interrupt and EOF
// prompts are a bare newline so Ctrl+C and Ctrl+D end the line without
// echoing "^C" or "^D".
func promptConfig(prompt, historyFile string) *readline.Config {
	return &readline.Config{
		Prompt:            prompt,
		HistoryFile:       historyFile,
		HistorySearchFold: true,
		InterruptPrompt:   "\n",
		EOFPrompt:         "\n",
	}
}

// exec runs one line; Ctrl+C while it runs cancels it.
func (r *REPL) exec(ctx context.Context, line string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() {
		select {
		case <-sig:
			cancel()
		case <-ctx.Done():
		}
	}()
	if _, err := r.Registry.Execute(ctx, r.Env, line); err != nil {
		if errors.Is(err, context.Canceled) {
			r.Env.Out.Print("\n(interrupted)")
			return
		}
		r.Env.Out.Error("%v", err)
	}
}
