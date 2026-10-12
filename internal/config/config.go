// Package config owns ~/.fusion-hub: the directory layout and the JSON
// settings file that the "set" command edits.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Context directories under the root.
const (
	Histories = "histories"
	Templates = "templates"
	TTS       = "tts"
	Actors    = "actors"
	Apps      = "apps"
)

// Paths resolves files under the fusion-hub root directory.
type Paths struct{ Root string }

// DefaultPaths uses $FUSION_HUB_HOME, falling back to ~/.fusion-hub.
func DefaultPaths() (Paths, error) {
	if dir := os.Getenv("FUSION_HUB_HOME"); dir != "" {
		return Paths{Root: dir}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	return Paths{Root: filepath.Join(home, ".fusion-hub")}, nil
}

func (p Paths) SettingsFile() string    { return filepath.Join(p.Root, "settings.json") }
func (p Paths) CLIHistoryFile() string  { return filepath.Join(p.Root, ".cli-history") }
func (p Paths) ModeHistoryFile() string { return filepath.Join(p.Root, ".cmd-mode-history") }

// Dir returns (and creates) a context directory under the root.
func (p Paths) Dir(context string) string {
	dir := filepath.Join(p.Root, context)
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// Expand resolves a user-supplied path the way fusion-hub always has:
//
//	~~~/x  -> <root>/<context>/x
//	~~/x   -> <root>/x
//	~/x    -> $HOME/x
//	name   -> <root>/<context>/name   (bare names live in the context dir)
//	./x, ../x, /x are used as given.
//
// The parent directory is created so callers can write straight away.
func (p Paths) Expand(path, context string) string {
	var out string
	switch {
	case strings.HasPrefix(path, "~~~"):
		out = filepath.Join(p.Root, context, strings.TrimPrefix(path, "~~~"))
	case strings.HasPrefix(path, "~~"):
		out = filepath.Join(p.Root, strings.TrimPrefix(path, "~~"))
	case strings.HasPrefix(path, "~"):
		home, _ := os.UserHomeDir()
		out = filepath.Join(home, strings.TrimPrefix(path, "~"))
	case filepath.IsAbs(path) || strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../"):
		out = path
	default:
		out = filepath.Join(p.Root, context, path)
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o755)
	return out
}

// Store is the persisted settings map. Keys starting with "." are internal
// state and hidden from the plain "set" listing.
type Store struct {
	mu   sync.RWMutex
	path string
	data map[string]any
}

// Load reads the settings file, returning an empty store if it is missing.
func Load(path string) (*Store, error) {
	s := &Store{path: path, data: map[string]any{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("settings %s: %w", path, err)
	}
	return s, nil
}

// NewMemory returns a store that is never written to disk.
func NewMemory() *Store { return &Store{data: map[string]any{}} }

func (s *Store) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

// String returns the value for key as a string ("" when unset).
func (s *Store) String(key string) string {
	v, ok := s.Get(key)
	if !ok || v == nil {
		return ""
	}
	if str, ok := v.(string); ok {
		return str
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (s *Store) Set(key string, v any) {
	s.mu.Lock()
	s.data[key] = v
	s.mu.Unlock()
}

func (s *Store) Delete(key string) {
	s.mu.Lock()
	delete(s.data, key)
	s.mu.Unlock()
}

// Merge copies every key of m into the store.
func (s *Store) Merge(m map[string]any) {
	s.mu.Lock()
	for k, v := range m {
		s.data[k] = v
	}
	s.mu.Unlock()
}

// Replace swaps the whole settings map.
func (s *Store) Replace(m map[string]any) {
	s.mu.Lock()
	s.data = m
	if s.data == nil {
		s.data = map[string]any{}
	}
	s.mu.Unlock()
}

// Snapshot returns a deep copy of the settings.
func (s *Store) Snapshot() map[string]any {
	s.mu.RLock()
	b, _ := json.Marshal(s.data)
	s.mu.RUnlock()
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

// Keys returns the public (non-dot) keys in sorted order.
func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var keys []string
	for k := range s.data {
		if !strings.HasPrefix(k, ".") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// Save writes the settings atomically. A memory store is a no-op.
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}
	s.mu.RLock()
	b, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
