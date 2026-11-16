// Package guide holds the tour authoring guide `docent guide` prints.
package guide

import _ "embed"

// Markdown is the authoring guide, written to be pasted into an agent's context.
//
//go:embed guide.md
var Markdown string
