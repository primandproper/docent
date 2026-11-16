package guide

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/primandproper/docent/internal/visual"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func TestShippedCopies(T *testing.T) {
	T.Parallel()

	T.Run("the skill and the AGENTS snippet in the repo match the guide", func(t *testing.T) {
		t.Parallel()

		for path, want := range map[string]string{
			filepath.Join("..", "..", "skill", "docent-tours", "SKILL.md"): Skill(),
			filepath.Join("..", "..", "skill", "AGENTS.snippet.md"):        AgentsSnippet,
		} {
			got, err := os.ReadFile(path)
			must.NoError(t, err)
			test.Eq(t, want, string(got), test.Sprintf("%s is out of date: run `make skill`", path))
		}
	})
}

func TestExamples(T *testing.T) {
	T.Parallel()

	T.Run("every visual the guide shows is a valid one", func(t *testing.T) {
		t.Parallel()

		checked := 0

		for _, fence := range visual.Fences(Markdown) {
			// The guide nests its examples in four-backtick fences.
			for _, inner := range visual.Fences(fence.Body) {
				if !strings.HasPrefix(inner.Lang, visual.FencePrefix) || inner.Lang == visual.LinksFence {
					continue
				}

				value, err := visual.FromFence(&inner)
				must.NoError(t, err)

				problems, err := visual.Validate(value)
				must.NoError(t, err)
				test.Len(t, 0, problems, test.Sprintf("%s: %+v", inner.Lang, problems))

				checked++
			}
		}

		test.Positive(t, checked)
	})
}
