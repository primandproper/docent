package links

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/primandproper/docent/internal/workspace"
)

// GitHubHost is the only forge Docent builds permalinks for.
const GitHubHost = "github.com"

// Permalink builds a GitHub URL for lines of a file at a commit. It is built
// from the commit SHA, never a branch name, so the link keeps pointing at the
// code the reader saw. It returns "" for a remote that isn't on github.com.
// path is relative to the repository's top level; a zero startLine links to
// the file as a whole.
func Permalink(remote, sha, path string, startLine, endLine int) string {
	host, repo, ok := strings.Cut(workspace.NormalizeRemote(remote), "/")
	if !ok || host != GitHubHost || strings.Count(repo, "/") != 1 || sha == "" {
		return ""
	}

	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}

	link := fmt.Sprintf("https://%s/%s/blob/%s/%s", GitHubHost, repo, sha, strings.Join(segments, "/"))

	switch {
	case startLine <= 0:
		return link
	case endLine <= startLine:
		return fmt.Sprintf("%s#L%d", link, startLine)
	default:
		return fmt.Sprintf("%s#L%d-L%d", link, startLine, endLine)
	}
}
