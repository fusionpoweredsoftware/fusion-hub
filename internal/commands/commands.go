// Package commands holds the REPL commands and the registry that parses a
// line, honours sticky command mode and dispatches to a command.
package commands

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/chat"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/tts"
)

// ErrNotFound is returned for an unknown command.
var ErrNotFound = errors.New("Command not found.")

// ErrNotStarted is returned by a StartFunc that decided not to start (for
// example "start last" on an empty conversation): nothing is logged, the
// mode does not change and no error is shown, as in the original.
var ErrNotStarted = errors.New("not started")

// Input is a parsed command line (without the command name, unless the
// line was entered in command mode, where the whole line is input).
type Input struct {
	Raw  string
	Args []string
}

// StartFunc starts a command mode. It returns an optional custom
// "... started." message.
type StartFunc func(ctx context.Context, env *Env, args []string) (string, error)

// Command is one REPL command.
type Command struct {
	ID            string
	Name          string
	Description   string
	Examples      string
	CaseSensitive bool
	// Start holds the start modes ("" is the plain "start <id>") for
	// commands that can become the sticky command mode.
	Start map[string]StartFunc
	Run   func(ctx context.Context, env *Env, in Input) (string, error)
}

// Startable reports whether "start <id>" works.
func (c *Command) Startable() bool { return len(c.Start) > 0 }

// State is the REPL's mode state.
type State struct {
	mu       sync.Mutex
	mode     string
	lastMode string
	lockMode string
	stopped  bool
}

func (s *State) Mode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// SetMode makes id the sticky command mode ("" returns to CLI mode).
func (s *State) SetMode(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode != "" && id == "" {
		s.lastMode = s.mode
	}
	s.mode = id
	if id != "" {
		s.lastMode = id
	}
}

// Toggle switches between CLI mode and the last command mode (Ctrl+C).
func (s *State) Toggle() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode != "" {
		s.lastMode, s.mode = s.mode, ""
	} else {
		s.mode = s.lastMode
	}
	return s.mode
}

// Lock pins the REPL to one command mode; Ctrl+C then exits.
func (s *State) Lock(id string) {
	s.mu.Lock()
	s.lockMode, s.mode, s.lastMode = id, id, id
	s.mu.Unlock()
}

func (s *State) Locked() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lockMode
}

func (s *State) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
}

func (s *State) Stopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

// Env is what commands work with.
type Env struct {
	Out       *output.Writer
	Settings  *config.Store
	Paths     config.Paths
	Chat      *chat.Manager
	Speaker   tts.Speaker
	SessionID string
	State     *State
	Registry  *Registry
}

// Registry is the set of commands.
type Registry struct{ cmds map[string]*Command }

// NewRegistry returns a registry with every built-in command.
func NewRegistry() *Registry {
	r := &Registry{cmds: map[string]*Command{}}
	for _, c := range []*Command{help(), exit(), set(), start(), chatWithAI(), say(), appUsingAI(), multiCommand()} {
		r.Add(c)
	}
	return r
}

func (r *Registry) Add(c *Command) { r.cmds[c.ID] = c }

func (r *Registry) Get(id string) *Command { return r.cmds[strings.ToLower(id)] }

// All returns the commands sorted by id.
func (r *Registry) All() []*Command {
	var out []*Command
	for _, c := range r.cmds {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Execute runs a line, routing it to the sticky command mode if one is
// active. Blank lines do nothing.
func (r *Registry) Execute(ctx context.Context, env *Env, line string) (string, error) {
	if mode := env.State.Mode(); mode != "" {
		if strings.TrimSpace(line) == "" {
			return "", nil
		}
		c := r.Get(mode)
		if c == nil {
			return "", ErrNotFound
		}
		return r.run(ctx, env, c, strings.TrimSpace(line))
	}
	return r.Dispatch(ctx, env, line)
}

// Dispatch runs a line as "<command> <input>", ignoring command mode.
func (r *Registry) Dispatch(ctx context.Context, env *Env, line string) (string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	name, rest, _ := strings.Cut(line, " ")
	c := r.Get(name)
	if c == nil {
		return "", ErrNotFound
	}
	if lock := env.State.Locked(); lock != "" && lock != c.ID {
		return "", ErrNotFound
	}
	return r.run(ctx, env, c, strings.TrimSpace(rest))
}

func (r *Registry) run(ctx context.Context, env *Env, c *Command, raw string) (string, error) {
	if !c.CaseSensitive {
		raw = strings.ToLower(raw)
	}
	return c.Run(ctx, env, Input{Raw: raw, Args: Args(raw)})
}

var argPattern = regexp.MustCompile(`(?:[^\s']+|'[^'\\]*(?:\\.[^'\\]*)*')+`)

// Args splits a line on whitespace, keeping 'single quoted' runs together
// (the quotes are removed and \' unescaped).
func Args(s string) []string {
	m := argPattern.FindAllString(strings.TrimSpace(s), -1)
	for i, a := range m {
		if len(a) >= 2 && a[0] == '\'' && a[len(a)-1] == '\'' {
			m[i] = strings.ReplaceAll(a[1:len(a)-1], `\'`, `'`)
		}
	}
	return m
}

// RestAfter returns raw with its first n whitespace-separated tokens removed.
func RestAfter(raw string, n int) string {
	s := strings.TrimSpace(raw)
	for i := 0; i < n && s != ""; i++ {
		idx := strings.IndexAny(s, " \t")
		if idx < 0 {
			return ""
		}
		s = strings.TrimSpace(s[idx:])
	}
	return s
}
