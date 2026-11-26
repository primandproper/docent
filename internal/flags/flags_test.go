package flags_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/primandproper/docent/internal/flags"
	"github.com/primandproper/docent/internal/flags/flagstest"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func token(context.Context) (string, error) { return flagstest.Token, nil }

func request(flag, env string) *flags.Request {
	return &flags.Request{Project: flagstest.Project, Flag: flag, Environment: env, Pinned: []string{"production", "staging"}}
}

func newProvider(t *testing.T, server *flagstest.Server, cfg *flags.LaunchDarklyConfig) flags.Provider {
	t.Helper()

	if cfg == nil {
		cfg = &flags.LaunchDarklyConfig{}
	}

	if cfg.Token == nil {
		cfg.Token = token
	}

	if cfg.Host == "" {
		cfg.Host = server.URL
	}

	cfg.Transport = server.Client().Transport

	return flags.NewLaunchDarkly(cfg)
}

func TestLaunchDarkly(T *testing.T) {
	T.Parallel()

	T.Run("a boolean flag with prerequisites, targets, rules and a rollout", func(t *testing.T) {
		t.Parallel()

		server := flagstest.NewServer()
		t.Cleanup(server.Close)

		status := newProvider(t, server, nil).Status(t.Context(), request("new-checkout-flow", "production"))

		must.Eq(t, flags.StateOn, status.State)
		test.Eq(t, "New checkout flow", status.Name)
		test.Eq(t, flags.KindBoolean, status.Kind)
		test.Eq(t, []flags.Variation{{Index: 0, Name: "v2", Value: "true"}, {Index: 1, Name: "v1", Value: "false"}}, status.Variations)
		// LaunchDarkly's own link, on the configured host.
		test.Eq(t, server.URL+"/default/production/features/new-checkout-flow", status.URL)

		must.NotNil(t, status.Targeting)
		tg := status.Targeting
		test.Eq(t, []flags.Prerequisite{{Key: "pricing-service-v2", Variation: 0}}, tg.Prerequisites)
		test.Eq(t, []flags.Target{{ContextKind: "account", Variation: 0, Count: 3}}, tg.Targets)
		test.Eq(t, 1, tg.OffVariation)
		test.Eq(t, flags.Serve{Variation: 1, Rollout: []flags.Share{}}, tg.Fallthrough)

		must.Len(t, 2, tg.Rules)
		test.Eq(t, "Beta customers", tg.Rules[0].Description)
		test.Eq(t, []flags.Clause{{Attribute: "segmentMatch", Op: flags.OpSegmentMatch, ContextKind: "user", Values: []string{"beta-testers", "internal-staff"}}}, tg.Rules[0].Clauses)
		test.Eq(t, 0, tg.Rules[0].Serve.Variation)

		ramp := tg.Rules[1]
		test.Eq(t, -1, ramp.Serve.Variation)
		test.Eq(t, []flags.Share{{Variation: 0, Percent: 25}, {Variation: 1, Percent: 75}}, ramp.Serve.Rollout)
		must.Len(t, 2, ramp.Clauses)
		test.True(t, ramp.Clauses[1].Negate)

		must.Len(t, 2, status.Segments)
		test.Eq(t, flags.Segment{Key: "beta-testers", Name: "Beta testers", Found: true, Included: 12, Rules: [][]flags.Clause{}}, status.Segments[0])
		test.Eq(t, 1, len(status.Segments[1].Rules))
	})

	T.Run("the token is no part of what the frontend is sent", func(t *testing.T) {
		t.Parallel()

		server := flagstest.NewServer()
		t.Cleanup(server.Close)

		provider := newProvider(t, server, nil)

		for _, flag := range []string{"new-checkout-flow", "payment-routing", "one-click-reorder", flagstest.RateLimited} {
			encoded, err := json.Marshal(provider.Status(t.Context(), request(flag, "production")))
			must.NoError(t, err)
			test.StrNotContains(t, string(encoded), flagstest.Token)
		}
	})

	T.Run("every environment is listed: the shown one, the pinned ones, then the rest", func(t *testing.T) {
		t.Parallel()

		server := flagstest.NewServer()
		t.Cleanup(server.Close)

		status := newProvider(t, server, nil).Status(t.Context(), request("new-checkout-flow", "staging"))

		must.Eq(t, flags.StateOn, status.State)
		must.Len(t, 14, status.Environments)
		test.Eq(t, flags.Environment{Key: "staging", Name: "Staging", On: true, Shown: true, Pinned: true}, status.Environments[0])
		test.Eq(t, flags.Environment{Key: "production", Name: "Production", On: true, Pinned: true}, status.Environments[1])
		test.Eq(t, "apac-production", status.Environments[2].Key)
		test.False(t, status.Environments[2].Pinned)
	})

	T.Run("a JSON flag keeps each variation's value as JSON", func(t *testing.T) {
		t.Parallel()

		server := flagstest.NewServer()
		t.Cleanup(server.Close)

		status := newProvider(t, server, nil).Status(t.Context(), request("one-page-checkout", "production"))

		must.Eq(t, flags.StateOn, status.State)
		test.Eq(t, flags.KindMultivariate, status.Kind)
		must.Len(t, 2, status.Variations)
		test.Eq(t, `{"layout":"classic","steps":["cart","address","shipping","payment","review"]}`, status.Variations[0].Value)

		must.Len(t, 1, status.Segments)
		test.Eq(t, 18, status.Segments[0].Included)
		test.Eq(t, 2, status.Segments[0].Excluded)
	})

	T.Run("a segment that can't be read is listed as not found, and values that aren't strings are kept", func(t *testing.T) {
		t.Parallel()

		server := flagstest.NewServer()
		t.Cleanup(server.Close)

		status := newProvider(t, server, nil).Status(t.Context(), request("payment-routing", "production"))

		must.Eq(t, flags.StateOn, status.State)
		must.Len(t, 6, status.Targeting.Rules)
		test.Eq(t, []flags.Target{{ContextKind: "account", Variation: 3, Count: 1}}, status.Targeting.Targets)
		test.Eq(t, []string{"5000"}, status.Targeting.Rules[3].Clauses[0].Values)

		must.Len(t, 4, status.Segments)
		test.Eq(t, flags.Segment{Key: "deleted-segment", Rules: [][]flags.Clause{}}, status.Segments[3])
	})

	T.Run("off, archived and not found are each their own state", func(t *testing.T) {
		t.Parallel()

		server := flagstest.NewServer()
		t.Cleanup(server.Close)

		provider := newProvider(t, server, nil)

		off := provider.Status(t.Context(), request("payment-routing", "staging"))
		test.Eq(t, flags.StateOff, off.State)
		test.Len(t, 1, off.Targeting.Rules)

		test.Eq(t, flags.StateArchived, provider.Status(t.Context(), request("legacy-coupon-path", "production")).State)

		missing := provider.Status(t.Context(), request("one-click-reorder", "production"))
		test.Eq(t, flags.StateNotFound, missing.State)
		test.Nil(t, missing.Targeting)
		test.NotNil(t, missing.Environments)
	})

	T.Run("every failure is a state with a reason, never an error", func(t *testing.T) {
		t.Parallel()

		server := flagstest.NewServer()
		t.Cleanup(server.Close)

		limited := newProvider(t, server, nil).Status(t.Context(), request(flagstest.RateLimited, "production"))
		test.Eq(t, flags.StateUnavailable, limited.State)
		test.StrContains(t, limited.Reason, "rate limiting")

		refused := newProvider(t, server, &flags.LaunchDarklyConfig{Token: func(context.Context) (string, error) { return "api-wrong", nil }}).
			Status(t.Context(), request("new-checkout-flow", "production"))
		test.Eq(t, flags.StateUnavailable, refused.State)
		test.StrContains(t, refused.Reason, "refused the token")

		noEnv := newProvider(t, server, nil).Status(t.Context(), request("new-checkout-flow", "nowhere"))
		test.Eq(t, flags.StateUnavailable, noEnv.State)
		test.StrContains(t, noEnv.Reason, `no environment "nowhere"`)

		broken := newProvider(t, server, &flags.LaunchDarklyConfig{Token: func(context.Context) (string, error) { return "", errors.New("locked") }}).
			Status(t.Context(), request("new-checkout-flow", "production"))
		test.Eq(t, flags.StateUnavailable, broken.State)
		test.StrContains(t, broken.Reason, "credential store")

		gone := flagstest.NewServer()
		provider := newProvider(t, gone, nil)
		gone.Close()

		unreachable := provider.Status(t.Context(), request("new-checkout-flow", "production"))
		test.Eq(t, flags.StateUnavailable, unreachable.State)
		test.StrContains(t, unreachable.Reason, "can't be reached")
	})

	T.Run("with no token nothing is asked", func(t *testing.T) {
		t.Parallel()

		server := flagstest.NewServer()
		t.Cleanup(server.Close)

		status := newProvider(t, server, &flags.LaunchDarklyConfig{Token: func(context.Context) (string, error) { return "", nil }}).
			Status(t.Context(), request("new-checkout-flow", "production"))

		test.Eq(t, flags.StateNoToken, status.State)
		test.StrContains(t, status.Reason, flags.LaunchDarklyTokenEnvVar)
		test.Len(t, 0, server.Requests())
	})

	T.Run("a key that isn't a key never reaches the network", func(t *testing.T) {
		t.Parallel()

		server := flagstest.NewServer()
		t.Cleanup(server.Close)

		provider := newProvider(t, server, nil)

		for _, bad := range []*flags.Request{
			{Project: flagstest.Project, Flag: "../../segments/default/production/x", Environment: "production"},
			{Project: "default/../other", Flag: "new-checkout-flow", Environment: "production"},
			{Project: flagstest.Project, Flag: "new-checkout-flow", Environment: "production&env=staging"},
			{Project: flagstest.Project, Flag: "", Environment: "production"},
		} {
			test.Eq(t, flags.StateUnavailable, provider.Status(t.Context(), bad).State, test.Sprint(bad.Flag))
		}

		test.Len(t, 0, server.Requests())
	})

	T.Run("the token only goes to an https origin, and a redirect is not followed", func(t *testing.T) {
		t.Parallel()

		var elsewhere []string

		thief := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			elsewhere = append(elsewhere, r.Header.Get("Authorization"))
		}))
		t.Cleanup(thief.Close)

		redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, thief.URL+r.URL.Path, http.StatusFound)
		}))
		t.Cleanup(redirector.Close)

		redirected := flags.NewLaunchDarkly(&flags.LaunchDarklyConfig{Host: redirector.URL, Token: token, Transport: redirector.Client().Transport}).
			Status(t.Context(), request("new-checkout-flow", "production"))
		test.Eq(t, flags.StateUnavailable, redirected.State)
		test.StrContains(t, redirected.Reason, "redirects")
		test.Len(t, 0, elsewhere)

		for _, host := range []string{"http://ld.example.com", "ld.example.com", "https://user:pw@ld.example.com", "https://ld.example.com/path", "https://ld.example.com?x=1", "file:///etc/passwd"} {
			asked := false
			provider := flags.NewLaunchDarkly(&flags.LaunchDarklyConfig{Host: host, Token: func(context.Context) (string, error) {
				asked = true
				return flagstest.Token, nil
			}})

			test.Eq(t, flags.StateUnavailable, provider.Status(t.Context(), request("new-checkout-flow", "production")).State, test.Sprint(host))
			test.False(t, asked, test.Sprint(host))
		}
	})

	T.Run("an answer is remembered for a minute, unless a fresh one is asked for", func(t *testing.T) {
		t.Parallel()

		server := flagstest.NewServer()
		t.Cleanup(server.Close)

		now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
		provider := newProvider(t, server, &flags.LaunchDarklyConfig{Now: func() time.Time { return now }})

		flagRequests := func() int {
			count := 0

			for _, r := range server.Requests() {
				if strings.Contains(r, "/flags/") && strings.Contains(r, "env=") {
					count++
				}
			}

			return count
		}

		provider.Status(t.Context(), request("one-page-checkout", "staging"))
		provider.Status(t.Context(), request("one-page-checkout", "staging"))
		test.Eq(t, 1, flagRequests())

		// Another environment is another answer.
		provider.Status(t.Context(), request("one-page-checkout", "production"))
		test.Eq(t, 2, flagRequests())

		fresh := request("one-page-checkout", "staging")
		fresh.Fresh = true
		provider.Status(t.Context(), fresh)
		test.Eq(t, 3, flagRequests())

		now = now.Add(61 * time.Second)
		provider.Status(t.Context(), request("one-page-checkout", "staging"))
		test.Eq(t, 4, flagRequests())

		// A flag that isn't there is remembered too; a failure is not.
		provider.Status(t.Context(), request("one-click-reorder", "staging"))
		provider.Status(t.Context(), request("one-click-reorder", "staging"))
		provider.Status(t.Context(), request(flagstest.RateLimited, "staging"))
		provider.Status(t.Context(), request(flagstest.RateLimited, "staging"))
		test.Eq(t, 7, flagRequests())
	})
}

func TestOrigin(T *testing.T) {
	T.Parallel()

	T.Run("no host means LaunchDarkly's own, and a trailing slash is forgiven", func(t *testing.T) {
		t.Parallel()

		origin, problem := flags.Origin("")
		test.Eq(t, flags.LaunchDarklyDefaultHost, origin)
		test.Eq(t, "", problem)

		origin, problem = flags.Origin("https://app.eu.launchdarkly.com/")
		test.Eq(t, "https://app.eu.launchdarkly.com", origin)
		test.Eq(t, "", problem)
	})
}

func TestSettings(T *testing.T) {
	T.Parallel()

	settings := &flags.Settings{Project: "default", Environment: "prod", Environments: map[string]string{"prod": "production", "staging": "staging"}}

	T.Run("the environment is mapped, and the mapped ones are pinned", func(t *testing.T) {
		t.Parallel()

		got, problem := settings.Request("", "my-flag")
		must.Eq(t, "", problem)
		test.Eq(t, &flags.Request{Project: "default", Flag: "my-flag", Environment: "production", Pinned: []string{"production", "staging"}}, got)
	})

	T.Run("a card's own project wins, and an unmapped environment is used as it is", func(t *testing.T) {
		t.Parallel()

		got, problem := (&flags.Settings{Project: "default", Environment: "qa"}).Request("billing", "my-flag")
		must.Eq(t, "", problem)
		test.Eq(t, "billing", got.Project)
		test.Eq(t, "qa", got.Environment)
	})

	T.Run("what is missing or malformed is said", func(t *testing.T) {
		t.Parallel()

		for _, c := range []struct {
			settings      flags.Settings
			project, flag string
			want          string
		}{
			{settings: *settings, flag: "has space", want: "flag key"},
			{settings: flags.Settings{Environment: "prod"}, flag: "f", want: "no `project`"},
			{settings: *settings, project: "a/b", flag: "f", want: "project key"},
			{settings: flags.Settings{Project: "default"}, flag: "f", want: "no `environment`"},
			{settings: flags.Settings{Project: "default", Environment: "a b"}, flag: "f", want: "environment key"},
		} {
			got, problem := c.settings.Request(c.project, c.flag)
			test.Nil(t, got)
			test.StrContains(t, problem, c.want)
		}
	})
}

func TestMemory(T *testing.T) {
	T.Parallel()

	T.Run("it answers what it was given, and not found otherwise", func(t *testing.T) {
		t.Parallel()

		provider := &flags.Memory{Statuses: map[string]flags.Status{"default/a/production": flags.Empty(flags.StateOn, "")}}

		test.Eq(t, flags.StateOn, provider.Status(t.Context(), &flags.Request{Project: "default", Flag: "a", Environment: "production"}).State)
		test.Eq(t, flags.StateNotFound, provider.Status(t.Context(), &flags.Request{Project: "default", Flag: "b", Environment: "production"}).State)
		test.Len(t, 2, provider.Asked)
	})
}
