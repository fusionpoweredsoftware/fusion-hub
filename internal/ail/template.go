package ail

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Vars is the variable scope of a running pipeline.
type Vars map[string]any

var placeholder = regexp.MustCompile(`\$\{\s*([^}]+?)\s*\}`)

// Lookup resolves a dotted path such as chapter.objectives.0.title.
func (v Vars) Lookup(path string) (any, bool) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	var cur any = map[string]any(v)
	for _, p := range parts {
		switch c := cur.(type) {
		case map[string]any:
			next, ok := c[p]
			if !ok {
				return nil, false
			}
			cur = next
		case Vars:
			next, ok := c[p]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(c) {
				return nil, false
			}
			cur = c[i]
		case []string:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(c) {
				return nil, false
			}
			cur = c[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// eval resolves "path | filter | filter".
func (v Vars) eval(expr string) (any, error) {
	parts := strings.Split(expr, "|")
	val, _ := v.Lookup(parts[0])
	for _, f := range parts[1:] {
		name, arg, _ := strings.Cut(strings.TrimSpace(f), ":")
		switch name {
		case "json":
			b, err := json.Marshal(val)
			if err != nil {
				return nil, err
			}
			val = string(b)
		case "len":
			switch c := val.(type) {
			case []any:
				val = len(c)
			case map[string]any:
				val = len(c)
			case string:
				val = len([]rune(c))
			case nil:
				val = 0
			default:
				val = len(Stringify(c))
			}
		case "upper":
			val = strings.ToUpper(Stringify(val))
		case "lower":
			val = strings.ToLower(Stringify(val))
		case "trim":
			val = strings.TrimSpace(Stringify(val))
		case "default":
			if !Truthy(val) {
				val = arg
			}
		default:
			return nil, fmt.Errorf("unknown filter %q in ${%s}", name, expr)
		}
	}
	return val, nil
}

// Render substitutes every ${...} in s. Undefined variables render empty.
func (v Vars) Render(s string) (string, error) {
	var firstErr error
	out := placeholder.ReplaceAllStringFunc(s, func(m string) string {
		val, err := v.eval(placeholder.FindStringSubmatch(m)[1])
		if err != nil && firstErr == nil {
			firstErr = err
		}
		return Stringify(val)
	})
	return out, firstErr
}

// Value renders s, but when s is exactly one placeholder it returns the
// referenced value itself so lists and objects keep their type.
func (v Vars) Value(s string) (any, error) {
	t := strings.TrimSpace(s)
	if m := placeholder.FindStringSubmatchIndex(t); m != nil && m[0] == 0 && m[1] == len(t) {
		return v.eval(t[m[2]:m[3]])
	}
	return v.Render(s)
}

// Resolve renders every string inside an arbitrary YAML value.
func (v Vars) Resolve(x any) (any, error) {
	switch c := x.(type) {
	case string:
		return v.Value(c)
	case []any:
		out := make([]any, len(c))
		for i, e := range c {
			r, err := v.Resolve(e)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(c))
		for k, e := range c {
			r, err := v.Resolve(e)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	default:
		return x, nil
	}
}

// Stringify renders a value for text: strings as-is, nil as empty,
// everything else as JSON.
func Stringify(x any) string {
	switch c := x.(type) {
	case nil:
		return ""
	case string:
		return c
	case fmt.Stringer:
		return c.String()
	}
	b, err := json.Marshal(x)
	if err != nil {
		return fmt.Sprint(x)
	}
	return string(b)
}

// Truthy is false for nil, false, 0, empty strings/collections and the
// strings "false", "0", "no", "null".
func Truthy(x any) bool {
	switch c := x.(type) {
	case nil:
		return false
	case bool:
		return c
	case int:
		return c != 0
	case float64:
		return c != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(c)) {
		case "", "false", "0", "no", "null", "nil":
			return false
		}
		return true
	case []any:
		return len(c) > 0
	case map[string]any:
		return len(c) > 0
	}
	return true
}

// Condition evaluates a when/unless/until expression: a template that is
// truthy, or "a == b" / "a != b" comparing rendered text.
func (v Vars) Condition(expr string) (bool, error) {
	for _, op := range []string{"!=", "=="} {
		if l, r, ok := strings.Cut(expr, " "+op+" "); ok {
			ls, err := v.Render(l)
			if err != nil {
				return false, err
			}
			rs, err := v.Render(r)
			if err != nil {
				return false, err
			}
			eq := strings.TrimSpace(unquote(ls)) == strings.TrimSpace(unquote(rs))
			return eq == (op == "=="), nil
		}
	}
	val, err := v.Value(expr)
	if err != nil {
		return false, err
	}
	return Truthy(val), nil
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

// Assign sets a variable; a dotted name such as room.items sets a field
// of an existing object (objects are shared, so lists holding it see it).
func (v Vars) Assign(name string, val any) error {
	parent, field, ok := strings.Cut(name, ".")
	if !ok {
		v[name] = val
		return nil
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		parent, field = name[:i], name[i+1:]
	}
	obj, found := v.Lookup(parent)
	if !found {
		return fmt.Errorf("cannot set %s: %s is not defined", name, parent)
	}
	m, isMap := obj.(map[string]any)
	if !isMap {
		return fmt.Errorf("cannot set %s: %s is not an object", name, parent)
	}
	m[field] = val
	return nil
}
