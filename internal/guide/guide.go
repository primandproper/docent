// Package guide holds the tour authoring guide `docent guide` prints.
package guide

import _ "embed"

// Markdown is the authoring guide, written to be pasted into an agent's context.
//
//go:embed guide.md
var Markdown string

// skillHeader is the frontmatter that makes the guide an agent skill.
const skillHeader = `---
name: docent-tours
description: Write or refresh a Docent code tour (a .tour file that walks a reader through code across one or more repos). Use when asked for a code tour, a walkthrough of a feature, or to update tours that have fallen behind the code.
---

`

// AgentsSnippet is a short pointer for a repository's AGENTS.md.
const AgentsSnippet = `## Code tours

This repository's guided tours live in ` + "`.tours/`" + ` and are walked with [Docent](https://github.com/primandproper/docent).

- To write or update a tour, run ` + "`docent guide`" + ` and follow it exactly.
- Check every anchor with ` + "`docent resolve <tour> --step <id>`" + `.
- A tour isn't done until ` + "`docent validate --strict --json <tour>`" + ` prints no diagnostics.
- ` + "`docent stale --json`" + ` lists tours the code has moved away from, with the diffs to work from.
`

// Skill is the guide as an agent skill file.
func Skill() string {
	return skillHeader + Markdown
}
