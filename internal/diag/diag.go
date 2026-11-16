// Package diag defines the diagnostics every check in Docent reports. Their
// JSON form is the stable contract behind `docent validate --json`.
package diag

import (
	"strconv"
	"strings"
)

// Severities, in increasing order of importance.
const (
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Diagnostic codes. A code is never renamed or reused for a different meaning:
// agents branch on them.
const (
	// CodeReadFailed: the tour file couldn't be read.
	CodeReadFailed Code = "read-failed"
	// CodeJSONSyntax: the file isn't a single valid JSON document.
	CodeJSONSyntax Code = "json-syntax"
	// CodeSchema: the document doesn't match the tour JSON Schema.
	CodeSchema Code = "schema"
	// CodeExtensionIgnored: a Docent extension field is present but the tour
	// doesn't set `docent`, so the field is not interpreted.
	CodeExtensionIgnored Code = "extension-ignored"
	// CodeStepIDDuplicate: two steps share an `id`.
	CodeStepIDDuplicate Code = "step-id-duplicate"
	// CodePathEscapesRoot: a path resolves outside the root it belongs to.
	CodePathEscapesRoot Code = "path-escapes-root"
	// CodeFileMissing: a step's file or directory doesn't exist.
	CodeFileMissing Code = "file-missing"
	// CodeLineOutOfRange: `line` or `selection` points past the end of the file.
	CodeLineOutOfRange Code = "line-out-of-range"
	// CodePatternInvalid: `pattern` isn't a valid RE2 regular expression.
	CodePatternInvalid Code = "pattern-invalid"
	// CodePatternNoMatch: `pattern` matches no line of the file.
	CodePatternNoMatch Code = "pattern-no-match"
	// CodePatternAmbiguous: `pattern` matches more than one line and there is
	// no `line` to choose between them.
	CodePatternAmbiguous Code = "pattern-ambiguous"
	// CodeAnchorMoved: `pattern` matched, but not on `line`.
	CodeAnchorMoved Code = "anchor-moved"
	// CodeContentChanged: the anchored code differs from what the tour was
	// written against.
	CodeContentChanged Code = "content-changed"
	// CodeCodeRemoved: the anchored code no longer exists at the tracked ref.
	CodeCodeRemoved Code = "code-removed"
	// CodeStampUnreachable: the repo's `stamp` commit isn't in the local
	// repository, so drift can't be measured.
	CodeStampUnreachable Code = "stamp-unreachable"
	// CodeRefMissing: a pinned ref isn't in the local repository.
	CodeRefMissing Code = "ref-missing"
	// CodeRefFallback: a tracked branch no longer exists, so the default
	// branch was used instead.
	CodeRefFallback Code = "ref-fallback"
	// CodeRepoUnknown: a step names a repo the tour's `repos` doesn't define.
	CodeRepoUnknown Code = "repo-unknown"
	// CodeRepoMissing: a repo the tour uses isn't checked out on this machine.
	CodeRepoMissing Code = "repo-missing"
	// CodeMapMissing: the tour's `map` file doesn't exist, or isn't a flowchart.
	CodeMapMissing Code = "map-missing"
	// CodeMapFocusUnknown: a step's `mapFocus` names something the map doesn't
	// define. Edges must be named with Mermaid's `id@` syntax to be focused.
	CodeMapFocusUnknown Code = "map-focus-unknown"
	// CodeVisualInvalid: a visual (a step's `visual`, or a docent-* fence in a
	// body) doesn't match the visual schema, or refers to something it
	// doesn't define.
	CodeVisualInvalid Code = "visual-invalid"
	// CodeStepLinkUnknown: a docent:step link, or a visual's `step`, names a
	// step the tour doesn't have.
	CodeStepLinkUnknown Code = "step-link-unknown"
	// CodeBodyMissing: a step's `body` file doesn't exist or can't be read.
	CodeBodyMissing Code = "body-missing"
	// CodeLinkVarUnresolved: a link uses a {{variable}} nothing gives a value for.
	CodeLinkVarUnresolved Code = "link-var-unresolved"
	// CodeLinkScheme: a link's URL isn't http or https.
	CodeLinkScheme Code = "link-scheme"
)

type (
	// Severity says whether a diagnostic fails validation outright.
	Severity string

	// Code identifies the kind of problem, stably.
	Code string

	// Diagnostic is one finding about a tour file.
	Diagnostic struct {
		_ struct{} `json:"-"`

		// File is the tour file the finding is about.
		File string `json:"file"`
		// Pointer is an RFC 6901 JSON Pointer to the offending value, or ""
		// for the whole document.
		Pointer string `json:"pointer"`
		// Step is the step's `id`, or "#N" (1-based, as the step rail numbers
		// them) when it has none. Empty when the finding isn't about a step.
		Step     string   `json:"step"`
		Severity Severity `json:"severity"`
		Code     Code     `json:"code"`
		Message  string   `json:"message"`
		// Hint says what to do about it, when there's something specific to say.
		Hint string `json:"hint"`
	}
)

// Pointer builds an RFC 6901 JSON Pointer from reference tokens. Ints index arrays.
func Pointer(tokens ...any) string {
	var b strings.Builder

	for _, tok := range tokens {
		b.WriteByte('/')

		switch t := tok.(type) {
		case int:
			b.WriteString(strconv.Itoa(t))
		case string:
			b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(t))
		}
	}

	return b.String()
}

// HasErrors reports whether any diagnostic is an error.
func HasErrors(ds []Diagnostic) bool {
	for i := range ds {
		if ds[i].Severity == SeverityError {
			return true
		}
	}

	return false
}

// HasWarnings reports whether any diagnostic is a warning.
func HasWarnings(ds []Diagnostic) bool {
	for i := range ds {
		if ds[i].Severity == SeverityWarning {
			return true
		}
	}

	return false
}
