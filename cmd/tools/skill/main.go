// Command skill writes the authoring guide out in the forms agents pick up
// from a repository: an agent skill, and a snippet to paste into AGENTS.md.
// Both are generated from internal/guide so there is one guide to keep true.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/primandproper/docent/internal/guide"
)

func main() {
	dir := "skill"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	files := map[string]string{
		filepath.Join(dir, "docent-tours", "SKILL.md"): guide.Skill(),
		filepath.Join(dir, "AGENTS.snippet.md"):        guide.AgentsSnippet,
	}

	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			fmt.Fprintf(os.Stderr, "skill: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "skill: %v\n", err)
			os.Exit(1)
		}
	}
}
