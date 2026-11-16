package links

import (
	"testing"

	"github.com/shoenig/test"
)

func TestPermalink(T *testing.T) {
	T.Parallel()

	T.Run("links lines of a file at a commit", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, "https://github.com/acme/api/blob/3f9c2e1/internal/http/items.go#L26-L46",
			Permalink("git@github.com:acme/api.git", "3f9c2e1", "internal/http/items.go", 26, 46))
		test.Eq(t, "https://github.com/acme/api/blob/3f9c2e1/a%20b/c%23d.go#L7",
			Permalink("https://github.com/acme/api", "3f9c2e1", "/a b/c#d.go", 7, 7))
		test.Eq(t, "https://github.com/acme/api/blob/3f9c2e1/README.md",
			Permalink("https://github.com/acme/api", "3f9c2e1", "README.md", 0, 0))
	})

	T.Run("only for github.com, and only with a commit", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, "", Permalink("git@gitlab.com:acme/api.git", "3f9c2e1", "a.go", 1, 1))
		test.Eq(t, "", Permalink("git@github.example.com:acme/api.git", "3f9c2e1", "a.go", 1, 1))
		test.Eq(t, "", Permalink("git@github.com:acme/api.git", "", "a.go", 1, 1))
		test.Eq(t, "", Permalink("", "3f9c2e1", "a.go", 1, 1))
		test.Eq(t, "", Permalink("https://github.com/acme", "3f9c2e1", "a.go", 1, 1))
	})
}
