// Package ailjs runs the original .app.ail.js apps unchanged. The legacy
// multi-command preprocessor (command/multi-command.mjs) turns an app into a
// JavaScript module, and an embedded JS engine executes that module together
// with the original functions.mjs and app-lib files. The node built-ins and
// fusion-hub modules those files import (fs, path, crypto, json5,
// fusion-hub/cli, ...) are provided from Go, and chat-with-ai calls made by
// an app go through the Host so they use the same provider and history
// files as the REPL.
package ailjs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/dop251/goja"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
)

// Message is one chat turn in the legacy shape: role "user" or "model".
type Message struct {
	Role string
	Text string
}

// Host is what the runtime needs from the rest of fusion-hub.
type Host interface {
	Out() *output.Writer
	Paths() config.Paths
	// Settings is a snapshot of the settings file for setup.settings.
	Settings() map[string]any
	// Mode and SetMode read and change the REPL command mode ("" = CLI).
	Mode() string
	SetMode(mode string)
	// StartChat performs the chat-with-ai start for a session: with isNew the
	// current conversation is archived and a new one started; otherwise the
	// session is resumed (or initialized) if that has not happened yet.
	StartChat(ctx context.Context, sessionID string, isNew bool) error
	// History is the session's current conversation.
	History(sessionID string) []Message
	// Chat sends text with history as the conversation and returns the
	// reply ("" when the model gave none). It changes no session state.
	Chat(ctx context.Context, sessionID string, history []Message, text string, stream io.Writer) (string, error)
	// Persist replaces the session's conversation with channel "0" of
	// channels and keeps the other channels alongside it in the file.
	Persist(sessionID string, channels map[string][]Message) error
	// Execute runs a REPL line other than chat-with-ai (say, start, ...).
	// logStart says whether a plain "start <command>" logs its started line.
	Execute(ctx context.Context, line string, logStart bool) error
}

// Engine is one JS runtime. It is not safe for concurrent use; the REPL runs
// one command at a time.
type Engine struct {
	vm            *goja.Runtime
	host          Host
	legacy        fs.FS
	ctx           context.Context
	modules       map[string]goja.Value
	helpers       *goja.Object
	cwd           string
	stopInterrupt func()
	// histories is each session's chat history object as the last app run
	// left it. The original kept it in the app config between runs, so an
	// app's own edits (popped verification exchanges, spliced-in context)
	// carried over to the next line; an app-using-ai start reloads it from
	// the conversation file (ForgetHistory).
	histories map[string]goja.Value
}

// New creates a runtime. legacy holds example/, app-lib/ and actors.json
// (the module root's embedded files) as fallbacks for files not on disk.
func New(host Host, legacy fs.FS) (*Engine, error) {
	e := &Engine{vm: goja.New(), host: host, legacy: legacy, ctx: context.Background(), modules: map[string]goja.Value{}, histories: map[string]goja.Value{}}
	e.cwd, _ = os.Getwd()
	if err := e.setupGlobals(); err != nil {
		return nil, err
	}
	return e, nil
}

// RunApp runs an app the way "app-using-ai" did (runApp in
// command/app-using-ai.mjs): appFilePath is the app file, appInput the line
// typed by the user, appSettings the app's JSON settings.
func (e *Engine) RunApp(ctx context.Context, appFilePath, appInput string, appSettings map[string]any, sessionID string, debug bool) error {
	e.begin(ctx)
	defer e.end()
	mc := e.vm.NewObject()
	ai := e.vm.NewObject()
	_ = ai.Set("sessionId", sessionValue(e.vm, sessionID))
	hist, ok := e.histories[sessionID]
	if !ok {
		h := e.vm.NewObject()
		_ = h.Set("0", e.historyToJS(e.host.History(sessionID)))
		hist = h
	}
	_ = ai.Set("chatHistory", hist)
	defer func() { e.histories[sessionID] = ai.Get("chatHistory") }()
	if !debug {
		noop := e.vm.NewObject()
		_ = noop.Set("write", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = ai.Set("chatStdout", noop)
		_ = mc.Set("isLoggingMultiCommandFileContents", false)
		_ = mc.Set("isInterruptable", false)
		_ = mc.Set("isVerbose", false)
	}
	_ = mc.Set("aiChatSettings", ai)
	app := e.vm.NewObject()
	_ = app.Set("input", appInput)
	settings, err := e.toJS(appSettings)
	if err != nil {
		return err
	}
	if goja.IsUndefined(settings) || goja.IsNull(settings) {
		settings = e.vm.NewObject()
	}
	_ = app.Set("settings", settings)
	_ = app.Set("filePath", appFilePath)
	_ = mc.Set("app", app)
	_ = mc.Set("contextDir", "app")

	args, err := e.getArgs(appInput)
	if err != nil {
		return err
	}
	input := append([]string{appFilePath}, args...)
	return e.process(input, strings.TrimSpace(appFilePath+" "+appInput), mc)
}

// ForgetHistory makes the next RunApp for the session start from the
// conversation file again, as every app-using-ai start did.
func (e *Engine) ForgetHistory(sessionID string) {
	delete(e.histories, sessionID)
}

// MultiCommand runs "multi-command <file> [parameters]" (process in
// command/multi-command.mjs) with default settings: an empty history, the
// generated module logged and the history file announced.
func (e *Engine) MultiCommand(ctx context.Context, input []string, joinedInput string) error {
	e.begin(ctx)
	defer e.end()
	return e.process(input, joinedInput, e.vm.NewObject())
}

func (e *Engine) process(input []string, joinedInput string, settings *goja.Object) error {
	mod, err := e.exports("fusion-hub/mc-process")
	if err != nil {
		return err
	}
	fn, ok := goja.AssertFunction(mod.Get("process"))
	if !ok {
		return errors.New("ailjs: mc-process has no process function")
	}
	arg := e.vm.NewObject()
	in := make([]any, len(input))
	for i, s := range input {
		in[i] = s
	}
	_ = arg.Set("input", e.vm.ToValue(in))
	_ = arg.Set("joinedInput", joinedInput)
	_ = arg.Set("settings", settings)
	res, err := fn(goja.Undefined(), arg)
	if err != nil {
		return jsErr(err)
	}
	_, err = e.await(res)
	return err
}

// exports imports a module and returns its exports object.
func (e *Engine) exports(spec string) (*goja.Object, error) {
	v, err := e.await(e.importModule(spec))
	if err != nil {
		return nil, err
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return nil, fmt.Errorf("ailjs: %s has no exports", spec)
	}
	return obj, nil
}

// await returns a settled promise's value. Host functions resolve their
// promises before returning, so by the time a call into JS comes back every
// promise it created has settled.
func (e *Engine) await(v goja.Value) (goja.Value, error) {
	p, ok := v.Export().(*goja.Promise)
	if !ok {
		return v, nil
	}
	switch p.State() {
	case goja.PromiseStateFulfilled:
		return p.Result(), nil
	case goja.PromiseStateRejected:
		return nil, errors.New(errorText(p.Result()))
	}
	return nil, errors.New("ailjs: the app is still waiting on something that will never finish")
}

// errorText renders a thrown JS value like node's console would: Error
// objects as "Error: message" (with the stack when there is one).
func errorText(v goja.Value) string {
	if obj, ok := v.(*goja.Object); ok {
		if st := obj.Get("stack"); st != nil && !goja.IsUndefined(st) {
			return st.String()
		}
	}
	return v.String()
}

func jsErr(err error) error {
	var ex *goja.Exception
	if errors.As(err, &ex) {
		return errors.New(errorText(ex.Value()))
	}
	return err
}

func (e *Engine) begin(ctx context.Context) {
	e.ctx = ctx
	done := make(chan struct{})
	e.stopInterrupt = func() { close(done); e.vm.ClearInterrupt() }
	go func() {
		select {
		case <-ctx.Done():
			e.vm.Interrupt(ctx.Err())
		case <-done:
		}
	}()
}

func (e *Engine) end() {
	if e.stopInterrupt != nil {
		e.stopInterrupt()
		e.stopInterrupt = nil
	}
	e.ctx = context.Background()
}

// getArgs is the legacy tokenizer (single-quote grouping, \' escapes).
func (e *Engine) getArgs(s string) ([]string, error) {
	mod, err := e.exports("fusion-hub/functions")
	if err != nil {
		return nil, err
	}
	fn, _ := goja.AssertFunction(mod.Get("getArgs"))
	v, err := fn(goja.Undefined(), e.vm.ToValue(s))
	if err != nil {
		return nil, jsErr(err)
	}
	var out []string
	if err := e.fromJS(v, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// toJS converts a Go value to a native JS value through JSON.
func (e *Engine) toJS(v any) (goja.Value, error) {
	if v == nil {
		return goja.Undefined(), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return e.call(e.helpers.Get("parse"), e.vm.ToValue(string(b)))
}

// fromJS converts a JS value to Go through JSON.
func (e *Engine) fromJS(v goja.Value, out any) error {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	s, err := e.call(e.helpers.Get("stringify"), v)
	if err != nil {
		return err
	}
	if goja.IsUndefined(s) {
		return nil
	}
	return json.Unmarshal([]byte(s.String()), out)
}

func (e *Engine) call(fn goja.Value, args ...goja.Value) (goja.Value, error) {
	f, ok := goja.AssertFunction(fn)
	if !ok {
		return nil, errors.New("ailjs: not a function")
	}
	v, err := f(goja.Undefined(), args...)
	if err != nil {
		return nil, jsErr(err)
	}
	return v, nil
}

type jsHistory []struct {
	Role  string `json:"role"`
	Parts []struct {
		Text string `json:"text"`
	} `json:"parts"`
}

func (e *Engine) historyToJS(msgs []Message) goja.Value {
	h := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		h[i] = map[string]any{"role": m.Role, "parts": []map[string]any{{"text": m.Text}}}
	}
	v, err := e.toJS(h)
	if err != nil {
		return e.vm.NewArray()
	}
	return v
}

func (e *Engine) historyFromJS(v goja.Value) ([]Message, error) {
	var h jsHistory
	if err := e.fromJS(v, &h); err != nil {
		return nil, err
	}
	out := make([]Message, len(h))
	for i, m := range h {
		var sb strings.Builder
		for _, p := range m.Parts {
			sb.WriteString(p.Text)
		}
		out[i] = Message{Role: m.Role, Text: sb.String()}
	}
	return out, nil
}

// sessionValue is the legacy sessionId: 0 for the default session.
func sessionValue(vm *goja.Runtime, id string) goja.Value {
	if id == "" || id == "0" {
		return vm.ToValue(0)
	}
	return vm.ToValue(id)
}

func sessionID(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	s := v.String()
	if s == "0" {
		return ""
	}
	return s
}

// throw raises a JS Error from a host function.
func (e *Engine) throw(msg, code string) {
	v, err := e.call(e.helpers.Get("makeError"), e.vm.ToValue(msg), e.vm.ToValue(code))
	if err != nil {
		panic(e.vm.NewGoError(errors.New(msg)))
	}
	panic(v)
}

// resolved returns a promise already settled with v.
func (e *Engine) resolved(v goja.Value) goja.Value {
	p, resolve, _ := e.vm.NewPromise()
	_ = resolve(v)
	return e.vm.ToValue(p)
}

func (e *Engine) rejected(msg, code string) goja.Value {
	p, _, reject := e.vm.NewPromise()
	errv, err := e.call(e.helpers.Get("makeError"), e.vm.ToValue(msg), e.vm.ToValue(code))
	if err != nil {
		errv = e.vm.ToValue(msg)
	}
	_ = reject(errv)
	return e.vm.ToValue(p)
}

// truthy follows JS truthiness for a settings flag.
func truthy(v goja.Value) bool {
	return v != nil && v.ToBoolean()
}

func objectOf(v goja.Value) *goja.Object {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	obj, _ := v.(*goja.Object)
	return obj
}
