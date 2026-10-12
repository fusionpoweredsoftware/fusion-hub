// Package uidmd implements UIDMD notation: code blocks whose opening and
// closing markers both carry a unique id and a key, so a parser can find
// each block without ambiguity.
//
//	*** <format> (<uid>:<key>)
//	<value>
//	(<uid>:<key>)
package uidmd

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Block is one parsed UIDMD block.
type Block struct {
	Format string
	UID    string
	Key    string
	Value  string
}

var header = regexp.MustCompile(`\*\*\*\s*(\w+)\s*\((\w+):([^)\s]+)\)`)

// Parse returns the blocks found in s keyed by block key. A block whose
// closing marker is missing is ignored; repeated keys keep the last one.
func Parse(s string) map[string]Block {
	out := map[string]Block{}
	pos := 0
	for pos < len(s) {
		loc := header.FindStringSubmatchIndex(s[pos:])
		if loc == nil {
			break
		}
		format := s[pos+loc[2] : pos+loc[3]]
		uid := s[pos+loc[4] : pos+loc[5]]
		key := s[pos+loc[6] : pos+loc[7]]
		bodyStart := pos + loc[1]
		closing := "(" + uid + ":" + key + ")"
		end := strings.Index(s[bodyStart:], closing)
		if end < 0 {
			pos = bodyStart
			continue
		}
		out[key] = Block{Format: format, UID: uid, Key: key, Value: strings.TrimSpace(s[bodyStart : bodyStart+end])}
		pos = bodyStart + end + len(closing)
	}
	return out
}

// Values flattens Parse into key -> value, with "*" holding the raw text.
func Values(s string) map[string]any {
	out := map[string]any{"*": s}
	for k, b := range Parse(s) {
		out[k] = b.Value
	}
	return out
}

// Format renders one block.
func Format(format, uid, key, value string) string {
	return fmt.Sprintf("*** %s (%s:%s)\n%s\n(%s:%s)", format, uid, key, value, uid, key)
}

// NewUID returns a timestamp id such as 20240128T061128170Z.
func NewUID(t time.Time) string {
	return strings.NewReplacer("-", "", ":", "", ".", "").Replace(t.UTC().Format("2006-01-02T15:04:05.000Z"))
}

// Explanation teaches a model the notation, using uid in its examples.
func Explanation(uid string) string {
	return fmt.Sprintf(`For your code blocks, you must use the UIDMD notation. It is similar to backtick notation, but different. The abstract form is:

*** <code block data type> (<uid>:<key>)
<value>
(<uid>:<key>)

The left side of the colon in parentheses is the unique id (%[1]s for this conversation) and the right side is the key that names the block. The same (<uid>:<key>) marker closes the block; this is the most important part of the notation. For example, a plain-text block about whales:

*** plaintext (%[1]s:whales)
Did you know baby orcas are called calves?
(%[1]s:whales)

If there is more than one block with the same key, number them as <key>.<instance>, e.g. (%[1]s:item.1), (%[1]s:item.2).`, uid)
}
