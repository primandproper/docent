// Package tour discovers, parses and normalizes tours: CodeTour `.tour` files,
// optionally carrying Docent's extension fields.
package tour

import (
	"encoding/json"

	"github.com/primandproper/docent/internal/orderedjson"
)

// FormatVersion is the value of `docent` this build understands.
const FormatVersion = 1

// LinkKinds are the kinds of link card.
var LinkKinds = []string{"grafana", "logs", "traces", "runbook", "pr", "issue", "doc", "url"}

type (
	// Tour is a parsed tour file. Fields from Docent's extensions are only
	// populated when the file sets `docent`.
	Tour struct {
		_ struct{} `json:"-"`

		// Raw is the document as written, with key order and unknown fields intact.
		Raw         *orderedjson.Value `json:"-"`
		GeneratedBy *GeneratedBy       `json:"generatedBy"`
		Map         *Map               `json:"map"`
		// Path is the absolute path of the tour file.
		Path string `json:"path"`
		// Root is the directory CodeTour-style `file` paths are relative to.
		Root        string   `json:"root"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Ref         string   `json:"ref"`
		NextTour    string   `json:"nextTour"`
		Repos       []Repo   `json:"repos"`
		Vars        []Var    `json:"vars"`
		Tags        []string `json:"tags"`
		ReviewedBy  []Review `json:"reviewedBy"`
		Steps       []Step   `json:"steps"`
		// Format is the `docent` version, or 0 for a plain CodeTour file.
		Format    int  `json:"format"`
		IsPrimary bool `json:"isPrimary"`
	}

	// Repo is one entry of a tour's `repos`.
	Repo struct {
		_      struct{} `json:"-"`
		Name   string   `json:"name"`
		Remote string   `json:"remote"`
		Ref    string   `json:"ref"`
		Stamp  string   `json:"stamp"`
		Subdir string   `json:"subdir"`
	}

	// Var is one entry of a tour's `vars`.
	Var struct {
		_           struct{} `json:"-"`
		Name        string   `json:"name"`
		Default     string   `json:"default"`
		Description string   `json:"description"`
		Choices     []string `json:"choices"`
	}

	// Map is the tour's system map: inline Mermaid, or a file holding it.
	Map struct {
		_       struct{} `json:"-"`
		Mermaid string   `json:"mermaid"`
		File    string   `json:"file"`
	}

	// GeneratedBy records the provenance of an AI-drafted tour.
	GeneratedBy struct {
		_     struct{} `json:"-"`
		Agent string   `json:"agent"`
		Model string   `json:"model"`
		At    string   `json:"at"`
	}

	// Review records someone walking the tour and vouching for it.
	Review struct {
		_    struct{} `json:"-"`
		Name string   `json:"name"`
		At   string   `json:"at"`
		Ref  string   `json:"ref"`
	}

	// Position is a point in a file. CodeTour writes both fields 1-based.
	Position struct {
		_         struct{} `json:"-"`
		Line      int      `json:"line"`
		Character int      `json:"character"`
	}

	// Selection is a range of text in a file.
	Selection struct {
		_     struct{} `json:"-"`
		Start Position `json:"start"`
		End   Position `json:"end"`
	}

	// Anchor is an extra code reference on a step, shown as a tab.
	Anchor struct {
		_           struct{}   `json:"-"`
		Selection   *Selection `json:"selection"`
		Label       string     `json:"label"`
		Repo        string     `json:"repo"`
		File        string     `json:"file"`
		Pattern     string     `json:"pattern"`
		ContentHash string     `json:"contentHash"`
		Line        int        `json:"line"`
	}

	// Link is a link card to an external system.
	Link struct {
		_           struct{} `json:"-"`
		Kind        string   `json:"kind"`
		Title       string   `json:"title"`
		URL         string   `json:"url"`
		Description string   `json:"description"`
	}

	// Step is one stop on a tour.
	Step struct {
		_ struct{} `json:"-"`

		Selection *Selection `json:"selection"`
		// Visual is the step's structured visual, as written.
		Visual      json.RawMessage `json:"visual"`
		ID          string          `json:"id"`
		Title       string          `json:"title"`
		Description string          `json:"description"`
		File        string          `json:"file"`
		Directory   string          `json:"directory"`
		URI         string          `json:"uri"`
		Pattern     string          `json:"pattern"`
		Repo        string          `json:"repo"`
		Body        string          `json:"body"`
		Layout      string          `json:"layout"`
		// Transition asks for an animation on arriving from the step before.
		Transition  string   `json:"transition"`
		Ref         string   `json:"ref"`
		ContentHash string   `json:"contentHash"`
		Anchors     []Anchor `json:"anchors"`
		MapFocus    []string `json:"mapFocus"`
		Links       []Link   `json:"links"`
		Line        int      `json:"line"`
	}
)

// IsDocent reports whether the tour opted in to Docent's extension fields.
func (t *Tour) IsDocent() bool {
	return t.Format > 0
}

// StepLabel names a step for diagnostics: its id, or "#N" counting from one.
func (t *Tour) StepLabel(index int) string {
	if index >= 0 && index < len(t.Steps) && t.Steps[index].ID != "" {
		return t.Steps[index].ID
	}

	return stepNumber(index)
}
