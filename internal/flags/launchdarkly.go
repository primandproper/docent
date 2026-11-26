package flags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const (
	o11yName = "flags"

	// remember is how long an answer is reused before LaunchDarkly is asked again.
	remember = time.Minute
	// patience is how long one request may take.
	patience = 5 * time.Second
	// maxAnswer bounds what is read of one response.
	maxAnswer = 8 << 20
)

type (
	// LaunchDarklyConfig is what the LaunchDarkly provider is built from.
	// None of it ever comes from a tour or a workspace file.
	LaunchDarklyConfig struct {
		_              struct{}
		Logger         logging.Logger
		TracerProvider tracing.Provider
		// Token returns the access token, or "" when there is none. It is
		// asked on every lookup, so a token stored while the app is open is
		// found without restarting it.
		Token func(ctx context.Context) (string, error)
		// Transport overrides how requests are sent. Tests set it.
		Transport http.RoundTripper
		// Now overrides the clock. Tests set it.
		Now func() time.Time
		// Host is the https origin to ask, or "" for LaunchDarkly's own.
		Host string
	}

	launchDarkly struct {
		_       struct{}
		logger  logging.Logger
		tracer  tracing.Tracer
		token   func(ctx context.Context) (string, error)
		now     func() time.Time
		client  *http.Client
		answers map[string]answer
		host    string
		mu      sync.Mutex
	}

	answer struct {
		_      struct{}
		at     time.Time
		status Status
	}

	// What LaunchDarkly sends, as far as a card needs it.
	rawServe struct {
		Variation *int `json:"variation"`
		Rollout   *struct {
			Variations []struct {
				Variation int     `json:"variation"`
				Weight    float64 `json:"weight"`
			} `json:"variations"`
		} `json:"rollout"`
	}

	rawTarget struct {
		ContextKind string   `json:"contextKind"`
		Values      []string `json:"values"`
		Variation   int      `json:"variation"`
	}

	rawClause struct {
		Attribute   string `json:"attribute"`
		Op          string `json:"op"`
		ContextKind string `json:"contextKind"`
		Values      []any  `json:"values"`
		Negate      bool   `json:"negate"`
	}

	rawRule struct {
		rawServe
		Description string      `json:"description"`
		Clauses     []rawClause `json:"clauses"`
	}

	rawEnvironment struct {
		OffVariation *int   `json:"offVariation"`
		Name         string `json:"_environmentName"`
		Site         struct {
			Href string `json:"href"`
		} `json:"_site"`
		Fallthrough    rawServe       `json:"fallthrough"`
		Targets        []rawTarget    `json:"targets"`
		ContextTargets []rawTarget    `json:"contextTargets"`
		Prerequisites  []Prerequisite `json:"prerequisites"`
		Rules          []rawRule      `json:"rules"`
		On             bool           `json:"on"`
	}

	rawFlag struct {
		Environments map[string]rawEnvironment `json:"environments"`
		Name         string                    `json:"name"`
		Description  string                    `json:"description"`
		Kind         string                    `json:"kind"`
		Variations   []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Value       json.RawMessage `json:"value"`
		} `json:"variations"`
		Archived bool `json:"archived"`
	}

	rawSegment struct {
		Name             string      `json:"name"`
		Description      string      `json:"description"`
		Included         []string    `json:"included"`
		Excluded         []string    `json:"excluded"`
		IncludedContexts []rawTarget `json:"includedContexts"`
		ExcludedContexts []rawTarget `json:"excludedContexts"`
		Rules            []struct {
			Clauses []rawClause `json:"clauses"`
		} `json:"rules"`
	}
)

var _ Provider = (*launchDarkly)(nil)

// NewLaunchDarkly returns a provider that reads flags from LaunchDarkly's
// REST API.
func NewLaunchDarkly(cfg *LaunchDarklyConfig) Provider {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &launchDarkly{
		logger:  logging.NewNamedLogger(cfg.Logger, o11yName),
		tracer:  tracing.NewNamedTracer(cfg.TracerProvider, o11yName),
		token:   cfg.Token,
		now:     now,
		host:    cfg.Host,
		answers: map[string]answer{},
		client: &http.Client{
			Timeout:   patience,
			Transport: cfg.Transport,
			// A redirect could carry the token to a host the reader never chose.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Origin checks a configured host and returns it as an origin to build URLs
// on: https, a host, and nothing else. problem says what is wrong with it.
func Origin(host string) (origin, problem string) {
	if host == "" {
		return LaunchDarklyDefaultHost, ""
	}

	parsed, err := url.Parse(strings.TrimRight(host, "/"))

	switch {
	case err != nil || parsed.Host == "":
		return "", "the configured LaunchDarkly host isn't a URL"
	case parsed.Scheme != "https":
		return "", "the configured LaunchDarkly host must be https"
	case parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "":
		return "", "the configured LaunchDarkly host must be an origin only: https://host"
	}

	return "https://" + parsed.Host, ""
}

func (l *launchDarkly) Status(ctx context.Context, request *Request) Status {
	ctx, span := l.tracer.StartSpan(ctx)
	defer span.End()

	logger := l.logger.WithValues(map[string]any{"flag": request.Flag, "project": request.Project, "environment": request.Environment})

	unavailable := func(reason string) Status {
		status := Empty(StateUnavailable, reason)
		status.Project, status.Environment = request.Project, request.Environment

		logger.WithValue("reason", reason).Info("flag status unavailable")

		return status
	}

	// Checked again here, whatever the caller did: these go into a URL.
	if !ValidKey(request.Project) || !ValidKey(request.Flag) || !ValidKey(request.Environment) {
		return unavailable("a key has characters a key can't have")
	}

	origin, problem := Origin(l.host)
	if problem != "" {
		return unavailable(problem)
	}

	token := ""

	if l.token != nil {
		var err error
		if token, err = l.token(ctx); err != nil {
			logger.Error("reading the LaunchDarkly token", err)

			return unavailable("the credential store couldn't be read")
		}
	}

	if token == "" {
		status := Empty(StateNoToken, "Run `docent integrations set "+LaunchDarkly+"`, or set "+LaunchDarklyTokenEnvVar+", to see this flag's status")
		status.Project, status.Environment = request.Project, request.Environment

		return status
	}

	if strings.ContainsFunc(token, func(r rune) bool { return r < 0x21 || r > 0x7e }) {
		return unavailable("the LaunchDarkly token has characters a token can't have")
	}

	cacheKey := strings.Join([]string{origin, request.Project, request.Flag, request.Environment}, "\n")

	l.mu.Lock()
	remembered, ok := l.answers[cacheKey]
	l.mu.Unlock()

	if ok && !request.Fresh && l.now().Sub(remembered.at) < remember {
		return remembered.status
	}

	flagURL := origin + "/api/v2/flags/" + url.PathEscape(request.Project) + "/" + url.PathEscape(request.Flag)

	// Asked for by name, an environment comes back whole: its rules and
	// targets. Asked for without one, every environment comes back in brief.
	detail, state, reason := get[rawFlag](ctx, l.client, flagURL+"?env="+url.QueryEscape(request.Environment), token)

	switch state {
	case "":
	case StateNotFound:
		status := Empty(StateNotFound, "")
		status.Project, status.Environment = request.Project, request.Environment
		l.keep(cacheKey, &status)

		logger.Info("flag not found")

		return status
	default:
		return unavailable(reason)
	}

	all, state, _ := get[rawFlag](ctx, l.client, flagURL, token)
	if state != "" {
		// The one environment is still worth showing on its own.
		all = detail
	}

	status := build(request, origin, detail, all)
	if status.State == StateUnavailable {
		return unavailable(status.Reason)
	}

	status.Segments = segments(&status, func(segmentKey string) *rawSegment {
		segment, _, _ := get[rawSegment](ctx, l.client,
			origin+"/api/v2/segments/"+url.PathEscape(request.Project)+"/"+url.PathEscape(request.Environment)+"/"+url.PathEscape(segmentKey), token)

		return segment
	})

	l.keep(cacheKey, &status)

	logger.WithValue("state", status.State).Info("read flag status")

	return status
}

func (l *launchDarkly) keep(cacheKey string, status *Status) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.answers[cacheKey] = answer{at: l.now(), status: *status}
}

// get makes one request. A non-empty state says why there is no answer.
func get[T any](ctx context.Context, client *http.Client, endpoint, token string) (answer *T, state, reason string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, StateUnavailable, "the configured LaunchDarkly host isn't a URL"
	}

	req.Header.Set("Authorization", token)
	req.Header.Set("User-Agent", "docent")

	res, err := client.Do(req)
	if err != nil {
		var timeout interface{ Timeout() bool }
		if errors.As(err, &timeout) && timeout.Timeout() {
			return nil, StateUnavailable, "LaunchDarkly didn't answer in time"
		}

		return nil, StateUnavailable, "LaunchDarkly can't be reached"
	}
	defer res.Body.Close() //nolint:errcheck // nothing to do about it

	switch {
	case res.StatusCode == http.StatusOK:
	case res.StatusCode == http.StatusNotFound:
		return nil, StateNotFound, ""
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return nil, StateUnavailable, "LaunchDarkly refused the token"
	case res.StatusCode == http.StatusTooManyRequests:
		return nil, StateUnavailable, "LaunchDarkly is rate limiting; try again shortly"
	case res.StatusCode >= 300 && res.StatusCode < 400:
		return nil, StateUnavailable, "the configured LaunchDarkly host redirects elsewhere"
	default:
		return nil, StateUnavailable, fmt.Sprintf("LaunchDarkly answered %d", res.StatusCode)
	}

	answer = new(T)
	if err = json.NewDecoder(io.LimitReader(res.Body, maxAnswer)).Decode(answer); err != nil {
		return nil, StateUnavailable, "LaunchDarkly's answer couldn't be read"
	}

	return answer, "", ""
}

// build turns LaunchDarkly's answers into a status: detail holds the shown
// environment in full, all holds every environment at least in brief.
func build(request *Request, origin string, detail, all *rawFlag) Status {
	out := Empty("", "")
	out.Project, out.Environment = request.Project, request.Environment
	out.Name, out.Description, out.Kind = detail.Name, detail.Description, detail.Kind
	out.URL = origin + "/projects/" + url.PathEscape(request.Project) + "/flags/" + url.PathEscape(request.Flag) +
		"/targeting?env=" + url.QueryEscape(request.Environment) + "&selected-env=" + url.QueryEscape(request.Environment)

	for i := range detail.Variations {
		v := &detail.Variations[i]
		out.Variations = append(out.Variations, Variation{Index: i, Name: v.Name, Description: v.Description, Value: string(v.Value)})
	}

	for envKey := range all.Environments {
		e := all.Environments[envKey]

		name := e.Name
		if name == "" {
			name = envKey
		}

		shown := envKey == request.Environment

		out.Environments = append(out.Environments, Environment{
			Key: envKey, Name: name, On: e.On, Shown: shown, Pinned: shown || slices.Contains(request.Pinned, envKey),
		})
	}

	// The shown one, then the pinned ones, then the rest, each by key.
	sort.Slice(out.Environments, func(i, j int) bool {
		a, b := &out.Environments[i], &out.Environments[j]

		switch {
		case a.Shown != b.Shown:
			return a.Shown
		case a.Pinned != b.Pinned:
			return a.Pinned
		default:
			return a.Key < b.Key
		}
	})

	shown, known := detail.Environments[request.Environment]
	if !known {
		return Empty(StateUnavailable, fmt.Sprintf("the flag has no environment %q", request.Environment))
	}

	// LaunchDarkly's own link to the flag, when it gives one on its own site.
	if strings.HasPrefix(shown.Site.Href, "/") && !strings.HasPrefix(shown.Site.Href, "//") {
		out.URL = origin + shown.Site.Href
	}

	out.Targeting = targeting(&shown)

	switch {
	case detail.Archived:
		out.State = StateArchived
	case shown.On:
		out.State = StateOn
	default:
		out.State = StateOff
	}

	return out
}

func targeting(e *rawEnvironment) *Targeting {
	t := &Targeting{
		Prerequisites: e.Prerequisites, Targets: []Target{}, Rules: []Rule{},
		Fallthrough: serve(&e.Fallthrough), OffVariation: -1,
	}

	if t.Prerequisites == nil {
		t.Prerequisites = []Prerequisite{}
	}

	if e.OffVariation != nil {
		t.OffVariation = *e.OffVariation
	}

	for _, list := range [][]rawTarget{e.Targets, e.ContextTargets} {
		for i := range list {
			if len(list[i].Values) > 0 {
				t.Targets = append(t.Targets, Target{ContextKind: list[i].ContextKind, Variation: list[i].Variation, Count: len(list[i].Values)})
			}
		}
	}

	for i := range e.Rules {
		r := &e.Rules[i]
		t.Rules = append(t.Rules, Rule{Description: r.Description, Clauses: clauses(r.Clauses), Serve: serve(&r.rawServe)})
	}

	return t
}

func clauses(raw []rawClause) []Clause {
	out := make([]Clause, 0, len(raw))

	for i := range raw {
		c := &raw[i]
		clause := Clause{Attribute: c.Attribute, Op: c.Op, ContextKind: c.ContextKind, Negate: c.Negate, Values: []string{}}

		for _, v := range c.Values {
			if s, isString := v.(string); isString {
				clause.Values = append(clause.Values, s)
			} else if encoded, err := json.Marshal(v); err == nil {
				clause.Values = append(clause.Values, string(encoded))
			}
		}

		out = append(out, clause)
	}

	return out
}

func serve(raw *rawServe) Serve {
	out := Serve{Variation: -1, Rollout: []Share{}}

	if raw.Variation != nil {
		out.Variation = *raw.Variation
	}

	if raw.Rollout != nil {
		for _, share := range raw.Rollout.Variations {
			// LaunchDarkly counts in thousandths of a percent.
			out.Rollout = append(out.Rollout, Share{Variation: share.Variation, Percent: share.Weight / 1000})
		}
	}

	return out
}

// segments reads each segment the shown environment's rules point at, a
// handful at most. One that can't be read is still listed, as not found.
func segments(status *Status, read func(segmentKey string) *rawSegment) []Segment {
	out := []Segment{}
	if status.Targeting == nil {
		return out
	}

	for i := range status.Targeting.Rules {
		for j := range status.Targeting.Rules[i].Clauses {
			c := &status.Targeting.Rules[i].Clauses[j]
			if c.Op != OpSegmentMatch {
				continue
			}

			for _, segmentKey := range c.Values {
				// The key came from LaunchDarkly, and is still checked before it goes into a URL.
				if len(out) >= maxSegments || !ValidKey(segmentKey) || slices.ContainsFunc(out, func(s Segment) bool { return s.Key == segmentKey }) {
					continue
				}

				segment := Segment{Key: segmentKey, Rules: [][]Clause{}}

				if raw := read(segmentKey); raw != nil {
					segment.Found, segment.Name, segment.Description = true, raw.Name, raw.Description
					segment.Included, segment.Excluded = len(raw.Included), len(raw.Excluded)

					for k := range raw.IncludedContexts {
						segment.Included += len(raw.IncludedContexts[k].Values)
					}

					for k := range raw.ExcludedContexts {
						segment.Excluded += len(raw.ExcludedContexts[k].Values)
					}

					for k := range raw.Rules {
						segment.Rules = append(segment.Rules, clauses(raw.Rules[k].Clauses))
					}
				}

				out = append(out, segment)
			}
		}
	}

	return out
}
