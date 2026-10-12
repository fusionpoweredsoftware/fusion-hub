// Package ail runs AIL apps: declarative YAML pipelines of AI prompts with
// typed outputs (text, json, uidmd), variables, loops and conditions, plus
// steps that print, speak, set variables, run REPL commands or let an
// actor answer.
package ail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/actors"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/tts"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/uidmd"
)

// Output types for prompt steps.
const (
	Text  = "text"
	JSON  = "json"
	UIDMD = "uidmd"
)

// Pipeline is one AIL app.
type Pipeline struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	Variables   Variables `yaml:"variables"`
	Steps       []Step    `yaml:"steps"`
}

// Step is one pipeline step. Exactly one action field is set: prompt,
// print, say, command, set, actor, foreach, repeat, fail or stop.
type Step struct {
	// ID names the variable the step's result is stored in.
	ID     string `yaml:"id"`
	When   string `yaml:"when"`
	Unless string `yaml:"unless"`

	Prompt    *string  `yaml:"prompt"`
	Output    string   `yaml:"output"`
	Require   []string `yaml:"require"`
	Retries   *int     `yaml:"retries"`
	Inputs    []string `yaml:"inputs"`
	Ephemeral bool     `yaml:"ephemeral"`
	Show      bool     `yaml:"show"`

	Print   *string        `yaml:"print"`
	Say     *string        `yaml:"say"`
	Voice   string         `yaml:"voice"`
	Sync    bool           `yaml:"sync"`
	Command *string        `yaml:"command"`
	Set     map[string]any `yaml:"set"`
	Actor   *ActorStep     `yaml:"actor"`

	ForEach *string `yaml:"foreach"`
	As      string  `yaml:"as"`
	Split   string  `yaml:"split"`
	Repeat  any     `yaml:"repeat"`
	Until   string  `yaml:"until"`
	Steps   []Step  `yaml:"steps"`

	Save  *string `yaml:"save"`
	Value any     `yaml:"value"`

	Fail *string `yaml:"fail"`
	Stop bool    `yaml:"stop"`
}

// Variables keeps the YAML order so later variables can use earlier ones.
type Variables []Variable

type Variable struct {
	Name  string
	Value any
}

func (v *Variables) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return errors.New("variables must be a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		var val any
		if err := node.Content[i+1].Decode(&val); err != nil {
			return err
		}
		*v = append(*v, Variable{Name: node.Content[i].Value, Value: val})
	}
	return nil
}

// ActorStep lets one actor answer, or several take turns for Rounds.
type ActorStep struct {
	ID           string `yaml:"id"`
	IDs          any    `yaml:"ids"`
	Person       string `yaml:"person"`
	Biosignature string `yaml:"biosignature"`
	Input        string `yaml:"input"`
	Voice        string `yaml:"voice"`
	Rounds       any    `yaml:"rounds"`
}

func (s *Step) kind() (string, error) {
	var kinds []string
	add := func(set bool, k string) {
		if set {
			kinds = append(kinds, k)
		}
	}
	add(s.Prompt != nil, "prompt")
	add(s.Print != nil, "print")
	add(s.Say != nil, "say")
	add(s.Command != nil, "command")
	add(s.Set != nil, "set")
	add(s.Actor != nil, "actor")
	add(s.ForEach != nil, "foreach")
	add(s.Repeat != nil, "repeat")
	add(s.Save != nil, "save")
	add(s.Fail != nil, "fail")
	add(s.Stop, "stop")
	switch len(kinds) {
	case 0:
		return "", errors.New("step has no action (prompt, print, say, command, set, actor, foreach, repeat, save, fail or stop)")
	case 1:
		return kinds[0], nil
	}
	return "", fmt.Errorf("step has more than one action: %s", strings.Join(kinds, ", "))
}

// Parse decodes and validates a pipeline.
func Parse(b []byte) (*Pipeline, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	p := &Pipeline{}
	if err := dec.Decode(p); err != nil {
		return nil, fmt.Errorf("ail: %w", err)
	}
	if len(p.Steps) == 0 {
		return nil, errors.New("ail: pipeline has no steps")
	}
	if err := validate(p.Steps, "steps"); err != nil {
		return nil, err
	}
	return p, nil
}

// Load reads and parses a pipeline file.
func Load(path string) (*Pipeline, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

func validate(steps []Step, where string) error {
	for i := range steps {
		s := &steps[i]
		at := fmt.Sprintf("%s[%d]", where, i)
		if s.ID != "" {
			at += " (" + s.ID + ")"
		}
		k, err := s.kind()
		if err != nil {
			return fmt.Errorf("ail: %s: %w", at, err)
		}
		switch k {
		case "prompt":
			switch s.Output {
			case "", Text, JSON, UIDMD:
			default:
				return fmt.Errorf("ail: %s: output must be text, json or uidmd, not %q", at, s.Output)
			}
			if len(s.Inputs) > 0 && s.Output != UIDMD {
				return fmt.Errorf("ail: %s: inputs are only used with output: uidmd", at)
			}
		case "foreach", "repeat":
			if len(s.Steps) == 0 {
				return fmt.Errorf("ail: %s: %s needs steps", at, k)
			}
			if err := validate(s.Steps, at+".steps"); err != nil {
				return err
			}
		case "actor":
			if s.Actor.ID != "" && s.Actor.IDs != nil {
				return fmt.Errorf("ail: %s: actor takes id or ids, not both", at)
			}
		}
		if k != "foreach" && k != "repeat" && len(s.Steps) > 0 {
			return fmt.Errorf("ail: %s: only foreach and repeat take nested steps", at)
		}
	}
	return nil
}

// ChatOptions tune one message to the model.
type ChatOptions struct {
	// Ephemeral leaves the conversation unchanged after the reply.
	Ephemeral bool
	// Out receives the streamed reply (nil = silent).
	Out io.Writer
}

// Runtime connects a pipeline to the rest of fusion-hub.
type Runtime struct {
	// Chat sends a message in the app's chat session.
	Chat func(ctx context.Context, prompt string, opts ChatOptions) (string, error)
	// Undo drops the last exchange from the chat session.
	Undo func() error
	// HistoryLen is the number of messages in the chat session.
	HistoryLen func() int
	// Command runs a REPL command line, e.g. "say hello".
	Command func(ctx context.Context, line string) (string, error)
	Speaker tts.Speaker
	Actors  *actors.Book
	Out     *output.Writer
	Now     func() time.Time
	// MaxIterations caps repeat loops (default 1000).
	MaxIterations int
}

var errStop = errors.New("stop")

// Run executes the pipeline with input (the user's line) and app
// settings, returning the final variables.
func (p *Pipeline) Run(ctx context.Context, rt *Runtime, input string, settings map[string]any) (Vars, error) {
	if rt.Out == nil {
		rt.Out = output.Discard()
	}
	if rt.Now == nil {
		rt.Now = time.Now
	}
	if rt.MaxIterations == 0 {
		rt.MaxIterations = 1000
	}
	if settings == nil {
		settings = map[string]any{}
	}
	vars := Vars{"input": input, "args": toAny(strings.Fields(input)), "settings": settings}
	for _, v := range p.Variables {
		r, err := vars.Resolve(v.Value)
		if err != nil {
			return vars, err
		}
		vars[v.Name] = r
	}
	r := &runner{rt: rt, vars: vars, explained: map[string]bool{}}
	err := r.steps(ctx, p.Steps)
	if errors.Is(err, errStop) {
		err = nil
	}
	return vars, err
}

type runner struct {
	rt        *Runtime
	vars      Vars
	explained map[string]bool
}

func (r *runner) steps(ctx context.Context, steps []Step) error {
	for i := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.step(ctx, &steps[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) step(ctx context.Context, s *Step) error {
	if s.When != "" {
		ok, err := r.vars.Condition(s.When)
		if err != nil || !ok {
			return err
		}
	}
	if s.Unless != "" {
		ok, err := r.vars.Condition(s.Unless)
		if err != nil || ok {
			return err
		}
	}
	kind, _ := s.kind()
	var result any
	var err error
	switch kind {
	case "prompt":
		result, err = r.prompt(ctx, s)
	case "print":
		var text string
		if text, err = r.vars.Render(*s.Print); err == nil {
			r.rt.Out.Print(text)
			result = text
		}
	case "say":
		var text string
		if text, err = r.vars.Render(*s.Say); err == nil {
			result = text
			err = r.say(ctx, text, s.Voice == "male", s.Sync)
		}
	case "command":
		var line string
		if line, err = r.vars.Render(*s.Command); err == nil {
			if r.rt.Command == nil {
				return errors.New("ail: commands are not available here")
			}
			result, err = r.rt.Command(ctx, line)
		}
	case "set":
		keys := make([]string, 0, len(s.Set))
		for k := range s.Set {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val, err := r.vars.Resolve(s.Set[k])
			if err != nil {
				return err
			}
			if err := r.vars.Assign(k, val); err != nil {
				return err
			}
		}
		return nil
	case "save":
		result, err = r.save(s)
	case "actor":
		result, err = r.actor(ctx, s.Actor)
	case "foreach":
		err = r.foreach(ctx, s)
		return err
	case "repeat":
		return r.repeat(ctx, s)
	case "fail":
		msg, _ := r.vars.Render(*s.Fail)
		return errors.New(msg)
	case "stop":
		return errStop
	}
	if err != nil {
		if s.ID != "" {
			return fmt.Errorf("step %s: %w", s.ID, err)
		}
		return err
	}
	if s.ID != "" {
		r.vars[s.ID] = result
	}
	return nil
}

func (r *runner) say(ctx context.Context, text string, male, sync bool) error {
	if r.rt.Speaker == nil {
		return nil
	}
	return r.rt.Speaker.Say(ctx, text, male, sync)
}

func (r *runner) prompt(ctx context.Context, s *Step) (any, error) {
	if r.rt.Chat == nil {
		return nil, errors.New("ail: no chat available")
	}
	text, err := r.vars.Render(*s.Prompt)
	if err != nil {
		return nil, err
	}
	retries := 0
	if s.Output == JSON || s.Output == UIDMD {
		retries = 3
	}
	if s.Retries != nil {
		retries = *s.Retries
	}
	var uid string
	if s.Output == UIDMD {
		uid = uidmd.NewUID(r.rt.Now())
		text, err = r.uidmdPrompt(text, uid, s)
		if err != nil {
			return nil, err
		}
	}
	opts := ChatOptions{Ephemeral: s.Ephemeral}
	if s.Show {
		opts.Out = r.rt.Out
	}
	msg := text
	var problem string
	for attempt := 0; attempt <= retries; attempt++ {
		reply, err := r.rt.Chat(ctx, msg, opts)
		if err != nil {
			return nil, err
		}
		var value any
		switch s.Output {
		case JSON:
			v, perr := ExtractJSON(reply)
			if perr != nil {
				problem = "Your last response contained malformed JSON. Please double check your JSON and do not leave any of it out. Use the output from the JSON parser to help resolve the issue: " + perr.Error()
				break
			}
			if m := missing(v, s.Require); len(m) > 0 {
				problem = "Your last JSON response was missing or had empty values for: " + strings.Join(m, ", ") + ". Please provide the complete JSON."
				break
			}
			value = v
		case UIDMD:
			v := uidmd.Values(reply)
			if m := missing(v, s.Require); len(m) > 0 {
				problem = fmt.Sprintf("Your last response was missing these UIDMD code blocks: %s. Remember to open and close each block with (%s:<key>).", strings.Join(m, ", "), uid)
				break
			}
			value = map[string]any(v)
		default:
			value = reply
		}
		if value != nil {
			return value, nil
		}
		msg = problem + "\n\n" + text
	}
	return nil, fmt.Errorf("no valid %s response after %d attempts: %s", s.Output, retries+1, problem)
}

func (r *runner) uidmdPrompt(text, uid string, s *Step) (string, error) {
	var sb strings.Builder
	sb.WriteString(uidmd.Explanation(uid))
	sb.WriteString("\n\n")
	sb.WriteString(text)
	if len(s.Inputs) > 0 {
		sb.WriteString("\n\nProvided code blocks:")
		for _, name := range s.Inputs {
			v, _ := r.vars.Lookup(name)
			key := name[strings.LastIndex(name, ".")+1:]
			sb.WriteString("\n\n")
			sb.WriteString(uidmd.Format("plaintext", uid, key, Stringify(v)))
		}
	}
	if len(s.Require) > 0 {
		fmt.Fprintf(&sb, "\n\nRespond with these UIDMD code blocks using the id %s: %s.", uid, strings.Join(s.Require, ", "))
	}
	return sb.String(), nil
}

func (r *runner) foreach(ctx context.Context, s *Step) error {
	val, err := r.vars.Value(*s.ForEach)
	if err != nil {
		return err
	}
	var items []any
	switch c := val.(type) {
	case []any:
		items = c
	case nil:
	case map[string]any:
		for k, v := range c {
			items = append(items, map[string]any{"key": k, "value": v})
		}
	default:
		str := Stringify(c)
		if s.Split == "" {
			items = []any{str}
			break
		}
		sep := strings.NewReplacer(`\n`, "\n", `\t`, "\t").Replace(s.Split)
		for _, part := range strings.Split(str, sep) {
			if strings.TrimSpace(part) != "" {
				items = append(items, part)
			}
		}
	}
	as := s.As
	if as == "" {
		as = "item"
	}
	var results []any
	for i, item := range items {
		r.vars[as] = item
		r.vars["index"] = i
		if err := r.steps(ctx, s.Steps); err != nil {
			return err
		}
		if s.ID != "" {
			results = append(results, r.vars[as])
		}
	}
	if s.ID != "" {
		r.vars[s.ID] = results
	}
	return nil
}

func (r *runner) count(x any) (int, error) {
	switch c := x.(type) {
	case int:
		return c, nil
	case float64:
		return int(c), nil
	case string:
		v, err := r.vars.Render(c)
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("%q is not a count", v)
		}
		return n, nil
	case nil:
		return 0, nil
	}
	return 0, fmt.Errorf("%v is not a count", x)
}

func (r *runner) repeat(ctx context.Context, s *Step) error {
	n, err := r.count(s.Repeat)
	if err != nil {
		return err
	}
	if n > r.rt.MaxIterations || n < 0 {
		n = r.rt.MaxIterations
	}
	for i := 0; i < n; i++ {
		r.vars["index"] = i
		if err := r.steps(ctx, s.Steps); err != nil {
			return err
		}
		if s.Until != "" {
			done, err := r.vars.Condition(s.Until)
			if err != nil {
				return err
			}
			if done {
				break
			}
		}
	}
	return nil
}

func (r *runner) actor(ctx context.Context, a *ActorStep) (any, error) {
	book := r.rt.Actors
	if book == nil {
		book = actors.Builtin()
	}
	input := r.vars.Stringify("input")
	if a.Input != "" {
		var err error
		if input, err = r.vars.Render(a.Input); err != nil {
			return nil, err
		}
	}
	var ids []string
	if a.IDs != nil {
		v, err := r.vars.Resolve(a.IDs)
		if err != nil {
			return nil, err
		}
		switch c := v.(type) {
		case []any:
			for _, e := range c {
				ids = append(ids, Stringify(e))
			}
		default:
			ids = strings.FieldsFunc(Stringify(c), func(r rune) bool { return r == ',' || r == ' ' })
		}
	}
	if len(ids) == 0 {
		id, err := r.vars.Render(a.ID)
		if err != nil {
			return nil, err
		}
		ids = []string{id}
	}
	voice, err := r.vars.Render(a.Voice)
	if err != nil {
		return nil, err
	}
	rounds, err := r.count(a.Rounds)
	if err != nil {
		return nil, err
	}
	if rounds < 1 {
		rounds = 1
	}
	person, _ := r.vars.Render(a.Person)
	bio, _ := r.vars.Render(a.Biosignature)
	multi := len(ids) > 1 || rounds > 1
	conversation := "Today we are discussing: " + input
	var last string
	for round := 0; round < rounds; round++ {
		for _, id := range ids {
			act, rid, err := book.Resolve(id, person, bio)
			if err != nil {
				return nil, err
			}
			in := input
			if multi {
				in = "Avoid repeating the same initial words or phrases as earlier responses. This is the conversation so far:\n\n" + conversation
			}
			value, err := r.askActor(ctx, act, rid, in)
			if err != nil {
				return nil, err
			}
			last = value
			if rid == actors.BioSearch {
				name := strings.TrimSpace(input)
				if i := strings.IndexAny(name, "(:"); i >= 0 {
					name = strings.TrimSpace(name[:i])
				}
				book.Profiles[actors.BioFound] = actors.Actor{Person: name, Biosignature: value}
				if err := book.Save(); err != nil {
					return nil, err
				}
			}
			if err := r.speakActor(ctx, act, value, voice); err != nil {
				return nil, err
			}
			conversation += "\n\n" + act.Person + " said: " + value
		}
	}
	if multi {
		return conversation, nil
	}
	return last, nil
}

func (r *runner) askActor(ctx context.Context, act actors.Actor, id, input string) (string, error) {
	uid := uidmd.NewUID(r.rt.Now())
	var emphasis map[int]string
	for attempt := 0; attempt < 4; attempt++ {
		explain := attempt > 0 || !r.explained[id]
		if r.rt.HistoryLen != nil && r.rt.HistoryLen() < 4 {
			explain = true
		}
		reply, err := r.rt.Chat(ctx, actors.Prompt(act, input, uid, explain, emphasis), ChatOptions{})
		if err != nil {
			return "", err
		}
		block, ok := uidmd.Parse(reply)["response"]
		if ok && block.Value != "" && !actors.HasCJK(block.Value) {
			r.explained[id] = true
			return block.Value, nil
		}
		if actors.HasCJK(reply) {
			emphasis = map[int]string{1: "! (THIS MEANS NO CHINESE OR JAPANESE CHARACTERS!!!)"}
		}
		if r.rt.Undo != nil {
			if err := r.rt.Undo(); err != nil {
				return "", err
			}
		}
		r.rt.Out.Print("Loading...")
	}
	return "", fmt.Errorf("bad response from %s", act.Person)
}

func (r *runner) speakActor(ctx context.Context, act actors.Actor, value, voice string) error {
	if voice == "" {
		r.rt.Out.Print("\n" + output.Blue + act.Person + ">" + output.Reset + " " + value + "\n")
		return nil
	}
	male := voice == "male" || (voice == "auto" && act.Gender == "male")
	for _, p := range actors.Speakable(value) {
		r.rt.Out.Print("\n" + output.Blue + act.Person + ">" + output.Reset + " " + p + "\n")
		if err := r.say(ctx, p, male, true); err != nil {
			return err
		}
	}
	return nil
}

// Stringify returns the named variable as text.
func (v Vars) Stringify(name string) string {
	val, _ := v.Lookup(name)
	return Stringify(val)
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// save writes a value to a file: strings as text, anything else as JSON.
func (r *runner) save(s *Step) (any, error) {
	path, err := r.vars.Render(*s.Save)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("save needs a file name")
	}
	val, err := r.vars.Resolve(s.Value)
	if err != nil {
		return nil, err
	}
	var data []byte
	if str, ok := val.(string); ok {
		data = []byte(str)
	} else if data, err = json.MarshalIndent(val, "", "  "); err != nil {
		return nil, err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return path, os.WriteFile(path, data, 0o644)
}
