// Package examples ships the built-in AIL apps inside the binary, so
// "start new app-using-ai english2spanish" works from anywhere.
package examples

import "embed"

//go:embed *.ail.yaml
var FS embed.FS
