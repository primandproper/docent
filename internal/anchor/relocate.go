package anchor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
)

const (
	// HashPrefix marks a contentHash's algorithm.
	HashPrefix = "sha256:"
	// fuzzyThreshold is how alike edited code must be to its original to
	// count as the same code, changed.
	fuzzyThreshold = 0.6
)

type (
	// Drift names the two commits an anchor is carried between.
	Drift struct {
		_ struct{}
		// Stamp is the commit the tour was written against.
		Stamp string
		// Tip is the commit being read now.
		Tip string
	}

	// Then is the anchored code as it was at the stamp, kept when it has
	// since changed or gone, so the reader can see what the step was about.
	Then struct {
		_         struct{} `json:"-"`
		Lines     []string `json:"lines"`
		StartLine int      `json:"startLine"`
	}
)

// Hash is the contentHash of a range of lines: SHA-256 over the lines with
// their whitespace normalized, so reformatting alone doesn't count as change.
func Hash(lines []string, r Range) string {
	if r.StartLine < 1 || r.EndLine > len(lines) || r.EndLine < r.StartLine {
		return ""
	}

	h := sha256.New()

	for _, line := range lines[r.StartLine-1 : r.EndLine] {
		h.Write([]byte(normalize(line)))
		h.Write([]byte{'\n'})
	}

	return HashPrefix + hex.EncodeToString(h.Sum(nil))
}

func normalize(line string) string {
	return strings.Join(strings.Fields(line), " ")
}

// relocate follows an anchor forward from the stamp to the tip, in stages,
// stopping at the first that succeeds:
//
//  1. follow a rename of the file
//  2. carry the range through the diff; if the range itself is untouched the
//     new location is exact
//  3. re-find edited code, by pattern and then by similarity
//  4. report it removed, with the commit that removed it
//
// It returns nil when the anchor can't be found at the stamp either, in which
// case there is nothing to follow and the caller locates it at the tip.
func (r *resolver) relocate(ctx context.Context, req *Request) (*Resolution, error) {
	stamp, tip := req.Drift.Stamp, req.Drift.Tip

	before, err := r.git.ReadFile(ctx, req.Root, stamp, req.File)
	if err != nil {
		if errors.Is(err, gitsrc.ErrNotFound) || errors.Is(err, gitsrc.ErrIsDirectory) {
			return nil, nil //nolint:nilnil // nothing to follow is an answer
		}

		return nil, fmt.Errorf("reading %s at the stamp: %w", req.File, err)
	}

	oldLines := SplitLines(before)

	was := Locate(oldLines, req.Spec)
	if was.Status == StatusMissing {
		return nil, nil //nolint:nilnil // the anchor never resolved, so it can't have drifted
	}

	file := req.File

	renamed, err := r.git.RenamedTo(ctx, req.Root, stamp, tip, req.File)
	if err != nil {
		return nil, fmt.Errorf("following renames of %s: %w", req.File, err)
	}

	if renamed != "" {
		file = renamed
	}

	out := &Resolution{File: file}

	after, err := r.git.ReadFile(ctx, req.Root, tip, file)
	if err != nil {
		if !errors.Is(err, gitsrc.ErrNotFound) && !errors.Is(err, gitsrc.ErrIsDirectory) {
			return nil, fmt.Errorf("reading %s at the tip: %w", file, err)
		}

		out.Then = then(oldLines, was.Range)
		out.Location = Location{Status: StatusMissing, Problem: diag.CodeCodeRemoved, Detail: fmt.Sprintf("%q no longer exists", req.File)}
		out.Removed = r.removal(ctx, req, req.File, "")

		return out, nil
	}

	newLines := SplitLines(after)
	out.Content, out.LineCount = after, len(newLines)

	if was.Range.StartLine == 0 {
		// An anchor on the file as a whole only has to follow the file.
		out.Location = Location{Status: StatusOK}
		if renamed != "" {
			out.Location = Location{Status: StatusMoved, Problem: diag.CodeAnchorMoved, Detail: fmt.Sprintf("file moved to %q", renamed)}
		}

		return out, nil
	}

	hunks, err := r.git.Diff(ctx, req.Root, stamp, tip, req.File, file)
	if err != nil {
		return nil, fmt.Errorf("diffing %s: %w", req.File, err)
	}

	offset, touched := carry(hunks, was.Range)

	if !touched {
		out.Location = Location{Status: StatusOK, Matches: was.Matches, Range: shift(was.Range, offset)}

		switch {
		case renamed != "":
			out.Location.Status, out.Location.Problem = StatusMoved, diag.CodeAnchorMoved
			out.Location.Offset = offset
			out.Location.Detail = fmt.Sprintf("file moved to %q since %s", renamed, short(stamp))
		case offset != 0:
			out.Location.Status, out.Location.Problem = StatusMoved, diag.CodeAnchorMoved
			out.Location.Offset = offset
			out.Location.Detail = fmt.Sprintf("moved %+d lines since %s", offset, short(stamp))
		}

		return out, nil
	}

	out.Then = then(oldLines, was.Range)

	if found, ok := refind(oldLines, newLines, req.Spec, was.Range, offset); ok {
		out.Location = Location{
			Status:  StatusChanged,
			Problem: diag.CodeContentChanged,
			Detail:  fmt.Sprintf("this code changed since %s", short(stamp)),
			Range:   found,
			Offset:  found.StartLine - was.Range.StartLine,
		}

		return out, nil
	}

	out.Location = Location{Status: StatusMissing, Problem: diag.CodeCodeRemoved, Detail: fmt.Sprintf("this code was removed since %s", short(stamp))}
	out.Removed = r.removal(ctx, req, file, firstCode(oldLines, was.Range))

	return out, nil
}

// removal finds the commit that took the code away. Failing to find one only
// costs the reader an explanation, so errors are logged, not returned.
func (r *resolver) removal(ctx context.Context, req *Request, file, text string) *gitsrc.Commit {
	commit, err := r.git.LastTouch(ctx, req.Root, req.Drift.Stamp, req.Drift.Tip, file, text)
	if err != nil {
		r.logger.WithValue("file", file).WithError(err).Warn("looking for the commit that removed anchored code")
		return nil
	}

	return commit
}

// carry maps a range through a diff. It returns how far the range shifted,
// and whether any hunk changed lines inside it.
func carry(hunks []gitsrc.Hunk, r Range) (offset int, touched bool) {
	for i := range hunks {
		h := &hunks[i]

		if h.OldCount == 0 {
			// A pure insertion sits after line OldStart.
			switch {
			case h.OldStart < r.StartLine:
				offset += h.NewCount
			case h.OldStart < r.EndLine:
				touched = true
			}

			continue
		}

		last := h.OldStart + h.OldCount - 1

		switch {
		case last < r.StartLine:
			offset += h.NewCount - h.OldCount
		case h.OldStart <= r.EndLine:
			touched = true
		}
	}

	return offset, touched
}

func shift(r Range, by int) Range {
	r.StartLine += by
	r.EndLine += by

	return r
}

// refind looks for code that was edited in place: by the anchor's pattern
// near where it should now be, then by similarity to the original lines.
func refind(oldLines, newLines []string, spec Spec, was Range, offset int) (Range, bool) {
	if spec.Pattern != "" {
		hint := spec
		hint.Line = max(1, was.StartLine+offset)

		if hint.Selection != nil {
			moved := *hint.Selection
			moved.End.Line += hint.Line - moved.Start.Line
			moved.Start.Line = hint.Line
			hint.Selection = &moved
		}

		if loc := Locate(newLines, hint); loc.Status != StatusMissing && loc.Range.StartLine > 0 {
			return loc.Range, true
		}
	}

	original := oldLines[was.StartLine-1 : was.EndLine]

	start, score := mostSimilar(original, newLines)
	if score < fuzzyThreshold {
		return Range{}, false
	}

	return Range{StartLine: start, EndLine: start + len(original) - 1, StartCol: was.StartCol, EndCol: was.EndCol}, true
}

// mostSimilar slides the original lines over the file and returns the
// 1-based line where they fit best, and how well (0 to 1).
func mostSimilar(original, lines []string) (start int, score float64) {
	n := len(original)
	if n == 0 || len(lines) < n {
		return 0, 0
	}

	want := make([]string, n)
	for i, line := range original {
		want[i] = normalize(line)
	}

	have := make([]string, len(lines))
	for i, line := range lines {
		have[i] = normalize(line)
	}

	for at := 0; at+n <= len(have); at++ {
		total := 0.0
		for i := range want {
			total += similarity(want[i], have[at+i])
		}

		if s := total / float64(n); s > score {
			start, score = at+1, s
		}
	}

	return start, score
}

// similarity is the Dice coefficient over character pairs: 1 for equal
// strings, 0 for strings with no pair in common.
func similarity(a, b string) float64 {
	if a == b {
		return 1
	}

	if len(a) < 2 || len(b) < 2 {
		return 0
	}

	pairs := make(map[string]int, len(a))
	for i := 0; i+2 <= len(a); i++ {
		pairs[a[i:i+2]]++
	}

	shared := 0

	for i := 0; i+2 <= len(b); i++ {
		if pairs[b[i:i+2]] > 0 {
			pairs[b[i:i+2]]--
			shared++
		}
	}

	return 2 * float64(shared) / float64(len(a)-1+len(b)-1)
}

func then(lines []string, r Range) *Then {
	if r.StartLine < 1 || r.EndLine > len(lines) {
		return nil
	}

	return &Then{StartLine: r.StartLine, Lines: append([]string{}, lines[r.StartLine-1:r.EndLine]...)}
}

// firstCode picks the line to search history for: the first anchored line
// with something on it.
func firstCode(lines []string, r Range) string {
	for _, line := range lines[r.StartLine-1 : r.EndLine] {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}

	return sha
}
