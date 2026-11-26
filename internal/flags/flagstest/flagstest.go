// Package flagstest is a stand-in for LaunchDarkly's REST API: a TLS server
// that answers the requests the flags package makes, from a made-up project.
// Tests use it, and so does the tool that records the browser tests' fixture.
package flagstest

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

const (
	// Token is the access token the server accepts.
	Token = "api-00000000-test-token"
	// Project is the one project the server has.
	Project = "default"
	// RateLimited is a flag key the server always answers 429 for.
	RateLimited = "express-pay-button"
)

// quiet are environments nobody set up rules in: there to show that a card
// copes with many of them.
var quiet = []string{"apac-production", "canary", "demo", "dev", "dr", "eu-production", "eu-staging", "perf", "preview", "qa", "sandbox"}

//go:embed project.json
var projectJSON []byte

type (
	project struct {
		Flags    map[string]map[string]any `json:"flags"`
		Segments map[string]map[string]any `json:"segments"`
	}

	// Server is the stand-in. Its URL and Client come from the embedded
	// httptest server; hand Client().Transport to the provider.
	Server struct {
		*httptest.Server
		_       struct{}
		project project
		// requests are the paths asked for, with their queries, in order.
		requests []string
		mu       sync.Mutex
	}
)

// NewServer starts the stand-in. Close it when done.
func NewServer() *Server {
	s := &Server{}

	if err := json.Unmarshal(projectJSON, &s.project); err != nil {
		panic("flagstest: project.json: " + err.Error())
	}

	for flagKey, flag := range s.project.Flags {
		envs, _ := flag["environments"].(map[string]any) //nolint:errcheck // the embedded file is known

		for i, name := range quiet {
			envs[name] = map[string]any{"_environmentName": name, "on": (i+len(flagKey))%3 == 0}
		}
	}

	s.Server = httptest.NewTLSServer(http.HandlerFunc(s.handle))

	return s
}

// Requests returns what has been asked for so far.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.requests...)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, r.URL.RequestURI())
	s.mu.Unlock()

	if r.Header.Get("Authorization") != Token {
		http.Error(w, `{"code":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	switch {
	case len(parts) == 5 && parts[2] == "flags" && parts[3] == Project:
		s.flag(w, r, parts[4])
	case len(parts) == 6 && parts[2] == "segments" && parts[3] == Project:
		if segment, ok := s.project.Segments[parts[5]]; ok {
			respond(w, segment)
			return
		}

		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) flag(w http.ResponseWriter, r *http.Request, flagKey string) {
	if flagKey == RateLimited {
		http.Error(w, `{"code":"rate_limited"}`, http.StatusTooManyRequests)
		return
	}

	flag, ok := s.project.Flags[flagKey]
	if !ok {
		http.NotFound(w, r)
		return
	}

	out := map[string]any{}
	for name, value := range flag {
		out[name] = value
	}

	all, _ := flag["environments"].(map[string]any) //nolint:errcheck // the embedded file is known
	envs := map[string]any{}

	// As LaunchDarkly does: an environment asked for by name comes back
	// whole, and without a name every environment comes back in brief.
	if asked := r.URL.Query()["env"]; len(asked) > 0 {
		for _, name := range asked {
			if env, found := all[name]; found {
				envs[name] = env
			}
		}
	} else {
		for name, env := range all {
			full, _ := env.(map[string]any) //nolint:errcheck // the embedded file is known
			envs[name] = map[string]any{"_environmentName": full["_environmentName"], "on": full["on"]}
		}
	}

	out["environments"] = envs

	respond(w, out)
}

func respond(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
