package ailjs

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
)

// setupGlobals installs the helpers and the globals node provides
// (console, process, setTimeout).
func (e *Engine) setupGlobals() error {
	_ = e.vm.Set("__cwd", func(goja.FunctionCall) goja.Value { return e.vm.ToValue(e.cwd) })
	src, err := fs.ReadFile(assets, "js/runtime.js")
	if err != nil {
		return err
	}
	v, err := e.vm.RunScript("runtime.js", string(src))
	if err != nil {
		return err
	}
	e.helpers = v.(*goja.Object)
	_ = e.helpers.Set("parse", e.vm.Get("JSON").(*goja.Object).Get("parse"))
	_ = e.vm.Set("console", e.console())
	_ = e.vm.Set("process", e.processObject())
	_ = e.vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			return goja.Undefined()
		}
		if ms := call.Argument(1).ToFloat(); ms > 0 {
			select {
			case <-time.After(time.Duration(ms * float64(time.Millisecond))):
			case <-e.ctx.Done():
			}
		}
		if _, err := fn(goja.Undefined(), call.Arguments[min(2, len(call.Arguments)):]...); err != nil {
			panic(err)
		}
		return e.vm.ToValue(1)
	})
	_ = e.vm.Set("clearTimeout", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	return nil
}

// builtin returns the exports object of a module provided by Go, or nil.
func (e *Engine) builtin(spec string) *goja.Object {
	switch spec {
	case "fusion-hub/cli":
		return e.cliModule()
	case "fusion-hub/paths":
		return e.pathsModule()
	case "fusion-hub/setup":
		return e.setupModule()
	case "fusion-hub/variables":
		return withDefault(e.vm, map[string]any{"app": map[string]any{"name": "fusion-hub"}, "currentAiApiName": "claude"})
	case "path":
		return defaultOnly(e.vm, e.helpers.Get("path"))
	case "fs/promises":
		return e.fsPromisesModule()
	case "fs":
		return e.fsModule()
	case "os":
		return e.osModule()
	case "crypto":
		return e.cryptoModule()
	case "console":
		return defaultOnly(e.vm, e.console())
	case "json5":
		return e.json5Module()
	case "color-json":
		return defaultOnly(e.vm, e.helpers.Get("colorJson"))
	}
	return nil
}

// defaultOnly wraps a value as a module whose default export it is, with the
// value's own properties as named exports.
func defaultOnly(vm *goja.Runtime, v goja.Value) *goja.Object {
	m := vm.NewObject()
	_ = m.Set("default", v)
	if obj, ok := v.(*goja.Object); ok {
		for _, k := range obj.Keys() {
			_ = m.Set(k, obj.Get(k))
		}
	}
	return m
}

func withDefault(vm *goja.Runtime, fields map[string]any) *goja.Object {
	m := vm.NewObject()
	def := vm.NewObject()
	for k, v := range fields {
		_ = m.Set(k, v)
		_ = def.Set(k, v)
	}
	_ = m.Set("default", def)
	return m
}

// text renders a JS value for console output the way node prints a single
// argument: strings as-is, Errors as "Error: message", objects via toString.
func text(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return ""
	}
	return v.String()
}

func joinArgs(args []goja.Value) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if a == nil || goja.IsUndefined(a) {
			parts[i] = "undefined"
		} else {
			parts[i] = a.String()
		}
	}
	return strings.Join(parts, " ")
}

func (e *Engine) console() *goja.Object {
	c := e.vm.NewObject()
	log := func(call goja.FunctionCall) goja.Value {
		e.host.Out().Print(joinArgs(call.Arguments))
		return goja.Undefined()
	}
	errf := func(call goja.FunctionCall) goja.Value {
		e.host.Out().Error("%s", joinArgs(call.Arguments))
		return goja.Undefined()
	}
	_ = c.Set("log", log)
	_ = c.Set("info", log)
	_ = c.Set("debug", log)
	_ = c.Set("error", errf)
	_ = c.Set("warn", errf)
	return c
}

func (e *Engine) processObject() *goja.Object {
	p := e.vm.NewObject()
	stdout := e.vm.NewObject()
	_ = stdout.Set("write", func(call goja.FunctionCall) goja.Value {
		_, _ = io.WriteString(e.host.Out(), text(call.Argument(0)))
		return e.vm.ToValue(true)
	})
	_ = p.Set("stdout", stdout)
	_ = p.Set("stderr", stdout)
	env := e.vm.NewObject()
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			_ = env.Set(k, v)
		}
	}
	_ = p.Set("env", env)
	_ = p.Set("platform", runtime.GOOS)
	_ = p.Set("cwd", func(goja.FunctionCall) goja.Value { return e.vm.ToValue(e.cwd) })
	_ = p.Set("on", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = p.Set("emitWarning", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = p.Set("exit", func(call goja.FunctionCall) goja.Value {
		e.throw("process.exit is not available inside an app", "")
		return nil
	})
	return p
}

// cliModule is fusion-hub/cli: the console output helpers with their
// timestamps, the REPL properties (commandMode is live) and executeCommand.
func (e *Engine) cliModule() *goja.Object {
	m := e.vm.NewObject()
	out := e.vm.NewObject()
	// event(eventType, eventData, stamp, seperator) in cli.mjs.
	event := func(kind string) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			data := text(call.Argument(0))
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
				sep := " - "
				if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
					sep = call.Argument(2).String()
				}
				line := call.Argument(1).String()
				if !goja.IsUndefined(call.Argument(0)) && data != "" {
					line += sep + data
				}
				if kind == "log" {
					e.host.Out().Print(line)
				} else {
					e.host.Out().Error("%s", line)
				}
				return goja.Undefined()
			}
			switch kind {
			case "log":
				e.host.Out().Log("%s", data)
			case "warn":
				e.host.Out().Warn("%s", data)
			default:
				e.host.Out().Error("%s", data)
			}
			return goja.Undefined()
		}
	}
	write := func(call goja.FunctionCall) goja.Value {
		_, _ = io.WriteString(e.host.Out(), text(call.Argument(0)))
		return goja.Undefined()
	}
	print := func(call goja.FunctionCall) goja.Value {
		_, _ = io.WriteString(e.host.Out(), text(call.Argument(0))+"\n")
		return goja.Undefined()
	}
	_ = out.Set("log", event("log"))
	_ = out.Set("error", event("error"))
	_ = out.Set("warn", event("warn"))
	_ = out.Set("write", write)
	_ = out.Set("print", print)

	props := e.vm.NewObject()
	_ = props.Set("isPromptRunning", true)
	mode := func(goja.FunctionCall) goja.Value {
		if m := e.host.Mode(); m != "" {
			return e.vm.ToValue(m)
		}
		return goja.Null()
	}
	setMode := func(call goja.FunctionCall) goja.Value {
		v := call.Argument(0)
		if goja.IsNull(v) || goja.IsUndefined(v) || v.String() == "" {
			e.host.SetMode("")
		} else {
			e.host.SetMode(v.String())
		}
		return goja.Undefined()
	}
	_ = props.DefineAccessorProperty("commandMode", e.vm.ToValue(mode), e.vm.ToValue(setMode), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = props.Set("lastCommandMode", goja.Null())
	_ = props.Set("lockModeTo", goja.Null())
	_ = props.Set("commandPath", goja.Null())
	_ = props.Set("commandModuleRoot", "fusion-hub/command")
	_ = props.Set("import", e.vm.ToValue(e.importFn))

	errs := e.vm.NewObject()
	msg := func(s string) func(goja.FunctionCall) goja.Value {
		return func(goja.FunctionCall) goja.Value {
			e.host.Out().Error("%s", s)
			return goja.Undefined()
		}
	}
	plural := func(n goja.Value) string {
		if n.ToInteger() > 1 {
			return "s"
		}
		return ""
	}
	_ = errs.Set("commandNotFoundError", msg("Command not found."))
	_ = errs.Set("noCommandSuppliedError", msg("Unable to process command: No command supplied."))
	_ = errs.Set("invalidStartTypeError", msg("Unable to process command: Not a valid start type."))
	_ = errs.Set("commandNotStartableError", msg("Unable to process command: Not a valid command mode to start."))
	_ = errs.Set("noArgumentsAllowedError", msg("Command takes no arguments."))
	_ = errs.Set("templateNotFoundError", msg("Template not found"))
	_ = errs.Set("noArgumentsSuppliedError", func(call goja.FunctionCall) goja.Value {
		e.host.Out().Error("Command takes %s argument%s.", call.Argument(0).String(), plural(call.Argument(0)))
		return goja.Undefined()
	})
	_ = errs.Set("argumentsLimitError", func(call goja.FunctionCall) goja.Value {
		e.host.Out().Error("Command limited to %s argument%s.", call.Argument(0).String(), plural(call.Argument(0)))
		return goja.Undefined()
	})

	exec := e.vm.ToValue(e.executeCommand)
	def := e.vm.NewObject()
	_ = def.Set("properties", props)
	_ = def.Set("init", func(goja.FunctionCall) goja.Value { return e.resolved(goja.Undefined()) })
	_ = def.Set("errors", errs)
	_ = def.Set("output", out)
	_ = def.Set("executeCommand", exec)
	_ = m.Set("default", def)
	_ = m.Set("properties", props)
	_ = m.Set("output", out)
	_ = m.Set("executeCommand", exec)
	for _, k := range []string{"log", "error", "warn", "write", "print"} {
		_ = m.Set(k, out.Get(k))
	}
	return m
}

// pathsModule is fusion-hub/paths. currentAppCacheDir is chosen so that the
// original expandFilePath (path.join(os.homedir(), "." + currentAppCacheDir))
// lands on this installation's root, even when FUSION_HUB_HOME moved it.
func (e *Engine) pathsModule() *goja.Object {
	paths := e.host.Paths()
	home, _ := os.UserHomeDir()
	cache := "fusion-hub/fusion-hub"
	if rel, err := filepath.Rel(home, paths.Root); err == nil {
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, ".") {
			rel = "./" + rel
		}
		cache = rel[1:]
	}
	async := func(f func() string) func(goja.FunctionCall) goja.Value {
		return func(goja.FunctionCall) goja.Value { return e.resolved(e.vm.ToValue(f())) }
	}
	fields := map[string]any{
		"cliRoot":                       e.cwd,
		"appCacheRootDir":               "fusion-hub",
		"currentAppCacheDir":            cache,
		"defaultContextDir":             config.Histories,
		"commandModuleRoot":             "fusion-hub/command",
		"defaultCommandPath":            "fusion-hub/command",
		"getAppCacheDirPath":            async(func() string { return paths.Root }),
		"getChatHistoryDirPath":         async(func() string { return paths.Dir(config.Histories) }),
		"getChatTemplateDirPath":        async(func() string { return paths.Dir(config.Templates) }),
		"getCliHistoryFilePath":         async(paths.CLIHistoryFile),
		"getCommandModeHistoryFilePath": async(paths.ModeHistoryFile),
		"getSetupSettingsFilePath":      async(paths.SettingsFile),
		"getCurrentChatHistoryFilePath": async(func() string { return filepath.Join(paths.Dir(config.Histories), ".ai-chat-history.json") }),
	}
	return withDefault(e.vm, fields)
}

// setupModule is fusion-hub/setup: a snapshot of the settings file.
func (e *Engine) setupModule() *goja.Object {
	settings, err := e.toJS(e.host.Settings())
	if err != nil || goja.IsUndefined(settings) {
		settings = e.vm.NewObject()
	}
	noop := func(goja.FunctionCall) goja.Value { return e.resolved(goja.Undefined()) }
	def := e.vm.NewObject()
	_ = def.Set("settings", settings)
	_ = def.Set("init", noop)
	_ = def.Set("pullSetupSettings", noop)
	_ = def.Set("pushSetupSettings", noop)
	return defaultOnly(e.vm, def)
}

func (e *Engine) osModule() *goja.Object {
	home, _ := os.UserHomeDir()
	return withDefault(e.vm, map[string]any{
		"homedir":  func(goja.FunctionCall) goja.Value { return e.vm.ToValue(home) },
		"platform": func(goja.FunctionCall) goja.Value { return e.vm.ToValue(runtime.GOOS) },
		"EOL":      "\n",
	})
}

func (e *Engine) cryptoModule() *goja.Object {
	createHash := func(call goja.FunctionCall) goja.Value {
		var h hash.Hash
		switch strings.ToLower(call.Argument(0).String()) {
		case "md5":
			h = md5.New()
		case "sha256":
			h = sha256.New()
		default:
			e.throw("Digest method not supported", "ERR_CRYPTO_INVALID_DIGEST")
		}
		obj := e.vm.NewObject()
		_ = obj.Set("update", func(c goja.FunctionCall) goja.Value {
			_, _ = io.WriteString(h, c.Argument(0).String())
			return obj
		})
		_ = obj.Set("digest", func(c goja.FunctionCall) goja.Value {
			sum := h.Sum(nil)
			switch c.Argument(0).String() {
			case "base64":
				return e.vm.ToValue(base64.StdEncoding.EncodeToString(sum))
			default:
				return e.vm.ToValue(hex.EncodeToString(sum))
			}
		})
		return obj
	}
	return withDefault(e.vm, map[string]any{"createHash": createHash})
}

// json5Module evaluates the json5 library (dist/index.min.js, MIT).
func (e *Engine) json5Module() *goja.Object {
	src, err := fs.ReadFile(assets, "js/json5.min.js")
	if err != nil {
		e.throw(err.Error(), "")
	}
	fn, err := e.vm.RunScript("json5.min.js", "(function(module, exports){"+string(src)+"\n;return module.exports;})")
	if err != nil {
		e.throw(fmt.Sprintf("json5: %v", err), "")
	}
	module := e.vm.NewObject()
	exports := e.vm.NewObject()
	_ = module.Set("exports", exports)
	v, err := e.call(fn, module, exports)
	if err != nil {
		e.throw(fmt.Sprintf("json5: %v", err), "")
	}
	return defaultOnly(e.vm, v)
}
