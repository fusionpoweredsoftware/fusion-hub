package ailjs

import "embed"

// assets holds the JavaScript the runtime is made of: the original
// functions.mjs, colors.mjs, multi-command.mjs and mc-runner.mjs under
// legacy/, the preprocessor split out of multi-command.mjs (mc-process.mjs),
// the json5 library and the runtime helpers.
//
//go:embed js
var assets embed.FS
