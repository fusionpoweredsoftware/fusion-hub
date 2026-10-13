// Package chat manages chat sessions the way the original fusion-hub did:
// the current conversation per session id, the instruction exchange that
// opens every conversation, archiving under an AI-suggested name when a
// new one starts, resuming old ones, undo, recap and templates.
package chat

import (
	"context"
	"encoding/json"
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
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/provider"
)

// DefaultInstructions are sent as the first user turn of every new
// conversation unless the "instructions" setting overrides them.
const DefaultInstructions = "Only speak in English unless instructed otherwise. Try your best to be a helpful assistant."

const filenamePrompt = `Come up with a .JSON filename to describe the nature of this conversation using less than 32 characters in double quotes (")`

// SaveAsKey is the setting naming the file the current conversation is
// archived to by the next "start new" ("" or null: the AI names it).
const SaveAsKey = "current-input-history-file-path"

// NoResponse is printed in place of an empty reply.
const NoResponse = "(No response)"

// BusyText replaces a provider's HTTP 429 error.
const BusyText = "The AI is currently busy. Try again later."

const tooManyCopies = "WARNING: The AI is suggesting the same filename, but 99 copies were found. This is excessive; reverting to timestamps."

// Console receives the chat's timestamped status lines.
type Console interface {
	Log(format string, a ...any)
	Warn(format string, a ...any)
	Error(format string, a ...any)
}

// Manager owns every chat session.
type Manager struct {
	Paths    config.Paths
	Settings *config.Store
	// Provider returns the model to talk to; it is resolved on every
	// message so "set provider as ..." takes effect immediately.
	Provider func() (provider.Provider, error)
	// Console gets the status lines ("Initializing chat with AI...").
	Console Console
	Now     func() time.Time

	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager(paths config.Paths, settings *config.Store) *Manager {
	if _, ok := settings.Get(SaveAsKey); !ok {
		settings.Set(SaveAsKey, nil)
	}
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
		Console:  output.New(os.Stdout),
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
	// started is set once the session has been initialized in this
	// process ("Initializing"/"Resuming" is logged once).
	started bool
}

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// CurrentFile is the file holding session id's live conversation:
// histories/.ai-chat-history.json, or .ai-chat-history.json.<id>.
func (m *Manager) CurrentFile(id string) string {
	dir := m.Paths.Dir(config.Histories)
	if id == "" || id == "0" {
		return filepath.Join(dir, ".ai-chat-history.json")
	}
	return filepath.Join(dir, ".ai-chat-history.json."+unsafeID.ReplaceAllString(id, "_"))
}

// Session returns (loading if needed) the session for id.
func (m *Manager) Session(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		return s
	}
	s := &Session{ID: id, Path: m.CurrentFile(id), conv: &history.Conversation{}}
	if c, err := history.Load(s.Path); err == nil {
		s.conv = c
	}
	m.sessions[id] = s
	return s
}

// Exists reports whether session id has a saved conversation.
func (m *Manager) Exists(id string) bool { return history.Exists(m.CurrentFile(id)) }

func (m *Manager) console() Console {
	if m.Console == nil {
		return output.Discard()
	}
	return m.Console
}

func (m *Manager) instructions() string {
	if s := m.Settings.String("instructions"); s != "" {
		return s
	}
	return DefaultInstructions
}

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

// Start readies session id as the original's start() did: the first time
// in this process it logs "Initializing chat with AI..." (no saved
// conversation) or "Resuming chat with AI...", and an empty conversation
// gets the instruction exchange (sent silently and persisted).
func (m *Manager) Start(ctx context.Context, id string) error {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	return m.start(ctx, s)
}

func (m *Manager) start(ctx context.Context, s *Session) error {
	if s.started {
		return nil
	}
	action := "Initializing"
	if history.Exists(s.Path) {
		action = "Resuming"
	}
	return m.initSession(ctx, s, action)
}

// initSession logs "<action> chat with AI..." and opens an empty
// conversation with the instruction turn.
func (m *Manager) initSession(ctx context.Context, s *Session, action string) error {
	m.console().Log("%s chat with AI...", action)
	if len(s.conv.Messages) == 0 {
		reply, err := m.chat(ctx, s, m.instructions(), nil, SendOptions{})
		if err != nil {
			return err
		}
		if reply == "" {
			return nil
		}
	}
	s.started = true
	return nil
}

// SendOptions tune a single message.
type SendOptions struct {
	// Ephemeral leaves the conversation as it was after the reply.
	Ephemeral bool
}

// Send starts the session if needed, adds text to the conversation,
// streams the reply to out (followed by a blank line) and persists the
// conversation. An empty reply prints "(No response)" and leaves history
// unchanged; a failed request returns the error, which the caller logs.
func (m *Manager) Send(ctx context.Context, id, text string, out io.Writer, opts SendOptions) (string, error) {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := m.start(ctx, s); err != nil {
		return "", err
	}
	return m.chat(ctx, s, text, out, opts)
}

func (m *Manager) chat(ctx context.Context, s *Session, text string, out io.Writer, opts SendOptions) (string, error) {
	s.conv.Clean()
	reply, sent, err := m.completeWith(ctx, s.conv.Messages, text, out)
	if err != nil || reply == "" || opts.Ephemeral {
		return reply, err
	}
	s.conv.Append(history.RoleUser, sent)
	s.conv.Append(history.RoleAssistant, reply)
	return reply, s.conv.Save(s.Path)
}

var bang = regexp.MustCompile(`[^ ]!`)

// Complete is a stateless send: msgs plus text as the new user turn go to
// the provider, the reply streams to out (when non-nil) followed by "\n\n".
// Trailing empty assistant turns are dropped first. A failed request is
// retried once; a second failure is returned (HTTP 429 as BusyText). An
// empty reply to an input matching /[^ ]!/ is retried once with the first
// "!" replaced by " !"; otherwise "(No response)" is written and "" is
// returned with no error. No session or file is touched.
func (m *Manager) Complete(ctx context.Context, msgs []history.Message, text string, out io.Writer) (string, error) {
	reply, _, err := m.completeWith(ctx, msgs, text, out)
	return reply, err
}

// completeWith is Complete that also returns the user text finally sent.
func (m *Manager) completeWith(ctx context.Context, msgs []history.Message, text string, out io.Writer) (reply, sent string, err error) {
	p, err := m.Provider()
	if err != nil {
		return "", text, err
	}
	conv := &history.Conversation{Messages: append([]history.Message{}, msgs...)}
	conv.Clean()
	return m.complete(ctx, p, conv.Messages, text, out, false)
}

func (m *Manager) complete(ctx context.Context, p provider.Provider, msgs []history.Message, text string, out io.Writer, secondAttempt bool) (reply, sent string, err error) {
	req := provider.Request{Messages: append(append([]history.Message{}, msgs...), history.Message{Role: history.RoleUser, Content: text})}
	reply, err = m.stream(ctx, p, req, out)
	if err != nil {
		return "", text, err
	}
	sent = text
	if reply == "" {
		if !secondAttempt && bang.MatchString(text) {
			// The original retried through chat(), which wrote its own
			// blank line before this one.
			reply, sent, err = m.complete(ctx, p, msgs, strings.Replace(text, "!", " !", 1), out, true)
			if err != nil {
				return "", sent, err
			}
		} else {
			write(out, NoResponse)
		}
	}
	write(out, "\n\n")
	return reply, sent, nil
}

// stream runs one request, retrying once on a provider error.
func (m *Manager) stream(ctx context.Context, p provider.Provider, req provider.Request, out io.Writer) (string, error) {
	onDelta := func(d string) { write(out, d) }
	reply, err := p.Stream(ctx, req, onDelta)
	if err == nil {
		return reply, nil
	}
	if ctx.Err() != nil {
		return reply, err
	}
	if reply, err = p.Stream(ctx, req, onDelta); err == nil {
		return reply, nil
	}
	if provider.IsRateLimited(err) {
		return "", errors.New(BusyText)
	}
	return "", err
}

func write(out io.Writer, s string) {
	if out != nil && s != "" {
		_, _ = io.WriteString(out, s)
	}
}

var jsonName = regexp.MustCompile(`"([^"]+\.json)"|(\S+\.json)`)

// ExtractFilename pulls a .json filename out of a model reply: the first
// quoted or bare word ending in .json, with every quote character removed
// and directories stripped.
func ExtractFilename(s string) string {
	mm := jsonName.FindStringSubmatch(s)
	if mm == nil {
		return ""
	}
	name := mm[1]
	if name == "" {
		name = mm[2]
	}
	name = strings.NewReplacer(`"`, "", "'", "", "`", "").Replace(name)
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" {
		return ""
	}
	return name
}

// archive moves the session's live file into the histories directory,
// under the SaveAsKey name or one the model suggests, logging the steps
// the original did.
func (m *Manager) archive(ctx context.Context, s *Session) error {
	dir := m.Paths.Dir(config.Histories)
	con := m.console()
	con.Log("Finalizing previous chat with AI...")
	var dest string
	if saveAs := m.Settings.String(SaveAsKey); saveAs != "" {
		dest = filepath.Join(dir, filepath.Base(saveAs))
	} else {
		ts := history.TimestampName(m.Now())
		name := ""
		if reply, err := m.Complete(ctx, s.conv.Messages, filenamePrompt, nil); err == nil {
			name = ExtractFilename(reply)
		}
		if name == "" {
			name = ts
		}
		var err error
		if dest, err = history.UniquePath(dir, name); err != nil {
			con.Warn("%s", tooManyCopies)
			dest = filepath.Join(dir, ts)
		}
	}
	con.Log("Previous conversation saved as: %s", filepath.Base(dest))
	if mode := m.Paths.ModeHistoryFile(); history.Exists(mode) {
		_ = os.Rename(mode, history.Hidden(dest))
	}
	return os.Rename(s.Path, dest)
}

// New archives the current conversation (when its file exists) and starts
// a fresh one with the instruction exchange. name, when given, is stored
// verbatim in SaveAsKey: the next New archives to histories/<base name>.
func (m *Manager) New(ctx context.Context, id, name string) error {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if history.Exists(s.Path) {
		if err := m.archive(ctx, s); err != nil {
			return err
		}
	}
	s.conv = &history.Conversation{}
	s.started = false
	var saveAs any
	if name != "" {
		saveAs = name
	}
	if err := m.setSaveAs(saveAs); err != nil {
		return err
	}
	if err := m.initSession(ctx, s, "Starting new"); err != nil {
		return err
	}
	if name != "" {
		m.console().Log("Current conversation will be saved as: %s", filepath.Base(name))
	}
	return nil
}

func (m *Manager) setSaveAs(v any) error {
	m.Settings.Set(SaveAsKey, v)
	return m.Settings.Save()
}

// resolve turns a conversation argument into a path: histories/<f> unless
// f starts with ".", "/" or "~" (then expanded as usual).
func (m *Manager) resolve(file string) string {
	if !strings.HasPrefix(file, ".") && !strings.HasPrefix(file, "/") && !strings.HasPrefix(file, "~") {
		file = "~~~/" + file
	}
	return m.Paths.Expand(file, config.Histories)
}

// Old backs up file (.bak and a rotating .oldN), archives the current
// conversation and resumes file, which becomes the SaveAsKey target.
func (m *Manager) Old(ctx context.Context, id, file string) error {
	src := m.resolve(file)
	if !history.Exists(src) {
		return fmt.Errorf("conversation not found: %s", file)
	}
	if err := history.Backup(src); err != nil {
		return err
	}
	return m.resume(ctx, id, src, src, "old")
}

// Templated copies the history file over templates/<base name>, backs the
// copy up and resumes it; the conversation is archived under a new name.
func (m *Manager) Templated(ctx context.Context, id, file string) error {
	src := m.resolve(file)
	tmpl := m.Paths.Expand("~~~/"+filepath.Base(file), config.Templates)
	if history.Exists(src) {
		if err := history.Copy(src, tmpl); err != nil {
			return err
		}
	} else {
		if !history.Exists(tmpl) {
			return errors.New("Template not found")
		}
		m.console().Error("Template not found")
	}
	if err := history.Backup(tmpl); err != nil {
		return err
	}
	return m.resume(ctx, id, tmpl, src, "template")
}

// resume loads path into the session after archiving the current
// conversation, restores the command-mode input history from the hidden
// sibling of path or of source, and records the save-back target.
func (m *Manager) resume(ctx context.Context, id, path, source, startType string) error {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	conv, err := history.Load(path)
	if err != nil {
		return err
	}
	if history.Exists(s.Path) {
		if err := m.archive(ctx, s); err != nil {
			return err
		}
	}
	s.conv = conv
	s.started = false
	if err := m.initSession(ctx, s, "Starting "+startType); err != nil {
		return err
	}
	for _, hidden := range []string{history.Hidden(path), history.Hidden(source)} {
		if history.Exists(hidden) {
			_ = history.Copy(hidden, m.Paths.ModeHistoryFile())
			break
		}
	}
	if startType == "template" {
		return m.setSaveAs(nil)
	}
	return m.setSaveAs(path)
}

// Undo drops the last exchange and persists. With turns [..., prev, user,
// reply] it returns user and prev, what the original showed; ok is false,
// and nothing changes, when the conversation would become empty.
func (m *Manager) Undo(id string) (user, prev string, ok bool, err error) {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	user, prev, ok = s.conv.Undo()
	if !ok {
		return "", "", false, nil
	}
	return user, prev, true, s.conv.Save(s.Path)
}

// Recap returns the last exchange; ok is false with fewer than two turns.
func (m *Manager) Recap(id string) (user, reply string, ok bool) {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conv.LastExchange()
}

// NewTemplate copies the session's live conversation file byte for byte
// to templates/<name> (name verbatim) and returns the full path.
func (m *Manager) NewTemplate(id, name string) (string, error) {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	dest := m.Paths.Expand(name, config.Templates)
	if err := history.Copy(s.Path, dest); err != nil {
		return "", err
	}
	m.console().Log("Template created: %s", dest)
	return dest, nil
}

// Replace swaps session id's conversation for msgs (roles user/assistant)
// plus extra channels (the non-"0" keys of the file, nil for none) and
// persists it to the session's current file.
func (m *Manager) Replace(id string, msgs []history.Message, extra map[string]json.RawMessage) error {
	s := m.Session(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	conv := &history.Conversation{Messages: make([]history.Message, 0, len(msgs))}
	for _, msg := range msgs {
		conv.Append(msg.Role, msg.Content)
	}
	if len(extra) > 0 {
		conv.Extra = make(map[string]json.RawMessage, len(extra))
		for k, v := range extra {
			conv.Extra[k] = append(json.RawMessage(nil), v...)
		}
	}
	s.conv = conv
	return s.conv.Save(s.Path)
}
