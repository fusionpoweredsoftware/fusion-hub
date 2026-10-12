// Package history stores chat conversations as JSON files and provides the
// file housekeeping (backups, unique names) that chat sessions rely on.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Conversation is one chat: an optional system prompt and its turns.
type Conversation struct {
	System   string    `json:"system,omitempty"`
	Messages []Message `json:"messages"`
}

// Load reads a conversation file.
func Load(path string) (*Conversation, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Conversation{}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Save writes the conversation as indented JSON.
func (c *Conversation) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func (c *Conversation) Clone() *Conversation {
	out := &Conversation{System: c.System, Messages: make([]Message, len(c.Messages))}
	copy(out.Messages, c.Messages)
	return out
}

func (c *Conversation) Append(role, content string) {
	c.Messages = append(c.Messages, Message{Role: role, Content: content})
}

// Truncate drops messages beyond n.
func (c *Conversation) Truncate(n int) {
	if n < len(c.Messages) {
		c.Messages = c.Messages[:n]
	}
}

// LastExchange returns the most recent user message and the reply to it.
func (c *Conversation) LastExchange() (user, assistant string, ok bool) {
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Role == RoleUser {
			user = c.Messages[i].Content
			if i+1 < len(c.Messages) {
				assistant = c.Messages[i+1].Content
			}
			return user, assistant, true
		}
	}
	return "", "", false
}

// Undo removes the last user message and everything after it, returning
// the new last exchange (the one now being resumed from).
func (c *Conversation) Undo() (user, assistant string, ok bool) {
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Role == RoleUser {
			c.Messages = c.Messages[:i]
			return c.LastExchange()
		}
	}
	return "", "", false
}

// Exists reports whether path is an existing file.
func Exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// Copy copies a file, creating the destination directory.
func Copy(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o600)
}

// Backup writes path.bak and a rotating path.old<N> (N cycles through 0-9,
// tracked in path.old) before an old conversation is resumed.
func Backup(path string) error {
	if !Exists(path) {
		return nil
	}
	if err := Copy(path, path+".bak"); err != nil {
		return err
	}
	counter := 1
	if b, err := os.ReadFile(path + ".old"); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			counter = n
		}
	}
	if err := Copy(path, fmt.Sprintf("%s.old%d", path, counter%10)); err != nil {
		return err
	}
	return os.WriteFile(path+".old", []byte(strconv.Itoa(counter+1)), 0o600)
}

// UniquePath returns dir/name, or dir/"name (n).json" when that exists.
func UniquePath(dir, name string) (string, error) {
	base := strings.TrimSuffix(name, ".json")
	p := filepath.Join(dir, base+".json")
	for n := 2; Exists(p); n++ {
		if n > 100 {
			return "", errors.New("over 99 conversations already use this name")
		}
		p = filepath.Join(dir, fmt.Sprintf("%s (%d).json", base, n))
	}
	return p, nil
}

// TimestampName is the fallback archive name, e.g. 2024-01-28T06-11-28.json.
func TimestampName(t time.Time) string {
	return strings.ReplaceAll(t.UTC().Format("2006-01-02T15:04:05"), ":", "-") + ".json"
}

// Hidden returns the dot-prefixed sibling of path with its .json removed;
// a conversation's command-mode input history lives there.
func Hidden(path string) string {
	return filepath.Join(filepath.Dir(path), "."+strings.TrimSuffix(filepath.Base(path), ".json"))
}
