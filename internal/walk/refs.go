package walk

import (
	"context"
	"fmt"
	"strings"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
)

// How a ref is being read.
const (
	// RefWorkingTree: files are read as they are on disk.
	RefWorkingTree RefKind = "working-tree"
	// RefTracking: the ref names a branch, read at its tip as of the last
	// fetch, so the tour follows the code.
	RefTracking RefKind = "tracking"
	// RefPinned: the ref names a commit or a tag, read exactly there.
	RefPinned RefKind = "pinned"
)

type (
	// RefKind is how a ref is being read.
	RefKind string

	// Ref is a tour's ref resolved against a local repository.
	Ref struct {
		_ struct{} `json:"-"`
		// Requested is the ref as the tour wrote it.
		Requested string  `json:"requested"`
		Kind      RefKind `json:"kind"`
		// Name is what is actually being read, for display: "origin/main".
		Name string `json:"name"`
		// Commit is the full SHA being read, or "" for the working tree or a
		// ref that couldn't be resolved.
		Commit string `json:"commit"`
		// Problem and Detail describe a fallback or a failure.
		Problem diag.Code `json:"problem"`
		Detail  string    `json:"detail"`
	}
)

// resolveRef works out what to read for a ref in the repository at root.
//
// A ref that names a branch is tracking: it resolves to the remote-tracking
// branch when there is one, since that is the tip as of the last fetch and is
// untouched by local work. Anything else that resolves is pinned. A branch
// that has gone falls back to the default branch; a commit that isn't here is
// reported missing, because nothing else could stand in for it.
func (w *walker) resolveRef(ctx context.Context, root, ref string) *Ref {
	key := root + "\x00" + ref

	w.mu.Lock()
	cached, ok := w.refs[key]
	workingTree := w.workingTree
	w.mu.Unlock()

	if workingTree || ref == "" || w.git == nil {
		return &Ref{Requested: ref, Kind: RefWorkingTree}
	}

	if ok {
		return cached
	}

	out := w.lookupRef(ctx, root, ref)

	w.mu.Lock()
	w.refs[key] = out
	w.mu.Unlock()

	return out
}

func (w *walker) lookupRef(ctx context.Context, root, ref string) *Ref {
	out := &Ref{Requested: ref, Kind: RefWorkingTree}

	if _, _, err := w.git.Locate(ctx, root); err != nil {
		// A plain folder has no history to read from. Showing what is on disk
		// is the only honest option, and it is what the reader would expect.
		out.Detail = "not a git repository, so files are read as they are on disk"
		return out
	}

	for _, candidate := range []string{"refs/remotes/origin/" + ref, "refs/heads/" + ref} {
		if !w.git.RefExists(ctx, root, candidate) {
			continue
		}

		if sha, err := w.git.Commit(ctx, root, candidate); err == nil {
			out.Kind, out.Commit = RefTracking, sha
			out.Name = strings.TrimPrefix(strings.TrimPrefix(candidate, "refs/remotes/"), "refs/heads/")

			return out
		}
	}

	if sha, err := w.git.Commit(ctx, root, ref); err == nil {
		out.Kind, out.Commit, out.Name = RefPinned, sha, ref
		return out
	}

	if gitsrc.LooksLikeSHA(ref) {
		out.Kind = RefPinned
		out.Problem = diag.CodeRefMissing
		out.Detail = fmt.Sprintf("commit %s isn't in the local repository", ref)

		return out
	}

	if fallback := w.git.DefaultBranch(ctx, root); fallback != "" {
		if sha, err := w.git.Commit(ctx, root, fallback); err == nil {
			out.Kind, out.Commit = RefTracking, sha
			out.Name = strings.TrimPrefix(strings.TrimPrefix(fallback, "refs/remotes/"), "refs/heads/")
			out.Problem = diag.CodeRefFallback
			out.Detail = fmt.Sprintf("%q no longer exists, so this is %s", ref, out.Name)

			return out
		}
	}

	out.Kind = RefPinned
	out.Problem = diag.CodeRefMissing
	out.Detail = fmt.Sprintf("%q isn't in the local repository", ref)

	return out
}
