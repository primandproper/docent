// Package anchor turns a step's `line`, `selection` and `pattern` into a
// concrete range of a file, plus a status saying how far that range can be
// trusted.
package anchor

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/tour"
)

// Anchor statuses. Docent always shows its best guess with a warning instead
// of failing the step.
const (
	// StatusOK: it resolved where the author said.
	StatusOK Status = "ok"
	// StatusMoved: the pattern matched, but not at `line`.
	StatusMoved Status = "moved"
	// StatusAmbiguous: the pattern matched more than once with no `line` to
	// break the tie.
	StatusAmbiguous Status = "ambiguous"
	// StatusChanged: it resolved, but the anchored text differs from the stamp.
	StatusChanged Status = "changed"
	// StatusMissing: file, pattern or ref not found.
	StatusMissing Status = "missing"
	// StatusUnverified: the stamp is unreachable, so the code can't be
	// confirmed unchanged.
	StatusUnverified Status = "unverified"
)

type (
	// Status is how an anchor resolved.
	Status string

	// Spec is the part of a step or anchor that says where in a file to point.
	Spec struct {
		_         struct{}
		Selection *tour.Selection
		Pattern   string
		Line      int
	}

	// Range is a span of a file. Lines are 1-based and inclusive. Columns are
	// 1-based with an exclusive end, as CodeTour records them; both are zero
	// when the range covers whole lines. A zero StartLine means the anchor
	// points at the file as a whole.
	Range struct {
		_         struct{} `json:"-"`
		StartLine int      `json:"startLine"`
		StartCol  int      `json:"startCol"`
		EndLine   int      `json:"endLine"`
		EndCol    int      `json:"endCol"`
	}

	// Location is where a Spec landed in a particular file.
	Location struct {
		_ struct{} `json:"-"`
		// Problem is the diagnostic code explaining a status other than ok,
		// and Detail the sentence that goes with it.
		Problem diag.Code `json:"problem"`
		Detail  string    `json:"detail"`
		Status  Status    `json:"status"`
		Range   Range     `json:"range"`
		// Offset is how many lines the match sits from `line` when moved.
		Offset int `json:"offset"`
		// Matches is how many lines the pattern matched.
		Matches int `json:"matches"`
	}
)

var patternCache sync.Map // pattern string -> *regexp.Regexp or error

func compile(pattern string) (*regexp.Regexp, error) {
	if cached, ok := patternCache.Load(pattern); ok {
		switch c := cached.(type) {
		case *regexp.Regexp:
			return c, nil
		case error:
			return nil, c
		}
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		patternCache.Store(pattern, err)
		return nil, err
	}

	patternCache.Store(pattern, re)

	return re, nil
}

// SplitLines splits file content into lines without their terminators. A
// trailing newline doesn't produce an empty last line.
func SplitLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}

	text := string(bytes.TrimSuffix(content, []byte("\n")))
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}

	return lines
}

// Locate applies a Spec to the lines of a file.
//
//   - `pattern` is matched per line. With `line`, the match nearest to it wins;
//     without, the first match wins.
//   - `selection` with a `pattern` moves with the match; without one it is absolute.
//   - `line` alone is absolute.
func Locate(lines []string, spec Spec) Location {
	if spec.Pattern == "" {
		return locateAbsolute(lines, spec)
	}

	re, err := compile(spec.Pattern)
	if err != nil {
		loc := locateAbsolute(lines, spec)
		loc.Status = StatusMissing
		loc.Problem = diag.CodePatternInvalid
		loc.Detail = fmt.Sprintf("pattern is not a valid RE2 regular expression: %v", err)

		return loc
	}

	var matches []int

	for i, line := range lines {
		if re.MatchString(line) {
			matches = append(matches, i+1)
		}
	}

	if len(matches) == 0 {
		loc := locateAbsolute(lines, spec)
		loc.Status = StatusMissing
		loc.Problem = diag.CodePatternNoMatch
		loc.Detail = "pattern matches no line of the file"

		return loc
	}

	loc := Location{Status: StatusOK, Matches: len(matches)}
	matched := matches[0]

	// base is the line the author was looking at when they wrote the anchor.
	base := spec.Line
	if base == 0 && spec.Selection != nil {
		base = spec.Selection.Start.Line
	}

	switch {
	case spec.Line > 0:
		for _, m := range matches[1:] {
			if abs(m-spec.Line) < abs(matched-spec.Line) {
				matched = m
			}
		}

		if matched != spec.Line {
			loc.Status = StatusMoved
			loc.Offset = matched - spec.Line
			loc.Problem = diag.CodeAnchorMoved
			loc.Detail = fmt.Sprintf("pattern matched on line %d, not line %d", matched, spec.Line)
		}
	case len(matches) > 1:
		loc.Status = StatusAmbiguous
		loc.Problem = diag.CodePatternAmbiguous
		loc.Detail = fmt.Sprintf("pattern matches %d lines and there is no `line` to choose between them", len(matches))
	}

	loc.Range = Range{StartLine: matched, EndLine: matched}

	if sel := spec.Selection; sel != nil && sel.Start.Line > 0 {
		delta := matched - base
		loc.Range = Range{
			StartLine: sel.Start.Line + delta,
			StartCol:  sel.Start.Character,
			EndLine:   sel.End.Line + delta,
			EndCol:    sel.End.Character,
		}

		if !inRange(loc.Range, len(lines)) {
			loc.Range = Range{StartLine: matched, EndLine: matched}
		}
	}

	return loc
}

func locateAbsolute(lines []string, spec Spec) Location {
	loc := Location{Status: StatusOK}

	switch {
	case spec.Selection != nil && spec.Selection.Start.Line > 0:
		loc.Range = Range{
			StartLine: spec.Selection.Start.Line,
			StartCol:  spec.Selection.Start.Character,
			EndLine:   spec.Selection.End.Line,
			EndCol:    spec.Selection.End.Character,
		}
	case spec.Line > 0:
		loc.Range = Range{StartLine: spec.Line, EndLine: spec.Line}
	default:
		return loc
	}

	if !inRange(loc.Range, len(lines)) {
		loc.Status = StatusMissing
		loc.Problem = diag.CodeLineOutOfRange
		loc.Detail = fmt.Sprintf("lines %d to %d are outside the file, which has %d lines", loc.Range.StartLine, loc.Range.EndLine, len(lines))
		loc.Range = Range{}

		// Keep whatever part of the request is still inside the file.
		if spec.Line > 0 && spec.Line <= len(lines) {
			loc.Range = Range{StartLine: spec.Line, EndLine: spec.Line}
		}
	}

	return loc
}

func inRange(r Range, lineCount int) bool {
	return r.StartLine >= 1 && r.EndLine >= r.StartLine && r.EndLine <= lineCount
}

func abs(n int) int {
	if n < 0 {
		return -n
	}

	return n
}
