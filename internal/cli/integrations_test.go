package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/primandproper/docent/internal/flags"
	"github.com/primandproper/docent/internal/secrets"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const testToken = "api-1234-not-a-real-token"

// machine is one reader's setup: a credential store, an environment and a
// user config, none of them the real ones.
type machine struct {
	_          struct{}
	store      *secrets.Memory
	env        map[string]string
	logDir     string
	userConfig string
}

func newMachine(t *testing.T, userConfig string) *machine {
	t.Helper()

	m := &machine{store: &secrets.Memory{}, env: map[string]string{}, logDir: t.TempDir(), userConfig: filepath.Join(t.TempDir(), "config.json")}

	if userConfig != "" {
		must.NoError(t, os.WriteFile(m.userConfig, []byte(userConfig), 0o600))
	}

	return m
}

func (m *machine) run(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	var out, errOut bytes.Buffer

	code = Execute(t.Context(), &Options{
		Args:           args,
		Streams:        Streams{Stdout: &out, Stderr: &errOut, Stdin: strings.NewReader(stdin)},
		LogDir:         m.logDir,
		UserConfigPath: m.userConfig,
		LibraryPath:    filepath.Join(t.TempDir(), "library.json"),
		Secrets:        m.store,
		Getenv:         func(name string) string { return m.env[name] },
	})

	return code, out.String(), errOut.String()
}

func TestIntegrations(T *testing.T) {
	T.Parallel()

	T.Run("a fresh machine has the experiment off, no token, and LaunchDarkly's own host", func(t *testing.T) {
		t.Parallel()

		code, stdout, stderr := newMachine(t, "").run(t, "", "integrations")
		must.Eq(t, ExitClean, code, must.Sprint(stderr))

		test.StrContains(t, stdout, "launchdarkly\n")
		test.StrContains(t, stdout, "experiment       off; turn it on with")
		test.StrContains(t, stdout, "DOCENT_EXPERIMENTAL=integrations")
		test.StrContains(t, stdout, "token            not set")
		test.StrContains(t, stdout, "host             https://app.launchdarkly.com\n")
	})

	T.Run("a token is stored from standard input, found again, and never printed or logged", func(t *testing.T) {
		t.Parallel()

		m := newMachine(t, `{"experimental": {"integrations": true}}`)

		code, stdout, stderr := m.run(t, testToken+"\n", "integrations", "set", "launchdarkly")
		must.Eq(t, ExitClean, code, must.Sprint(stderr))
		test.Eq(t, "Stored the LaunchDarkly token in the credential store.\n", stdout)
		test.Eq(t, map[string]string{flags.LaunchDarkly: testToken}, m.store.Stored)

		code, stdout, _ = m.run(t, "", "integrations")
		must.Eq(t, ExitClean, code)
		test.StrContains(t, stdout, "experiment       on\n")
		test.StrContains(t, stdout, "token            set (credential store)\n")

		code, doctor, _ := m.run(t, "", "doctor", t.TempDir())
		must.Eq(t, ExitClean, code)
		test.StrContains(t, doctor, "integrations: launchdarkly\n")
		test.StrContains(t, doctor, "token            set (credential store)\n")

		logged, err := os.ReadFile(filepath.Join(m.logDir, "docent.log"))
		must.NoError(t, err)

		for _, said := range []string{stdout, doctor, string(logged)} {
			test.StrNotContains(t, said, testToken)
		}
	})

	T.Run("the environment's token is used ahead of the stored one", func(t *testing.T) {
		t.Parallel()

		m := newMachine(t, "")
		m.env[flags.LaunchDarklyTokenEnvVar] = "api-from-the-environment"
		m.env["DOCENT_EXPERIMENTAL"] = "integrations"

		code, stdout, _ := m.run(t, testToken, "integrations", "set", "launchdarkly")
		must.Eq(t, ExitClean, code)
		test.StrContains(t, stdout, flags.LaunchDarklyTokenEnvVar+" is also set, and is used ahead of it.")

		code, stdout, _ = m.run(t, "", "integrations")
		must.Eq(t, ExitClean, code)
		test.StrContains(t, stdout, "experiment       on\n")
		test.StrContains(t, stdout, "token            set (environment)\n")
		test.StrNotContains(t, stdout, "api-from-the-environment")
	})

	T.Run("storing a token while the experiment is off says nothing uses it yet", func(t *testing.T) {
		t.Parallel()

		code, stdout, _ := newMachine(t, "").run(t, testToken, "integrations", "set", "launchdarkly")
		must.Eq(t, ExitClean, code)
		test.StrContains(t, stdout, "The integrations experiment is off")
	})

	T.Run("what isn't a token is refused, and nothing is stored", func(t *testing.T) {
		t.Parallel()

		for _, bad := range []string{"", "\n", "two words", "tab\tinside", "naïve"} {
			m := newMachine(t, "")

			code, _, stderr := m.run(t, bad, "integrations", "set", "launchdarkly")
			test.Eq(t, ExitErrors, code, test.Sprint(bad))
			test.StrContains(t, stderr, "token")
			test.MapEmpty(t, m.store.Stored)
		}
	})

	T.Run("a token is removed, and removing none is not an error", func(t *testing.T) {
		t.Parallel()

		m := newMachine(t, "")
		m.store.Stored = map[string]string{flags.LaunchDarkly: testToken}

		code, stdout, _ := m.run(t, "", "integrations", "remove", "launchdarkly")
		must.Eq(t, ExitClean, code)
		test.Eq(t, "Removed the LaunchDarkly token from the credential store.\n", stdout)
		test.MapEmpty(t, m.store.Stored)

		code, stdout, _ = m.run(t, "", "integrations", "remove", "launchdarkly")
		must.Eq(t, ExitClean, code)
		test.Eq(t, "There was no LaunchDarkly token in the credential store.\n", stdout)
	})

	T.Run("there is only one integration", func(t *testing.T) {
		t.Parallel()

		m := newMachine(t, "")

		for _, verb := range []string{"set", "remove"} {
			code, _, stderr := m.run(t, testToken, "integrations", verb, "split")
			test.Eq(t, ExitErrors, code)
			test.StrContains(t, stderr, `"split": unknown integration`)
		}

		test.MapEmpty(t, m.store.Stored)
	})

	T.Run("the host comes from the user config or the environment, and a bad one is said to be bad", func(t *testing.T) {
		t.Parallel()

		m := newMachine(t, `{"integrations": {"launchdarkly": {"host": "https://app.eu.launchdarkly.com/"}}}`)

		_, stdout, _ := m.run(t, "", "integrations")
		test.StrContains(t, stdout, "host             https://app.eu.launchdarkly.com\n")

		m.env[flags.LaunchDarklyHostEnvVar] = "http://insecure.example.com"

		_, stdout, _ = m.run(t, "", "integrations")
		test.StrContains(t, stdout, "host             http://insecure.example.com (the configured LaunchDarkly host must be https)\n")
	})
}

func TestDoctorIntegrations(T *testing.T) {
	T.Parallel()

	T.Run("doctor shows which flags a folder reads, and that a workspace file's host is ignored", func(t *testing.T) {
		t.Parallel()

		root := plainWorkspace(t, map[string]string{"clean.tour": cleanTour})
		must.NoError(t, os.WriteFile(filepath.Join(root, "docent.json"), []byte(`{
			"integrations": {"launchdarkly": {
				"host": "https://evil.example.com",
				"project": "default",
				"environment": "{{env}}",
				"environments": {"prod": "production", "dev": "development"}
			}}
		}`), 0o600))

		code, stdout, stderr := newMachine(t, "").run(t, "", "doctor", root)
		must.Eq(t, ExitClean, code, must.Sprint(stderr))

		for _, want := range []string{
			"integrations: launchdarkly\n",
			"host             https://app.launchdarkly.com\n",
			"project          default\n",
			"environment      {{env}}\n",
			"environments     dev is development, prod is production\n",
			"note             the workspace file sets a LaunchDarkly host, which is ignored",
		} {
			test.StrContains(t, stdout, want)
		}

		test.StrNotContains(t, stdout, "host             https://evil.example.com")
	})
}
