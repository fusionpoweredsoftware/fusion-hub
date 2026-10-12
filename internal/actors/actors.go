// Package actors lets the AI play a person. Profiles (person, gender,
// biosignature) live in ~/.fusion-hub/actors/actors.json, seeded from the
// built-in list, and are exchanged with the model in UIDMD notation.
package actors

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/uidmd"
)

//go:embed actors.json
var defaultProfiles []byte

// Actor is one persona.
type Actor struct {
	Person       string `json:"person"`
	Gender       string `json:"gender,omitempty"`
	Biosignature string `json:"biosignature"`
}

// DefaultID is used when nothing is specified.
const DefaultID = "EINSTEIN"

// BioSearch is the special actor that writes a biosignature for a person;
// its result is saved as BioFound.
const (
	BioSearch = "BIO_SEARCH"
	BioFound  = "BIO_FOUND"
)

// Book is the profiles file.
type Book struct {
	Path     string
	Profiles map[string]Actor
}

// Open loads dir/actors.json, creating it from the built-in profiles.
func Open(dir string) (*Book, error) {
	path := filepath.Join(dir, "actors.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, defaultProfiles, 0o600); err != nil {
			return nil, err
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	book := &Book{Path: path, Profiles: map[string]Actor{}}
	if err := json.Unmarshal(b, &book.Profiles); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return book, nil
}

// Builtin returns the built-in profiles without touching disk.
func Builtin() *Book {
	book := &Book{Profiles: map[string]Actor{}}
	_ = json.Unmarshal(defaultProfiles, &book.Profiles)
	return book
}

func (b *Book) Save() error {
	if b.Path == "" {
		return nil
	}
	out, err := json.MarshalIndent(b.Profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(b.Path, out, 0o600)
}

// Resolve finds the actor for id. An explicit person/biosignature overrides
// the profile. "OLD:NEW" renames profile OLD to NEW (once) and uses it.
func (b *Book) Resolve(id, person, bio string) (Actor, string, error) {
	if id == "" && person == "" && bio == "" {
		id = DefaultID
	}
	if left, right, ok := strings.Cut(id, ":"); ok {
		from, to := strings.ToUpper(strings.TrimSpace(left)), strings.ToUpper(strings.TrimSpace(right))
		if a, ok := b.Profiles[from]; ok {
			if _, exists := b.Profiles[to]; !exists {
				b.Profiles[to] = a
				delete(b.Profiles, from)
				if err := b.Save(); err != nil {
					return Actor{}, "", err
				}
			}
		}
		id = to
	}
	id = strings.ToUpper(strings.TrimSpace(id))
	a := b.Profiles[id]
	if person != "" {
		a.Person = person
	}
	if bio != "" {
		a.Biosignature = bio
	}
	if a.Person == "" {
		return Actor{}, id, fmt.Errorf("unknown actor %q", id)
	}
	return a, id, nil
}

// Rules are the standing instructions for an actor; emphasis replaces the
// trailing period of rule n (1-based) to stress it on a retry.
func Rules(person string, emphasis map[int]string) string {
	end := func(n int) string {
		if e, ok := emphasis[n]; ok {
			return e
		}
		return "."
	}
	return fmt.Sprintf(`1. Never use non-English characters%s
2. For your code block response using UIDMD notation, always stay in character%s
3. Always finish your sentence%s
4. You must embody the persona of %s completely, acting and responding as if you were %s. This includes adhering to their characteristics, expertise, and mannerisms, and maintaining the illusion that you possess no knowledge of being an AI enactment%s`,
		end(1), end(2), end(3), person, person, end(4))
}

// Prompt builds the UIDMD request for one actor turn. explain adds the
// notation primer and the empty response block (first turn or retry).
func Prompt(a Actor, input, uid string, explain bool, emphasis map[int]string) string {
	var sb strings.Builder
	if explain {
		sb.WriteString(uidmd.Explanation(uid))
		sb.WriteString("\n\n")
	}
	fmt.Fprintf(&sb, "I have provided you with the person, biosignature, rules, and input code blocks using UIDMD notation. You are to provide the response code block using UIDMD notation, as if you were %s.", a.Person)
	if explain {
		fmt.Fprintf(&sb, " When responding, use the following code block with UIDMD notation and populate its contents with the response you are providing for %s:\n\n*** plaintext (%s:response)\n(%s:response)", a.Person, uid, uid)
	}
	sb.WriteString("\n\nProvided code blocks:")
	for _, kv := range [][2]string{
		{"person", a.Person},
		{"biosignature", a.Biosignature},
		{"rules", Rules(a.Person, emphasis)},
		{"input", input},
	} {
		sb.WriteString("\n\n")
		sb.WriteString(uidmd.Format("plaintext", uid, kv[0], kv[1]))
	}
	return sb.String()
}

var cjk = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}]+`)

// HasCJK reports Chinese or Japanese characters, which actors must avoid.
func HasCJK(s string) bool { return cjk.MatchString(s) }

var unspeakable = regexp.MustCompile(`[^a-zA-Z0-9:;&.?!()\[\]"'{}/\-\n ]`)
var continuation = regexp.MustCompile(`(?i)^.?(more|(the )?end|continuation).?(.){0,2}$`)

// Speakable splits a reply into paragraphs fit for text-to-speech,
// dropping symbols and "(more)"-style continuation markers.
func Speakable(text string) []string {
	var out []string
	for _, p := range strings.Split(unspeakable.ReplaceAllString(text, ""), "\n\n") {
		p = strings.TrimSpace(p)
		if p == "" || continuation.MatchString(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}
