// Package flags reads a feature flag's live status from an integration the
// reader set up, so a `flag` link card can say what the flag is doing now.
// It is the only package that makes an HTTP request, and a tour never chooses
// where one goes: the host and the token come from the reader, and a tour
// supplies only keys that are checked here before they are used.
package flags

import (
	"context"
	"regexp"
	"slices"
	"sort"
)

const (
	// LaunchDarkly names the one integration there is: in the config, in
	// `docent integrations`, and as the name its token is stored under.
	LaunchDarkly = "launchdarkly"
	// LaunchDarklyTokenEnvVar holds the token, ahead of the credential store.
	LaunchDarklyTokenEnvVar = "DOCENT_LAUNCHDARKLY_TOKEN" //nolint:gosec // the variable's name, not a credential
	// LaunchDarklyHostEnvVar overrides the configured host.
	LaunchDarklyHostEnvVar = "DOCENT_LAUNCHDARKLY_HOST"
	// LaunchDarklyDefaultHost is where requests go unless the reader says otherwise.
	LaunchDarklyDefaultHost = "https://app.launchdarkly.com"
)

// The states a flag card can show. The frontend switches on these.
const (
	StateOn       = "on"
	StateOff      = "off"
	StateArchived = "archived"
	// StateNotFound is a flag the integration doesn't have: drift of its own kind.
	StateNotFound = "not-found"
	// StateUnavailable is every failure to find out. Reason says which.
	StateUnavailable = "unavailable"
	// StateNoToken is an integration that was never given a token.
	StateNoToken = "no-token"
	// StateDisabled is the integration switched off: the card is a plain link.
	StateDisabled = "disabled"
)

// Kinds of flag.
const (
	KindBoolean      = "boolean"
	KindMultivariate = "multivariate"
)

// OpSegmentMatch is the clause operator whose values are segment keys.
const OpSegmentMatch = "segmentMatch"

// maxSegments is how many segments are read for one flag.
const maxSegments = 6

var key = regexp.MustCompile(`^[A-Za-z0-9._-]{1,256}$`)

type (
	// Environment is whether a flag is on in one environment.
	Environment struct {
		_    struct{} `json:"-"`
		Key  string   `json:"key"`
		Name string   `json:"name"`
		On   bool     `json:"on"`
		// Shown marks the environment the card is about.
		Shown bool `json:"shown"`
		// Pinned marks the environments the configuration names: the ones
		// worth showing without being asked.
		Pinned bool `json:"pinned"`
	}

	// Variation is one of the values a flag can serve.
	Variation struct {
		_           struct{} `json:"-"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		// Value is the value as JSON: `true`, `"blue"`, `{"mode":"async"}`.
		Value string `json:"value"`
		Index int    `json:"index"`
	}

	// Share is one slice of a percentage rollout.
	Share struct {
		_         struct{} `json:"-"`
		Variation int      `json:"variation"`
		Percent   float64  `json:"percent"`
	}

	// Serve is what a rule hands out: one variation, or a rollout.
	Serve struct {
		_       struct{} `json:"-"`
		Rollout []Share  `json:"rollout"`
		// Variation is -1 when Rollout is set, or when nothing is served.
		Variation int `json:"variation"`
	}

	// Clause is one condition of a rule.
	Clause struct {
		_           struct{} `json:"-"`
		Attribute   string   `json:"attribute"`
		Op          string   `json:"op"`
		ContextKind string   `json:"contextKind"`
		Values      []string `json:"values"`
		Negate      bool     `json:"negate"`
	}

	// Rule is a targeting rule: every clause must hold.
	Rule struct {
		_           struct{} `json:"-"`
		Description string   `json:"description"`
		Clauses     []Clause `json:"clauses"`
		Serve       Serve    `json:"serve"`
	}

	// Target is a set of contexts named one by one.
	Target struct {
		_           struct{} `json:"-"`
		ContextKind string   `json:"contextKind"`
		Variation   int      `json:"variation"`
		Count       int      `json:"count"`
	}

	// Prerequisite is another flag that must serve a given variation first.
	Prerequisite struct {
		_         struct{} `json:"-"`
		Key       string   `json:"key"`
		Variation int      `json:"variation"`
	}

	// Targeting is how one environment decides what to serve, in the order
	// it is evaluated: prerequisites, named targets, rules, then the rest.
	Targeting struct {
		_             struct{}       `json:"-"`
		Prerequisites []Prerequisite `json:"prerequisites"`
		Targets       []Target       `json:"targets"`
		Rules         []Rule         `json:"rules"`
		Fallthrough   Serve          `json:"fallthrough"`
		// OffVariation is what everyone gets while the flag is off, or -1.
		OffVariation int `json:"offVariation"`
	}

	// Segment is a named set of contexts that a rule can point at.
	Segment struct {
		_           struct{} `json:"-"`
		Key         string   `json:"key"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		// Rules are the conditions that put a context in the segment, beyond
		// the ones listed by name. Each is a set of clauses that must all hold.
		Rules    [][]Clause `json:"rules"`
		Included int        `json:"included"`
		Excluded int        `json:"excluded"`
		// Found is false when the segment couldn't be read.
		Found bool `json:"found"`
	}

	// Status is everything a flag card shows. Text in it came from the
	// integration and is shown as plain text, never rendered.
	Status struct {
		_ struct{} `json:"-"`
		// Targeting is the shown environment's, or nil when it isn't known.
		Targeting *Targeting `json:"targeting"`
		State     string     `json:"state"`
		// Reason says why, for StateUnavailable and StateNoToken.
		Reason      string `json:"reason"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Project     string `json:"project"`
		Environment string `json:"environment"`
		// URL is the flag's own page, to open in the browser.
		URL          string        `json:"url"`
		Kind         string        `json:"kind"`
		Environments []Environment `json:"environments"`
		Variations   []Variation   `json:"variations"`
		// Segments are the ones the shown environment's rules point at.
		Segments []Segment `json:"segments"`
	}

	// Request names one flag in one environment.
	Request struct {
		_           struct{}
		Project     string
		Flag        string
		Environment string
		// Pinned are the environments to list first among the others.
		Pinned []string
		// Fresh asks again even if an answer is remembered.
		Fresh bool
	}

	// Settings is what the configuration says about an integration, apart
	// from where it lives and how to sign in.
	Settings struct {
		_ struct{}
		// Environments maps a value of Environment to the integration's own
		// environment key, where the names differ.
		Environments map[string]string
		Project      string
		// Environment is the configured template, already expanded: the
		// value of `{{env}}`, say.
		Environment string
	}

	// Provider reads flags from one integration.
	Provider interface {
		// Status reads one flag. It never fails: every way of not finding
		// out is a state, because a flag card must never break a step.
		Status(ctx context.Context, request *Request) Status
	}
)

// ValidKey reports whether s can be a flag, project or environment key. A
// key from a tour is checked with this before it goes anywhere near a URL.
func ValidKey(s string) bool {
	return key.MatchString(s)
}

// Empty returns a status in the given state with nothing else known, and
// every list present, so the frontend never meets a null.
func Empty(state, reason string) Status {
	return Status{
		State: state, Reason: reason,
		Environments: []Environment{}, Variations: []Variation{}, Segments: []Segment{},
	}
}

// Request turns a card's keys into a request, using the configured project
// and environment. problem says what is missing or malformed, or is empty.
func (s *Settings) Request(project, flag string) (request *Request, problem string) {
	if project == "" {
		project = s.Project
	}

	env := s.Environment
	if mapped, ok := s.Environments[env]; ok {
		env = mapped
	}

	switch {
	case !ValidKey(flag):
		return nil, "the flag key has characters a key can't have"
	case project == "":
		return nil, "no `project` is configured for LaunchDarkly"
	case !ValidKey(project):
		return nil, "the project key has characters a key can't have"
	case env == "":
		return nil, "no `environment` is configured for LaunchDarkly"
	case !ValidKey(env):
		return nil, "the environment key has characters a key can't have"
	}

	pinned := make([]string, 0, len(s.Environments))
	for _, mapped := range s.Environments {
		pinned = append(pinned, mapped)
	}

	sort.Strings(pinned)

	return &Request{Project: project, Flag: flag, Environment: env, Pinned: slices.Compact(pinned)}, ""
}
