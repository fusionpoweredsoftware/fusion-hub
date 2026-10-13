package ailjs

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/dop251/goja"
)

// The fs and fs/promises modules. Paths are used as given (relative to the
// working directory, like node); readFile and copyFile fall back to the
// files shipped in the binary for app-lib/, example/, actors.json and
// mc-runner.mjs so the originals' "./actors.json" and "./mc-runner.mjs" work
// outside the old install directory.

func fsCode(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "ENOENT"
	case errors.Is(err, os.ErrExist):
		return "EEXIST"
	case errors.Is(err, os.ErrPermission):
		return "EACCES"
	}
	return "EIO"
}

func (e *Engine) fsThrow(op string, err error) {
	e.throw(fsCode(err)+": "+op+" failed, "+err.Error(), fsCode(err))
}

// fsOps are the operations shared by both modules; each returns a value or
// an error that becomes a thrown Error (sync) or a rejection (promises).
func (e *Engine) fsOps() map[string]func(call goja.FunctionCall) (goja.Value, error) {
	str := func(v goja.Value) string { return v.String() }
	return map[string]func(call goja.FunctionCall) (goja.Value, error){
		"readFile": func(call goja.FunctionCall) (goja.Value, error) {
			b, err := e.readFile(str(call.Argument(0)))
			if err != nil {
				return nil, err
			}
			return e.vm.ToValue(string(b)), nil
		},
		"writeFile": func(call goja.FunctionCall) (goja.Value, error) {
			return goja.Undefined(), os.WriteFile(str(call.Argument(0)), []byte(text(call.Argument(1))), 0o644)
		},
		"appendFile": func(call goja.FunctionCall) (goja.Value, error) {
			f, err := os.OpenFile(str(call.Argument(0)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			_, err = io.WriteString(f, text(call.Argument(1)))
			return goja.Undefined(), err
		},
		"copyFile": func(call goja.FunctionCall) (goja.Value, error) {
			b, err := e.readFile(str(call.Argument(0)))
			if err != nil {
				return nil, err
			}
			return goja.Undefined(), os.WriteFile(str(call.Argument(1)), b, 0o644)
		},
		"rename": func(call goja.FunctionCall) (goja.Value, error) {
			return goja.Undefined(), os.Rename(str(call.Argument(0)), str(call.Argument(1)))
		},
		"unlink": func(call goja.FunctionCall) (goja.Value, error) {
			return goja.Undefined(), os.Remove(str(call.Argument(0)))
		},
		"mkdir": func(call goja.FunctionCall) (goja.Value, error) {
			path := str(call.Argument(0))
			if opts := objectOf(call.Argument(1)); opts != nil && truthy(opts.Get("recursive")) {
				return goja.Undefined(), os.MkdirAll(path, 0o755)
			}
			return goja.Undefined(), os.Mkdir(path, 0o755)
		},
		"readdir": func(call goja.FunctionCall) (goja.Value, error) {
			entries, err := os.ReadDir(str(call.Argument(0)))
			if err != nil {
				return nil, err
			}
			names := make([]any, len(entries))
			for i, en := range entries {
				names[i] = en.Name()
			}
			return e.vm.ToValue(names), nil
		},
		"stat": func(call goja.FunctionCall) (goja.Value, error) {
			st, err := os.Stat(str(call.Argument(0)))
			if err != nil {
				return nil, err
			}
			obj := e.vm.NewObject()
			_ = obj.Set("size", st.Size())
			_ = obj.Set("mtimeMs", st.ModTime().UnixMilli())
			_ = obj.Set("isFile", func(goja.FunctionCall) goja.Value { return e.vm.ToValue(st.Mode().IsRegular()) })
			_ = obj.Set("isDirectory", func(goja.FunctionCall) goja.Value { return e.vm.ToValue(st.IsDir()) })
			return obj, nil
		},
		"access": func(call goja.FunctionCall) (goja.Value, error) {
			_, err := os.Stat(str(call.Argument(0)))
			return goja.Undefined(), err
		},
	}
}

func (e *Engine) fsPromisesModule() *goja.Object {
	fields := map[string]any{}
	for name, op := range e.fsOps() {
		op := op
		fields[name] = func(call goja.FunctionCall) goja.Value {
			v, err := op(call)
			if err != nil {
				return e.rejected(fsCode(err)+": "+err.Error(), fsCode(err))
			}
			return e.resolved(v)
		}
	}
	return withDefault(e.vm, fields)
}

func (e *Engine) fsModule() *goja.Object {
	fields := map[string]any{}
	for name, op := range e.fsOps() {
		op, name := op, name
		fields[name+"Sync"] = func(call goja.FunctionCall) goja.Value {
			v, err := op(call)
			if err != nil {
				e.fsThrow(name, err)
			}
			return v
		}
	}
	fields["existsSync"] = func(call goja.FunctionCall) goja.Value {
		_, err := os.Stat(call.Argument(0).String())
		return e.vm.ToValue(err == nil)
	}
	// Callback-style writeFile(path, data, encoding?, cb?) as used by example.ail.js.
	fields["writeFile"] = func(call goja.FunctionCall) goja.Value {
		err := os.WriteFile(call.Argument(0).String(), []byte(text(call.Argument(1))), 0o644)
		if cb, ok := goja.AssertFunction(call.Argument(len(call.Arguments) - 1)); ok && len(call.Arguments) > 2 {
			arg := goja.Value(goja.Null())
			if err != nil {
				e.fsThrow("writeFile", err)
			}
			_, _ = cb(goja.Undefined(), arg)
		}
		return goja.Undefined()
	}
	fields["promises"] = e.fsPromisesModule().Get("default")
	return withDefault(e.vm, fields)
}

// absPath makes a path absolute against the working directory (for messages).
func (e *Engine) absPath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(e.cwd, p)
}
