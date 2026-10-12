// Package chat manages chat sessions: the current conversation per session
// id, archiving it under an AI-suggested name when a new one starts,
// resuming old ones, undo, recap and templates.
package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/history"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/provider"
)

// DefaultInstructions open every new conversation unless the
// "instructions" setting overrides them.
const DefaultInstructions = "Only speak in English unless instructed otherwise. Try your best to be a helpful assistant."

const filenamePrompt = `Come up with a .JSON filename to describe the nature of this conversation using less than 32 characters in double quotes (")`

// saveAsKey remembers, for the REPL session, which file an "old"
// conversation came from so "new" saves it back there.
const saveAsKey = ".current-input-history-file-path"

// Manager owns every chat session.
type Manager struct {
	Paths    config.Paths
	Settings *config.Store
	// Provider returns the model to talk to; it is resolved on every
	// message so "set provider as ..." takes effect immediately.
	Provider func() (provider.Provider, error)
	Now      func() time.Time

	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager(paths config.Paths, settings *config.Store) *Manager {
	return &Manager{
		Paths:    paths,
		Settings: settings,
		Provider: func() (provider.Provider, error) {
			cfg, err := provider.FromSettings(settings)
			if err != nil {
				return nil, err
			}
			return provider.New(cfg)
		},
		Now:      time.Now,
		sessions: map[string]*Session{},
	}
}

// Session is one live conversation.
type Session struct {
	ID   string
	Path string
	mu   sync.Mutex
	conv *history.Conversation
	// saveAs is where "new" archives this conversation ("" = AI names it).
	saveAs string
}

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// CurrentFile is the file holding session id's live conversation.
func (m *Manager) CurrentFile(id string) string {
	dir := m.Paths.Dir(config.Histories)
	if id == "" || id == "0" {
		return filepath.Join(dir, ".ai-chat-history.json")
	}
	return filepath.Join(dir, ".ai-chat-history."+unsafeID.ReplaceAllString(id, "_")+".json")
}

// Session returns (loading if needed) the session for id.
func (m *Manager) Session(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		return s
	}
	s := &Session{ID: id, Path: m.CurrentFile(id)}
	if c, err := history.Load(s.Path); err == nil {
		s.conv = c
	} else {
		s.conv = m.fresh()
	}
	if id == "" {
		s.saveAs = m.Settings.String(saveAsKey)
	}
	m.sessions[id] = s
	return s
}

func (m *Manager) fresh() *history.Conversation {
	sys := m.Settings.String("instructions")
	if sys == "" {
		sys = DefaultInstructions
	}
	return &history.Conversation{System: sys}
}

// Exists reports whether session id has a saved conversation.
func (m *Manager) Exists(id string) bool { return history.Exists(m.CurrentFile(id)) }

// Snapshot returns a copy of the session's conversation.
func (s *Session) Snapshot() *history.Conversation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conv.Clone()
}

// Len is the number of messages in the conversation.
func (s *Session) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conv.Messages)
}

// Truncate drops messages beyond n and persists.
func (s *Session) Truncate(n int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conv.Truncate(max(n, 0))
	return s.conv.Save(s.Path)
}

// SendOptions tune a single message.
type SendOptions struct {
	// Ephemeral leaves the conversation as it was after the reply.
	Ephemeral bool
}

// Send adds text to the conversation, streams the reply to out and
// persists the conversation. A failed request leaves history unchanged.
func (m *Manager) Send(ctx context.Context, id, text string, out io.Writer, opts SendOptions) (string, error) {
	p, err := m.Provider()
	if err != nil {
		return "", err
	}
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()

	req := provider.Request{System: s.conv.System, Messages: append(append([]history.Message{}, s.conv.Messages...), history.Message{Role: history.RoleUser, Content: text})}
	reply, err := p.Stream(ctx, req, func(d string) {
		if out != nil {
			_, _ = io.WriteString(out, d)
		}
	})
	if out != nil && reply != "" && !strings.HasSuffix(reply, "\n") {
		_, _ = io.WriteString(out, "\n")
	}
	if err != nil {
		return reply, err
	}
	if strings.TrimSpace(reply) == "" {
		return "", errors.New("no response from AI")
	}
	if opts.Ephemeral {
		return reply, nil
	}
	s.conv.Append(history.RoleUser, text)
	s.conv.Append(history.RoleAssistant, reply)
	return reply, s.conv.Save(s.Path)
}

var jsonName = regexp.MustCompile(`"([^"]+\.json)"|(\S+\.json)`)

// ExtractFilename pulls a .json filename out of a model reply.
func ExtractFilename(s string) string {
	mm := jsonName.FindStringSubmatch(s)
	if mm == nil {
		return ""
	}
	name := mm[1]
	if name == "" {
		name = mm[2]
	}
	name = strings.Trim(name, "\"'`")
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || strings.HasPrefix(name, ".") {
		return ""
	}
	return name
}

// archive saves the session's conversation under its saveAs path or an
// AI-suggested name, returning where it went ("" if there was nothing).
func (m *Manager) archive(ctx context.Context, s *Session) (string, error) {
	if len(s.conv.Messages) == 0 {
		return "", nil
	}
	dir := m.Paths.Dir(config.Histories)
	dest := s.saveAs
	if dest == "" {
		name := ""
		if p, err := m.Provider(); err == nil {
			req := provider.Request{System: s.conv.System, Messages: append(append([]history.Message{}, s.conv.Messages...), history.Message{Role: history.RoleUser, Content: filenamePrompt}), MaxTokens: 200}
			if reply, err := p.Stream(ctx, req, nil); err == nil {
				name = ExtractFilename(reply)
			}
		}
		if name == "" {
			name = history.TimestampName(m.Now())
		}
		var err error
		if dest, err = history.UniquePath(dir, name); err != nil {
			dest = filepath.Join(dir, history.TimestampName(m.Now()))
		}
	}
	if err := s.conv.Save(dest); err != nil {
		return "", err
	}
	if s.ID == "" {
		if mode := m.Paths.ModeHistoryFile(); history.Exists(mode) {
			_ = os.Rename(mode, history.Hidden(dest))
		}
	}
	return dest, nil
}

func (m *Manager) setSaveAs(s *Session, path string) error {
	s.saveAs = path
	if s.ID != "" {
		return nil
	}
	if path == "" {
		m.Settings.Delete(saveAsKey)
	} else {
		m.Settings.Set(saveAsKey, path)
	}
	return m.Settings.Save()
}

// New archives the current conversation and starts a fresh one. saveAs,
// when given, is where the new conversation will be archived. It returns
// the archive path of the previous conversation ("" if it was empty).
func (m *Manager) New(ctx context.Context, id, saveAs string) (string, error) {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	archived, err := m.archive(ctx, s)
	if err != nil {
		return "", err
	}
	s.conv = m.fresh()
	_ = os.Remove(s.Path)
	if saveAs != "" {
		saveAs = m.Paths.Expand(withJSON(saveAs), config.Histories)
	}
	return archived, m.setSaveAs(s, saveAs)
}

// Old archives the current conversation and resumes file. With template
// set, file is looked up in the templates directory and the resumed copy
// will be archived under a new name rather than overwriting the template.
func (m *Manager) Old(ctx context.Context, id, file string, template bool) (string, error) {
	dir := config.Histories
	if template {
		dir = config.Templates
	}
	src := m.Paths.Expand(withJSON(file), dir)
	conv, err := history.Load(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if template {
				return "", fmt.Errorf("template not found: %s", file)
			}
			return "", fmt.Errorf("conversation not found: %s", file)
		}
		return "", err
	}
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	archived, err := m.archive(ctx, s)
	if err != nil {
		return "", err
	}
	if !template {
		if err := history.Backup(src); err != nil {
			return "", err
		}
	}
	s.conv = conv
	if err := s.conv.Save(s.Path); err != nil {
		return "", err
	}
	if s.ID == "" {
		if hidden := history.Hidden(src); history.Exists(hidden) {
			_ = history.Copy(hidden, m.Paths.ModeHistoryFile())
		}
	}
	saveAs := src
	if template {
		saveAs = ""
	}
	return archived, m.setSaveAs(s, saveAs)
}

// Undo removes the last exchange and returns the one before it.
func (m *Manager) Undo(id string) (user, assistant string, ok bool, err error) {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	user, assistant, ok = s.conv.Undo()
	return user, assistant, ok, s.conv.Save(s.Path)
}

// Recap returns the last exchange.
func (m *Manager) Recap(id string) (user, assistant string, ok bool) {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conv.LastExchange()
}

// SaveTemplate copies the current conversation into the templates dir.
func (m *Manager) SaveTemplate(id, name string) (string, error) {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	dest := m.Paths.Expand(withJSON(name), config.Templates)
	return dest, s.conv.Save(dest)
}

func withJSON(name string) string {
	if strings.HasSuffix(strings.ToLower(name), ".json") {
		return name
	}
	return name + ".json"
}
