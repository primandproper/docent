package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/flags"
	"github.com/primandproper/docent/internal/secrets"
	"github.com/primandproper/docent/internal/workspace"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// maxToken bounds what is read as a token: far longer than any real one.
const maxToken = 4096

// errUnknownIntegration is returned for a name that isn't an integration.
var errUnknownIntegration = errors.New("unknown integration")

func (a *application) newIntegrationsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "integrations",
		Short: "List the integrations, or store and remove their tokens.",
		Long: "An integration lets a card show a live fact about what it links to. There is one,\n" +
			"launchdarkly, which shows a feature flag's status on a `flag` link card.\n\n" +
			"A token is kept in the operating system's credential store, never in a config file.\n" +
			"The " + flags.LaunchDarklyTokenEnvVar + " environment variable, when set, is used instead.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.listIntegrations(cmd.Context())
		},
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "set <name>",
			Short: "Store an integration's token, read from a prompt or from standard input.",
			Long: "Set reads the token from a prompt that doesn't echo, or from standard input when\n" +
				"that isn't a terminal. It is never an argument, so it stays out of shell history:\n\n" +
				"  docent integrations set launchdarkly\n" +
				"  pbpaste | docent integrations set launchdarkly",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return a.setIntegration(cmd.Context(), args[0])
			},
		},
		&cobra.Command{
			Use:   "remove <name>",
			Short: "Remove an integration's stored token.",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return a.removeIntegration(cmd.Context(), args[0])
			},
		},
	)

	return cmd
}

func knownIntegration(name string) error {
	if name != flags.LaunchDarkly {
		return fmt.Errorf("%q: %w (there is one: %s)", name, errUnknownIntegration, flags.LaunchDarkly)
	}

	return nil
}

func (a *application) listIntegrations(ctx context.Context) error {
	services, err := a.coreFor("")
	if err != nil {
		return err
	}
	defer services.Close()

	var b strings.Builder

	b.WriteString(flags.LaunchDarkly + "\n")

	rows := integrationRows(ctx, services, a.userConfigPath)
	for i := range rows {
		fmt.Fprintf(&b, "  %-16s %s\n", rows[i][0], rows[i][1])
	}

	_, err = io.WriteString(a.streams.Stdout, b.String())

	return err
}

// integrationRows describes the LaunchDarkly integration: whether it is
// switched on, whether it has a token and where from, and where it asks.
// The token itself is never part of it.
func integrationRows(ctx context.Context, services *core.Core, userConfigOverride string) [][2]string {
	experiment := "on"
	if !services.IntegrationsOn() {
		experiment = fmt.Sprintf("off; turn it on with %s in %s, or %s=%s",
			`"experimental": {"`+workspace.ExperimentIntegrations+`": true}`,
			describeUserConfig(userConfigOverride, services.Config.UserPath),
			workspace.ExperimentalEnvVar, workspace.ExperimentIntegrations)
	}

	token := "not set; store one with `docent integrations set " + flags.LaunchDarkly + "`"

	switch _, source, err := services.LaunchDarklyToken(ctx); {
	case err != nil:
		token = "unknown: " + err.Error()
	case source != "":
		token = "set (" + source + ")"
	}

	host := services.LaunchDarklyHost()

	origin, problem := flags.Origin(host)
	if problem != "" {
		origin = host + " (" + problem + ")"
	}

	return [][2]string{{"experiment", experiment}, {"token", token}, {"host", origin}}
}

func (a *application) setIntegration(ctx context.Context, name string) error {
	if err := knownIntegration(name); err != nil {
		return err
	}

	token, err := a.readSecret("LaunchDarkly access token: ")
	if err != nil {
		return err
	}

	switch {
	case token == "":
		return errors.New("no token was given")
	case strings.ContainsFunc(token, func(r rune) bool { return r < 0x21 || r > 0x7e }):
		return errors.New("that isn't a token: it has spaces or characters a token can't have")
	}

	services, err := a.coreFor("")
	if err != nil {
		return err
	}
	defer services.Close()

	if err = services.Secrets.Set(ctx, name, token); err != nil {
		return err
	}

	a.log().WithValue("integration", name).Info("stored an integration's token")

	message := "Stored the LaunchDarkly token in the credential store.\n"
	if services.Getenv(flags.LaunchDarklyTokenEnvVar) != "" {
		message += flags.LaunchDarklyTokenEnvVar + " is also set, and is used ahead of it.\n"
	}

	if !services.IntegrationsOn() {
		message += "The integrations experiment is off, so nothing uses it yet: see `docent integrations`.\n"
	}

	_, err = io.WriteString(a.streams.Stdout, message)

	return err
}

func (a *application) removeIntegration(ctx context.Context, name string) error {
	if err := knownIntegration(name); err != nil {
		return err
	}

	services, err := a.coreFor("")
	if err != nil {
		return err
	}
	defer services.Close()

	message := "Removed the LaunchDarkly token from the credential store.\n"

	switch err = services.Secrets.Delete(ctx, name); {
	case errors.Is(err, secrets.ErrNotFound):
		message = "There was no LaunchDarkly token in the credential store.\n"
	case err != nil:
		return err
	default:
		a.log().WithValue("integration", name).Info("removed an integration's token")
	}

	_, err = io.WriteString(a.streams.Stdout, message)

	return err
}

// readSecret reads one line without showing it: from a prompt when standard
// input is a terminal, and from standard input as it comes otherwise.
func (a *application) readSecret(prompt string) (string, error) {
	in := a.streams.Stdin
	if in == nil {
		return "", errors.New("there is nowhere to read the token from")
	}

	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(a.streams.Stderr, prompt) //nolint:errcheck // best effort

		secret, err := term.ReadPassword(int(file.Fd()))

		fmt.Fprintln(a.streams.Stderr) //nolint:errcheck // best effort

		if err != nil {
			return "", fmt.Errorf("reading the token: %w", err)
		}

		return strings.TrimSpace(string(secret)), nil
	}

	line, err := bufio.NewReader(io.LimitReader(in, maxToken)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading the token: %w", err)
	}

	return strings.TrimSpace(line), nil
}

// describeEnvironments renders the environment map in a stable order.
func describeEnvironments(environments map[string]string) string {
	pairs := make([]string, 0, len(environments))
	for from, to := range environments {
		pairs = append(pairs, from+" is "+to)
	}

	sort.Strings(pairs)

	return strings.Join(pairs, ", ")
}
