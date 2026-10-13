package ailjs

import (
	"bytes"
	"io"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

var leadingWord = regexp.MustCompile(`^[^ ]* `)

// executeCommand is cli.executeCommand as apps see it: evalCommand() in
// multi-command.mjs calls it with a block's text and {isThrowing,
// aiChatSettings, isCommandStartLogged}. As in the original, the REPL's
// command mode decides where a block goes: in CLI mode the first word is
// the command ("say ...", "start ...", "chat-with-ai ..."); once a block
// has run "start chat-with-ai", every later block is sent to the AI whole,
// which is how the examples write their prompts without a command word.
func (e *Engine) executeCommand(call goja.FunctionCall) goja.Value {
	line, ok := call.Argument(0).Export().(string)
	if !ok {
		return e.resolved(goja.Undefined())
	}
	settings := objectOf(call.Argument(1))
	if settings == nil {
		settings = e.vm.NewObject()
	}
	args, err := e.getArgs(line)
	if err != nil {
		return e.rejected(err.Error(), "")
	}
	if len(args) == 0 || args[0] == "" {
		return e.resolved(goja.Undefined())
	}
	command, joined := e.host.Mode(), line
	if command == "" {
		command = strings.ToLower(args[0])
		joined = strings.TrimSpace(leadingWord.ReplaceAllString(strings.TrimSpace(line)+" ", ""))
	}

	var result goja.Value = goja.Undefined()
	switch command {
	case "chat-with-ai":
		result, err = e.chatWithAI(joined, settings)
	default:
		err = e.host.Execute(e.ctx, strings.TrimSpace(line), truthy(settings.Get("isCommandStartLogged")))
	}
	if err != nil {
		if truthy(settings.Get("isThrowing")) {
			return e.rejected(err.Error(), "")
		}
		e.host.Out().Error("%s", err.Error())
		return e.resolved(goja.Undefined())
	}
	return e.resolved(result)
}

// chatWithAI is command/chat-with-ai.mjs's process() plus the chat() of the
// AI module, for a call from an app: the app's own history object is the
// conversation, the exchange is pushed onto it and the whole object (every
// channel) is written to the session's history file.
func (e *Engine) chatWithAI(text string, settings *goja.Object) (goja.Value, error) {
	ai := objectOf(settings.Get("aiChatSettings"))
	if ai == nil {
		ai = e.vm.NewObject()
	}
	sid := sessionID(ai.Get("sessionId"))
	if text == "" {
		return goja.Undefined(), e.host.StartChat(e.ctx, sid, true)
	}
	if err := e.host.StartChat(e.ctx, sid, false); err != nil {
		return goja.Undefined(), err
	}
	channel := "0"
	if c := ai.Get("channel"); c != nil && !goja.IsUndefined(c) && !goja.IsNull(c) {
		channel = c.String()
	}
	chatHistory := objectOf(ai.Get("chatHistory"))
	if chatHistory == nil {
		chatHistory = e.vm.NewObject()
	}
	arr := objectOf(chatHistory.Get(channel))
	if arr == nil || arr.ClassName() != "Array" {
		arr = e.historyToJS(e.host.History(sid)).(*goja.Object)
		_ = chatHistory.Set(channel, arr)
	}
	if _, err := e.call(e.helpers.Get("cleanHistory"), arr); err != nil {
		return nil, err
	}
	msgs, err := e.historyFromJS(arr)
	if err != nil {
		return nil, err
	}

	// chatStdout: the app's writer, or the console. With isStreaming off
	// (always, from evalCommand) the original wrote "\nai> " + the whole
	// reply once it had arrived instead of streaming it.
	var stream io.Writer = e.host.Out()
	if so := objectOf(ai.Get("chatStdout")); so != nil {
		if w, ok := goja.AssertFunction(so.Get("write")); ok {
			stream = jsWriter{e: e, this: so, write: w}
		}
	}
	streaming := truthy(ai.Get("isStreaming"))
	var buf bytes.Buffer
	target := stream
	if !streaming {
		target = &buf
	}
	reply, err := e.host.Chat(e.ctx, sid, msgs, text, target)
	if err != nil {
		e.host.Out().Error("%s", err.Error())
		return goja.Null(), nil
	}
	if !streaming {
		out := buf.String()
		if reply != "" {
			out = "\nai> " + out
		}
		_, _ = io.WriteString(stream, out)
	}
	if reply != "" {
		if _, err := e.call(e.helpers.Get("pushTurn"), arr, e.vm.ToValue(text), e.vm.ToValue(reply)); err != nil {
			return nil, err
		}
	}
	persistent := ai.Get("isPersistent")
	if persistent == nil || goja.IsUndefined(persistent) || persistent.ToBoolean() {
		channels := map[string][]Message{}
		for _, k := range chatHistory.Keys() {
			ch, err := e.historyFromJS(chatHistory.Get(k))
			if err != nil {
				return nil, err
			}
			channels[k] = ch
		}
		if err := e.host.Persist(sid, channels); err != nil {
			return nil, err
		}
	}
	return e.vm.ToValue(reply), nil
}

// jsWriter streams reply text to a JS object's write(text).
type jsWriter struct {
	e     *Engine
	this  *goja.Object
	write goja.Callable
}

func (w jsWriter) Write(p []byte) (int, error) {
	if _, err := w.write(w.this, w.e.vm.ToValue(string(p))); err != nil {
		return 0, jsErr(err)
	}
	return len(p), nil
}
