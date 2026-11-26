package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/flags"
	"github.com/primandproper/docent/internal/secrets"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func TestService_FlagStatus(T *testing.T) {
	T.Parallel()

	const (
		tourJSON = `{"docent": 1, "title": "Flags",
			"vars": { "env": { "choices": ["staging", "prod"], "default": "staging" } },
			"steps": [{"id": "s", "description": "d", "links": [{ "kind": "flag", "title": "New checkout", "flag": "new-checkout-flow" }]}]}`
		workspaceJSON = `{ "integrations": { "launchdarkly": {
			"host": "https://evil.example.com", "project": "default", "environment": "{{env}}", "environments": { "prod": "production" }
		} } }`
	)

	type world struct {
		svc      *Service
		provider *flags.Memory
		core     *core.Core
		path     string
	}

	setup := func(t *testing.T, workspace string, env map[string]string) *world {
		t.Helper()

		root, err := filepath.EvalSymlinks(t.TempDir())
		must.NoError(t, err)

		w := &world{path: filepath.Join(root, ".tours", "flags.tour"), provider: &flags.Memory{Statuses: map[string]flags.Status{
			"default/new-checkout-flow/production": flags.Empty(flags.StateOn, ""),
			"default/new-checkout-flow/staging":    flags.Empty(flags.StateOff, ""),
			"billing/new-checkout-flow/staging":    flags.Empty(flags.StateArchived, ""),
		}}}

		must.NoError(t, os.MkdirAll(filepath.Dir(w.path), 0o750))
		must.NoError(t, os.WriteFile(w.path, []byte(tourJSON), 0o600))

		if workspace != "" {
			must.NoError(t, os.WriteFile(filepath.Join(root, "docent.json"), []byte(workspace), 0o600))
		}

		w.core, err = core.New(&core.Options{
			Root:           root,
			UserConfigPath: filepath.Join(root, "no-config.json"),
			Secrets:        &secrets.Memory{},
			Flags:          w.provider,
			Getenv:         func(name string) string { return env[name] },
		})
		must.NoError(t, err)

		w.svc = NewService(root, w.core, &fakeLauncher{}, nil, nil)
		t.Cleanup(w.svc.close)

		_, err = w.svc.OpenTour(t.Context(), w.path)
		must.NoError(t, err)

		return w
	}

	on := map[string]string{"DOCENT_EXPERIMENTAL": "integrations"}

	T.Run("a step's flag card carries the keys the status is asked for with", func(t *testing.T) {
		t.Parallel()

		w := setup(t, workspaceJSON, on)

		step, err := w.svc.Step(t.Context(), w.path, 0)
		must.NoError(t, err)
		must.Len(t, 1, step.Links)
		test.Eq(t, "new-checkout-flow", step.Links[0].Flag)
		test.Eq(t, "", step.Links[0].Problem)
	})

	T.Run("the environment follows the variable picker, through the configured map", func(t *testing.T) {
		t.Parallel()

		w := setup(t, workspaceJSON, on)

		status, err := w.svc.FlagStatus(t.Context(), w.path, "new-checkout-flow", "", false)
		must.NoError(t, err)
		test.Eq(t, flags.StateOff, status.State)

		_, err = w.svc.SetVar(t.Context(), w.path, "env", "prod")
		must.NoError(t, err)

		status, err = w.svc.FlagStatus(t.Context(), w.path, "new-checkout-flow", "", true)
		must.NoError(t, err)
		test.Eq(t, flags.StateOn, status.State)

		must.Len(t, 2, w.provider.Asked)
		test.Eq(t, flags.Request{Project: "default", Flag: "new-checkout-flow", Environment: "staging", Pinned: []string{"production"}}, w.provider.Asked[0])
		test.Eq(t, flags.Request{Project: "default", Flag: "new-checkout-flow", Environment: "production", Pinned: []string{"production"}, Fresh: true}, w.provider.Asked[1])
	})

	T.Run("a card's own project is used instead of the configured one", func(t *testing.T) {
		t.Parallel()

		w := setup(t, workspaceJSON, on)

		status, err := w.svc.FlagStatus(t.Context(), w.path, "new-checkout-flow", "billing", false)
		must.NoError(t, err)
		test.Eq(t, flags.StateArchived, status.State)
	})

	T.Run("with the experiment off the card is a plain link and nothing is asked", func(t *testing.T) {
		t.Parallel()

		w := setup(t, workspaceJSON, nil)

		status, err := w.svc.FlagStatus(t.Context(), w.path, "new-checkout-flow", "", false)
		must.NoError(t, err)
		test.Eq(t, flags.StateDisabled, status.State)
		test.NotNil(t, status.Environments)
		test.Len(t, 0, w.provider.Asked)
		test.Nil(t, w.core.Flags)
	})

	T.Run("keys that aren't keys, and a folder with nothing configured, are said and never asked", func(t *testing.T) {
		t.Parallel()

		w := setup(t, workspaceJSON, on)

		for _, bad := range [][2]string{{"../../x", ""}, {"a b", ""}, {"", ""}, {"fine", "default/../other"}} {
			status, err := w.svc.FlagStatus(t.Context(), w.path, bad[0], bad[1], false)
			must.NoError(t, err)
			test.Eq(t, flags.StateUnavailable, status.State, test.Sprint(bad))
			test.NotEq(t, "", status.Reason)
		}

		bare := setup(t, "", on)

		status, err := bare.svc.FlagStatus(t.Context(), bare.path, "new-checkout-flow", "", false)
		must.NoError(t, err)
		test.Eq(t, flags.StateUnavailable, status.State)
		test.StrContains(t, status.Reason, "no `project`")

		test.Len(t, 0, w.provider.Asked)
		test.Len(t, 0, bare.provider.Asked)
	})

	T.Run("a tour that isn't open has no flags to ask about", func(t *testing.T) {
		t.Parallel()

		w := setup(t, workspaceJSON, on)

		_, err := w.svc.FlagStatus(t.Context(), filepath.Join(filepath.Dir(w.path), "other.tour"), "new-checkout-flow", "", false)
		test.ErrorIs(t, err, ErrUnknownTour)
		test.Len(t, 0, w.provider.Asked)
	})

	T.Run("a workspace file can't choose the host; the environment and the user config can", func(t *testing.T) {
		t.Parallel()

		w := setup(t, workspaceJSON, on)
		test.Eq(t, "", w.core.LaunchDarklyHost())

		w = setup(t, workspaceJSON, map[string]string{flags.LaunchDarklyHostEnvVar: "https://app.eu.launchdarkly.com"})
		test.Eq(t, "https://app.eu.launchdarkly.com", w.core.LaunchDarklyHost())
	})

	T.Run("the token is found in the environment, then the store, and is no part of a status", func(t *testing.T) {
		t.Parallel()

		w := setup(t, workspaceJSON, on)

		token, source, err := w.core.LaunchDarklyToken(t.Context())
		must.NoError(t, err)
		test.Eq(t, "", token+source)

		must.NoError(t, w.core.Secrets.Set(t.Context(), flags.LaunchDarkly, "api-stored"))

		token, source, err = w.core.LaunchDarklyToken(t.Context())
		must.NoError(t, err)
		test.Eq(t, "api-stored", token)
		test.Eq(t, secrets.SourceStore, source)
	})
}
