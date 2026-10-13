// Package fusionhub embeds the AIL apps (example/), app libraries (app-lib/)
// and actor profiles (actors.json) that ship with the binary. They are the
// original JavaScript files, unchanged; see internal/ailjs for how they run.
package fusionhub

import "embed"

//go:embed example app-lib actors.json
var Legacy embed.FS
