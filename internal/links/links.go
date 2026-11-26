// Package links turns the link templates in a tour into cards: it fills in
// {{variables}} and refuses any URL that isn't plain web.
package links

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/primandproper/docent/internal/flags"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/workspace"
)

// Where a variable's value came from, in priority order.
const (
	SourceFlag      = "flag"
	SourcePicker    = "picker"
	SourceWorkspace = "workspace"
	SourceUser      = "user"
	SourceTour      = "tour"
)

var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z0-9][A-Za-z0-9._-]*)\s*\}\}`)

type (
	// Card is a link ready to show.
	Card struct {
		_           struct{} `json:"-"`
		Kind        string   `json:"kind"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		URL         string   `json:"url"`
		Host        string   `json:"host"`
		Problem     string   `json:"problem"`
		Flag        string   `json:"flag"`
		Project     string   `json:"project"`
		Missing     []string `json:"missing"`
	}

	// Vars answers what a link variable's value is. Sources are consulted in
	// priority order: a command-line flag, the in-app picker, the workspace
	// file, the user config, then the tour's own default.
	Vars struct {
		_      struct{}
		config *workspace.Config
		flags  map[string]string
		picked map[string]string
		mu     sync.RWMutex
	}
)

// NewVars returns a Vars over a configuration and the values given with --var.
func NewVars(config *workspace.Config, given map[string]string) *Vars {
	return &Vars{config: config, flags: given, picked: map[string]string{}}
}

// Pick records a value chosen in the app. An empty value clears the choice.
func (v *Vars) Pick(name, value string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if value == "" {
		delete(v.picked, name)
		return
	}

	v.picked[name] = value
}

// Lookup returns a variable's value and where it came from. defined is the
// tour's own declarations, the last resort.
func (v *Vars) Lookup(name string, defined []tour.Var) (value, source string, ok bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if value, ok = v.flags[name]; ok {
		return value, SourceFlag, true
	}

	if value, ok = v.picked[name]; ok {
		return value, SourcePicker, true
	}

	if v.config != nil {
		if value, source, ok = v.config.Var(name); ok {
			return value, source, true
		}
	}

	for i := range defined {
		if defined[i].Name == name && defined[i].Default != "" {
			return defined[i].Default, SourceTour, true
		}
	}

	return "", "", false
}

// FlagProblem says what is wrong with a `flag` card's keys, or is empty. The
// keys end up in a request's path, so only the characters a key can have are
// allowed.
func FlagProblem(link *tour.Link) string {
	switch {
	case link.Flag == "":
		return "A flag card needs a `flag`: the flag's key"
	case !flags.ValidKey(link.Flag):
		return "`flag` must be a flag key: letters, digits, `.`, `_` and `-`"
	case link.Project != "" && !flags.ValidKey(link.Project):
		return "`project` must be a project key: letters, digits, `.`, `_` and `-`"
	}

	return ""
}

// Fill expands the variables in a template that isn't a link, such as an
// integration's `environment`. A variable with no value becomes nothing.
func (v *Vars) Fill(template string, defined []tour.Var) string {
	return placeholder.ReplaceAllStringFunc(template, func(match string) string {
		if value, _, ok := v.Lookup(placeholder.FindStringSubmatch(match)[1], defined); ok {
			return value
		}

		return ""
	})
}

// Names lists the distinct variables a template uses, in order of appearance.
func Names(template string) []string {
	var names []string

	for _, match := range placeholder.FindAllStringSubmatch(template, -1) {
		if !slices.Contains(names, match[1]) {
			names = append(names, match[1])
		}
	}

	return names
}

// Expand fills in a link's variables and checks the result. A card with a
// missing variable or a URL that isn't http(s) comes back with Problem set.
func (v *Vars) Expand(link *tour.Link, defined []tour.Var) Card {
	card := Card{Kind: link.Kind, Title: link.Title, Description: link.Description, Missing: []string{}}

	if link.Kind == tour.LinkKindFlag {
		if card.Problem = FlagProblem(link); card.Problem != "" {
			return card
		}

		card.Flag, card.Project = link.Flag, link.Project

		// With no url of its own, the card links to the flag's page, which
		// comes with its status.
		if link.URL == "" {
			return card
		}
	}

	card.URL = placeholder.ReplaceAllStringFunc(link.URL, func(match string) string {
		name := placeholder.FindStringSubmatch(match)[1]

		value, _, ok := v.Lookup(name, defined)
		if !ok {
			if !slices.Contains(card.Missing, name) {
				card.Missing = append(card.Missing, name)
			}

			return match
		}

		return value
	})

	if len(card.Missing) > 0 {
		card.Problem = "No value for " + list(card.Missing)
		return card
	}

	// A docent: link moves within the app: to a step, or to another tour.
	if IsInternal(card.URL) {
		return card
	}

	parsed, err := url.Parse(card.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		card.Problem = "Only http, https and docent links can be opened"
		return card
	}

	card.Host = parsed.Host

	return card
}

func list(names []string) string {
	out := ""

	for i, name := range names {
		if i > 0 {
			out += ", "
		}

		out += "{{" + name + "}}"
	}

	return out
}

// IsInternal reports whether a URL is one of Docent's own: `docent:step/<id>`
// or `docent:tour/<path>#<id>`.
func IsInternal(link string) bool {
	rest, ok := strings.CutPrefix(link, "docent:")
	if !ok {
		return false
	}

	for _, kind := range []string{"step/", "tour/"} {
		if target, found := strings.CutPrefix(rest, kind); found && target != "" {
			return true
		}
	}

	return false
}
