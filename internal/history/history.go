// Package history stores chat conversations in the legacy fusion-hub file
// format and provides the file housekeeping (backups, unique names) that
// chat sessions rely on.
//
// A conversation file is a compact JSON object keyed by channel; channel
// "0" holds the turns:
//
//	{"0":[{"role":"user","parts":[{"text":"..."}]},{"role":"model","parts":[{"text":"..."}]}]}
//
// In memory a turn is a Message with role "user" or "assistant"; "model"
// and "assistant" are converted at the file boundary. Other channels are
// kept verbatim.
package history

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"

	// roleModel is the assistant's role in the file format.
	roleModel = "model"
	// mainChannel holds the conversation turns.
	mainChannel = "0"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Conversation is one chat: its turns plus any extra channels of the file.
type Conversation struct {
	Messages []Message
	// Extra holds the channels other than "0", verbatim.
	Extra map[string]json.RawMessage
}

// legacyTurn is one turn in the file format.
type legacyTurn struct {
	Role  string       `json:"role"`
	Parts []legacyPart `json:"parts"`
}

type legacyPart struct {
	Text string `json:"text"`
}

// Load reads a conversation file: the legacy format, or the interim Go
// format {"system":...,"messages":[{"role","content"}]} (system dropped).
func Load(path string) (*Conversation, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse decodes a conversation file's content.
func Parse(b []byte) (*Conversation, error) {
	c := &Conversation{}
	if len(bytes.TrimSpace(b)) == 0 {
		return c, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	if msgs, ok := raw["messages"]; ok && raw[mainChannel] == nil {
		var list []Message
		if err := json.Unmarshal(msgs, &list); err != nil {
			return nil, err
		}
		for _, m := range list {
			c.Append(m.Role, m.Content)
		}
		return c, nil
	}
	for k, v := range raw {
		if k != mainChannel {
			if c.Extra == nil {
				c.Extra = map[string]json.RawMessage{}
			}
			c.Extra[k] = v
			continue
		}
		var turns []legacyTurn
		if err := json.Unmarshal(v, &turns); err != nil {
			return nil, err
		}
		for _, t := range turns {
			parts := make([]string, len(t.Parts))
			for i, p := range t.Parts {
				parts[i] = p.Text
			}
			c.Append(t.Role, strings.Join(parts, "\n"))
		}
	}
	return c, nil
}

// Bytes encodes the conversation in the file format.
func (c *Conversation) Bytes() ([]byte, error) {
	turns := make([]legacyTurn, len(c.Messages))
	for i, m := range c.Messages {
		role := m.Role
		if role == RoleAssistant {
			role = roleModel
		}
		turns[i] = legacyTurn{Role: role, Parts: []legacyPart{{Text: m.Content}}}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	buf.WriteString(`{"0":`)
	if err := enc.Encode(turns); err != nil {
		return nil, err
	}
	buf.Truncate(buf.Len() - 1) // Encode's newline
	keys := make([]string, 0, len(c.Extra))
	for k := range c.Extra {
		if k != mainChannel {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		buf.WriteByte(',')
		if err := enc.Encode(k); err != nil {
			return nil, err
		}
		buf.Truncate(buf.Len() - 1)
		buf.WriteByte(':')
		buf.Write(bytes.TrimSpace(c.Extra[k]))
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Save writes the conversation file (compact JSON, mode 0644).
func (c *Conversation) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := c.Bytes()
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (c *Conversation) Clone() *Conversation {
	out := &Conversation{Messages: make([]Message, len(c.Messages))}
	copy(out.Messages, c.Messages)
	if c.Extra != nil {
		out.Extra = make(map[string]json.RawMessage, len(c.Extra))
		for k, v := range c.Extra {
			out.Extra[k] = append(json.RawMessage(nil), v...)
		}
	}
	return out
}

// Append adds a turn; the file role "model" is stored as "assistant".
func (c *Conversation) Append(role, content string) {
	if role == roleModel {
		role = RoleAssistant
	}
	c.Messages = append(c.Messages, Message{Role: role, Content: content})
}

// Truncate drops messages beyond n.
func (c *Conversation) Truncate(n int) {
	if n < len(c.Messages) {
		c.Messages = c.Messages[:n]
	}
}

// Clean strips trailing empty assistant turns (the original's clean()).
func (c *Conversation) Clean() {
	for n := len(c.Messages); n > 0; n-- {
		m := c.Messages[n-1]
		if m.Role != RoleAssistant || m.Content != "" {
			break
		}
		c.Messages = c.Messages[:n-1]
	}
}

// LastExchange returns the last two turns: the user's input and the reply.
// ok is false with fewer than two turns.
func (c *Conversation) LastExchange() (user, reply string, ok bool) {
	n := len(c.Messages)
	if n < 2 {
		return "", "", false
	}
	return c.Messages[n-2].Content, c.Messages[n-1].Content, true
}

// Undo drops the last exchange. With turns [..., prev, user, reply] it
// returns user and prev (what the original printed); ok is false, and
// nothing changes, when the conversation would become empty.
func (c *Conversation) Undo() (user, prev string, ok bool) {
	n := len(c.Messages)
	if n < 3 {
		return "", "", false
	}
	user, prev = c.Messages[n-2].Content, c.Messages[n-3].Content
	c.Messages = c.Messages[:n-2]
	return user, prev, true
}

// Exists reports whether path is an existing file.
func Exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// Copy copies a file byte for byte, creating the destination directory.
func Copy(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
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
	return os.WriteFile(path+".old", []byte(strconv.Itoa(counter+1)), 0o644)
}

// ErrTooManyCopies is returned by UniquePath when 99 copies of a name exist.
var ErrTooManyCopies = errors.New("99 copies found")

// UniquePath returns dir/name, or dir/"name (n).json" (n from 2 to 99)
// when that exists. With 99 copies it returns ErrTooManyCopies.
func UniquePath(dir, name string) (string, error) {
	p := filepath.Join(dir, name)
	base := strings.TrimSuffix(p, ".json")
	for n := 2; Exists(p); n++ {
		if n > 99 {
			return "", ErrTooManyCopies
		}
		p = fmt.Sprintf("%s (%d).json", base, n)
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
