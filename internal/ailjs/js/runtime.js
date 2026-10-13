// Helpers for the embedded JS runtime: node's path module, the color-json
// package and small functions the Go side calls on JS values.
(function () {
  const path = {
    sep: '/',
    normalize(p) {
      if (p === '') return '.';
      const abs = p[0] === '/';
      const trail = p[p.length - 1] === '/';
      const out = [];
      for (const seg of p.split('/')) {
        if (seg === '' || seg === '.') continue;
        if (seg === '..') {
          if (out.length && out[out.length - 1] !== '..') out.pop();
          else if (!abs) out.push('..');
        } else out.push(seg);
      }
      let r = out.join('/');
      if (!r && !abs) r = '.';
      if (r && trail) r += '/';
      return (abs ? '/' : '') + r;
    },
    join(...parts) {
      const joined = parts.filter((p) => p !== '').join('/');
      return joined === '' ? '.' : path.normalize(joined);
    },
    basename(p, ext) {
      let b = p.replace(/\/+$/, '');
      b = b.slice(b.lastIndexOf('/') + 1);
      if (ext && b.endsWith(ext) && b !== ext) b = b.slice(0, -ext.length);
      return b;
    },
    dirname(p) {
      const s = p.replace(/\/+$/, '');
      const i = s.lastIndexOf('/');
      if (i < 0) return '.';
      if (i === 0) return '/';
      return s.slice(0, i);
    },
    extname(p) {
      const b = path.basename(p);
      const i = b.lastIndexOf('.');
      return i <= 0 ? '' : b.slice(i);
    },
    resolve(...parts) {
      let r = '';
      for (const p of parts) r = p[0] === '/' ? p : (r ? r + '/' + p : p);
      if (r[0] !== '/') r = __cwd() + '/' + r;
      return path.normalize(r).replace(/\/$/, '') || '/';
    },
    isAbsolute(p) { return p[0] === '/'; },
  };

  // color-json: JSON.stringify with ANSI colours for keys and values.
  const colorJson = (json, replacer, space) => {
    const text = JSON.stringify(json, replacer, space);
    if (text === undefined) return text;
    return text.replace(/("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?/g, (m, str, colon, kw) => {
      if (str !== undefined) return colon ? '\x1b[35m' + str + '\x1b[0m' + colon : '\x1b[32m' + str + '\x1b[0m';
      if (kw !== undefined) return '\x1b[33m' + m + '\x1b[0m';
      return '\x1b[36m' + m + '\x1b[0m';
    });
  };

  return {
    path,
    colorJson,
    // Chat history helpers, operating on the legacy {role, parts:[{text}]} shape.
    pushTurn(arr, user, model) {
      arr.push({ role: 'user', parts: [{ text: user }] }, { role: 'model', parts: [{ text: model }] });
    },
    // clean(): drop trailing pairs whose model turn is empty.
    cleanHistory(arr) {
      while (arr.length > 0 && JSON.stringify(arr.slice(-1)) == '[{"role":"model","parts":[{"text":""}]}]')
        arr.splice(-2, 2);
    },
    toArray(v) { return Array.isArray(v) ? v : []; },
    keys(o) { return Object.keys(o); },
    stringify(v) { return JSON.stringify(v); },
    makeError(message, code) { const e = new Error(message); if (code) e.code = code; return e; },
    renameImports: null,
  };
})()
