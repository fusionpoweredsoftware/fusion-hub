package ailjs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

// The module loader. The original files are ES modules; the engine runs
// scripts, so each module is rewritten into an async function taking an
// import function and an exports object:
//
//	import cli, { output as o } from 'fusion-hub/cli'  ->  const __m0 = await __import("fusion-hub/cli"); const cli = __m0.default; const { output: o } = __m0;
//	export async function process()                   ->  async function process()      (+ __exports.process = process)
//	export default { ... }                              ->  __exports.default = { ... }
//	await import("x")                                   ->  await __import("x")
//
// Imports are awaited in order, which for these modules is the same as the
// hoisted evaluation order node uses.

var (
	reImport        = regexp.MustCompile(`(?m)^[ \t]*import\s+(?:([A-Za-z_$][\w$]*)\s*,\s*)?(?:([A-Za-z_$][\w$]*)|\{([^}]*)\})\s+from\s+['"]([^'"]+)['"][ \t]*;?`)
	reDynImport     = regexp.MustCompile(`(^|[^\w$.])import\(`)
	reExportDefault = regexp.MustCompile(`(?m)^[ \t]*export\s+default\s+`)
	reExportFn      = regexp.MustCompile(`(?m)^[ \t]*export\s+((?:async\s+)?function\s+)([A-Za-z_$][\w$]*)`)
	reExportVar     = regexp.MustCompile(`(?m)^[ \t]*export\s+(const|let|var)\s+([A-Za-z_$][\w$]*)`)
	reExportDestr   = regexp.MustCompile(`(?m)^[ \t]*export\s+(const|let|var)\s+\{([^}]*)\}`)
	reAs            = regexp.MustCompile(`\bas\b`)
)

// transform rewrites one ES module into the body of the async wrapper.
func transform(src string) string {
	n := 0
	src = reImport.ReplaceAllStringFunc(src, func(m string) string {
		g := reImport.FindStringSubmatch(m)
		def, def2, names, spec := g[1], g[2], g[3], g[4]
		if def == "" {
			def = def2
		}
		v := fmt.Sprintf("__m%d", n)
		n++
		var sb strings.Builder
		fmt.Fprintf(&sb, "const %s = await __import(%q);", v, spec)
		if def != "" {
			fmt.Fprintf(&sb, " const %s = %s.default;", def, v)
		}
		if strings.TrimSpace(names) != "" {
			fmt.Fprintf(&sb, " const {%s} = %s;", reAs.ReplaceAllString(names, ":"), v)
		}
		return sb.String()
	})
	src = reDynImport.ReplaceAllString(src, "${1}__import(")

	var exported []string
	seen := map[string]bool{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if i := strings.Index(name, ":"); i >= 0 {
			name = strings.TrimSpace(name[i+1:])
		}
		if name != "" && !seen[name] {
			seen[name] = true
			exported = append(exported, name)
		}
	}
	src = reExportDefault.ReplaceAllString(src, "__exports.default = ")
	src = reExportFn.ReplaceAllStringFunc(src, func(m string) string {
		g := reExportFn.FindStringSubmatch(m)
		add(g[2])
		return g[1] + g[2]
	})
	src = reExportDestr.ReplaceAllStringFunc(src, func(m string) string {
		g := reExportDestr.FindStringSubmatch(m)
		for _, name := range strings.Split(g[2], ",") {
			add(name)
		}
		return g[1] + " {" + g[2] + "}"
	})
	src = reExportVar.ReplaceAllStringFunc(src, func(m string) string {
		g := reExportVar.FindStringSubmatch(m)
		add(g[2])
		return g[1] + " " + g[2]
	})
	var sb strings.Builder
	sb.WriteString(src)
	sb.WriteString("\n;")
	for _, name := range exported {
		fmt.Fprintf(&sb, "\n__exports.%s = %s;", name, name)
	}
	return sb.String()
}

// evaluate runs a module's source and returns the promise of its exports.
func (e *Engine) evaluate(name, src string) goja.Value {
	wrapped := "(async function(__import, __exports, __filename) {\n" + transform(src) + "\nreturn __exports;\n})"
	fn, err := e.vm.RunScript(name, wrapped)
	if err != nil {
		return e.rejected(fmt.Sprintf("%s: %v", name, jsErr(err)), "ERR_MODULE_PARSE")
	}
	f, _ := goja.AssertFunction(fn)
	p, err := f(goja.Undefined(), e.vm.ToValue(e.importFn), e.vm.NewObject(), e.vm.ToValue(name))
	if err != nil {
		return e.rejected(jsErr(err).Error(), "")
	}
	return p
}

// importFn is the JS-facing __import(spec): a promise of the module's exports.
func (e *Engine) importFn(call goja.FunctionCall) goja.Value {
	return e.importModule(call.Argument(0).String())
}

// importModule resolves a module specifier. "?update=..." (reloadModule)
// bypasses the cache so app libraries are re-read every run, as in node.
func (e *Engine) importModule(spec string) goja.Value {
	reload := false
	if i := strings.Index(spec, "?"); i >= 0 {
		spec, reload = spec[:i], true
	}
	if !reload {
		if p, ok := e.modules[spec]; ok {
			return p
		}
	}
	p := e.load(spec)
	if !reload {
		e.modules[spec] = p
	}
	return p
}

func (e *Engine) load(spec string) goja.Value {
	if obj := e.builtin(spec); obj != nil {
		return e.resolved(obj)
	}
	switch spec {
	case "fusion-hub/functions", "fusion-hub/colors", "fusion-hub/command/multi-command":
		src, err := fs.ReadFile(assets, "js/legacy/"+strings.TrimPrefix(spec, "fusion-hub/command/")+".mjs")
		if err != nil {
			src, err = fs.ReadFile(assets, "js/legacy/"+strings.TrimPrefix(spec, "fusion-hub/")+".mjs")
		}
		if err != nil {
			return e.rejected("Cannot find module '"+spec+"'", "ERR_MODULE_NOT_FOUND")
		}
		return e.evaluate(spec, string(src))
	case "fusion-hub/mc-process":
		src, _ := fs.ReadFile(assets, "js/mc-process.mjs")
		return e.evaluate(spec, string(src))
	}
	if strings.HasPrefix(spec, "fusion-hub/app-lib/") {
		name := strings.TrimPrefix(spec, "fusion-hub/")
		if !strings.HasSuffix(name, ".mjs") {
			name += ".mjs"
		}
		src, err := e.readFile(name)
		if err != nil {
			return e.rejected("Cannot find module '"+spec+"'", "ERR_MODULE_NOT_FOUND")
		}
		return e.evaluate(spec, string(src))
	}
	if strings.HasPrefix(spec, "fusion-hub/") {
		return e.rejected("Cannot find module '"+spec+"'", "ERR_MODULE_NOT_FOUND")
	}
	// A file: the generated .<app>.mjs module, or anything else by path.
	src, err := e.readFile(spec)
	if err != nil {
		return e.rejected("Cannot find module '"+spec+"'", "ERR_MODULE_NOT_FOUND")
	}
	return e.evaluate(spec, string(src))
}

// readFile reads a file from disk, falling back for relative paths to the
// app-lib/, example/ and actors.json shipped in the binary (the original
// install directory) and to the runtime's own mc-runner.mjs.
func (e *Engine) readFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil || filepath.IsAbs(path) {
		return b, err
	}
	rel := filepath.ToSlash(filepath.Clean(path))
	for _, prefix := range []string{"app-lib/", "example/"} {
		if strings.HasPrefix(rel, prefix) {
			if eb, eerr := fs.ReadFile(e.legacy, rel); eerr == nil {
				return eb, nil
			}
		}
	}
	// Bare app-lib names ("fusion-hub/app-lib/x" -> "app-lib/x.mjs") also
	// live under <root>/app-lib so users can add their own.
	if strings.HasPrefix(rel, "app-lib/") {
		if ub, uerr := os.ReadFile(filepath.Join(e.host.Paths().Root, rel)); uerr == nil {
			return ub, nil
		}
	}
	switch rel {
	case "actors.json":
		if eb, eerr := fs.ReadFile(e.legacy, rel); eerr == nil {
			return eb, nil
		}
	case "mc-runner.mjs":
		return fs.ReadFile(assets, "js/legacy/mc-runner.mjs")
	}
	return nil, err
}

// Embedded returns a file shipped with the binary (example/..., app-lib/...),
// or false.
func (e *Engine) Embedded(rel string) ([]byte, bool) {
	b, err := fs.ReadFile(e.legacy, filepath.ToSlash(rel))
	return b, err == nil
}
