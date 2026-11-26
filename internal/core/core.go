// Package core assembles the services behind both faces of Docent. The app
// and the CLI build the same Core for a folder, which is what keeps `docent
// validate` consistent with what the app shows.
package core

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/primandproper/docent/internal/flags"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/links"
	"github.com/primandproper/docent/internal/secrets"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/validate"
	"github.com/primandproper/docent/internal/walk"
	"github.com/primandproper/docent/internal/workspace"

	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

type (
	// Options say which folder to assemble services for.
	Options struct {
		_ struct{}
		// VarFlags are link variables given on the command line.
		VarFlags       map[string]string
		Logger         logging.Logger
		TracerProvider tracing.Provider
		// Root is the absolute path of the opened folder.
		Root string
		// Elsewhere lists the tours of the other folders that are open, when
		// there are any, so a link into one of them can be checked.
		Elsewhere func(ctx context.Context) ([]string, error)
		// Secrets overrides the operating system's credential store. Tests
		// set it, so they never touch the real one.
		Secrets secrets.Store
		// Flags overrides the provider flag cards read from. Tests set it.
		Flags flags.Provider
		// Getenv overrides os.Getenv. Tests set it.
		Getenv func(string) string
		// UserConfigPath overrides ~/.config/docent/config.json. Tests set it.
		UserConfigPath string
	}

	// Core is the set of services for one opened folder.
	Core struct {
		_         struct{}
		Config    *workspace.Config
		Vars      *links.Vars
		Store     tour.Store
		Source    gitsrc.FileSource
		Git       gitsrc.Git
		Repos     workspace.RepoResolver
		Walker    walk.Walker
		Validator validate.Validator
		// Secrets is the credential store integrations keep their tokens in.
		Secrets secrets.Store
		// Flags reads feature flags for flag cards. It is nil while the
		// integrations experiment is off: then nothing reaches the network.
		Flags flags.Provider
		// Getenv reads the environment.
		Getenv func(string) string
	}
)

// New loads the folder's configuration and wires the services up.
func New(opts *Options) (*Core, error) {
	var err error

	userPath := opts.UserConfigPath
	if userPath == "" {
		if userPath, err = workspace.UserConfigPath(); err != nil {
			return nil, fmt.Errorf("locating user config: %w", err)
		}
	}

	// With no folder open there is no workspace file to look for, only the
	// user's own config.
	config := &workspace.Config{}
	if opts.Root != "" {
		if config, err = workspace.Load(opts.Root, userPath); err != nil {
			return nil, fmt.Errorf("loading configuration: %w", err)
		}
	} else if _, statErr := os.Stat(userPath); statErr == nil {
		if config, err = workspace.Load(os.TempDir(), userPath); err != nil {
			return nil, fmt.Errorf("loading configuration: %w", err)
		}

		config.Root, config.Workspace, config.WorkspacePath = "", workspace.File{}, ""
	}

	git := gitsrc.NewGit(opts.Logger, opts.TracerProvider)

	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	store := opts.Secrets
	if store == nil {
		store = secrets.NewStore(opts.Logger, opts.TracerProvider)
	}

	c := &Core{
		Secrets: store,
		Getenv:  getenv,
		Config:  config,
		Vars:    links.NewVars(config, opts.VarFlags),
		Store:   tour.NewStore(opts.Logger, opts.TracerProvider),
		Source:  git,
		Git:     git,
		Repos:   workspace.NewResolver(config, opts.Logger, opts.TracerProvider),
	}

	if c.IntegrationsOn() {
		if c.Flags = opts.Flags; c.Flags == nil {
			c.Flags = flags.NewLaunchDarkly(&flags.LaunchDarklyConfig{
				Host:           c.LaunchDarklyHost(),
				Logger:         opts.Logger,
				TracerProvider: opts.TracerProvider,
				Token: func(ctx context.Context) (string, error) {
					token, _, tokenErr := c.LaunchDarklyToken(ctx)
					return token, tokenErr
				},
			})
		}
	}

	c.Walker = walk.NewWalker(&walk.Dependencies{
		Source:         c.Source,
		Git:            c.Git,
		Repos:          c.Repos,
		Vars:           c.Vars,
		Logger:         opts.Logger,
		TracerProvider: opts.TracerProvider,
	})
	// Links between tours are checked against the tours this folder shows,
	// then those of any other folder that is open. With no folder open there
	// are none.
	discover := func(ctx context.Context) ([][]string, error) {
		if config.Root == "" {
			return nil, nil
		}

		own, discoverErr := c.Store.Discover(ctx, config.Root, config.TourPaths()...)
		if discoverErr != nil {
			return nil, discoverErr
		}

		tiers := [][]string{own}

		if opts.Elsewhere != nil {
			others, elseErr := opts.Elsewhere(ctx)
			if elseErr != nil {
				return nil, elseErr
			}

			tiers = append(tiers, others)
		}

		return tiers, nil
	}

	c.Validator = validate.NewValidator(c.Store, c.Walker, discover, opts.Logger, opts.TracerProvider)

	return c, nil
}

// IntegrationsOn reports whether the integrations experiment is switched on:
// in the user config, or for this run in the environment.
func (c *Core) IntegrationsOn() bool {
	if c.Config.Experiment(workspace.ExperimentIntegrations) {
		return true
	}

	names := strings.FieldsFunc(c.Getenv(workspace.ExperimentalEnvVar), func(r rune) bool { return r == ',' || r == ' ' })

	return slices.Contains(names, workspace.ExperimentIntegrations)
}

// LaunchDarklyHost is where LaunchDarkly is asked: the environment's host,
// then the user config's, or "" for LaunchDarkly's own. Never a workspace
// file's or a tour's.
func (c *Core) LaunchDarklyHost() string {
	if host := c.Getenv(flags.LaunchDarklyHostEnvVar); host != "" {
		return host
	}

	return c.Config.LaunchDarkly().Host
}

// LaunchDarklyToken finds the LaunchDarkly token and says where it was
// found. Both are empty when there is none.
func (c *Core) LaunchDarklyToken(ctx context.Context) (token, source string, err error) {
	return secrets.Resolve(ctx, c.Secrets, c.Getenv, flags.LaunchDarklyTokenEnvVar, flags.LaunchDarkly)
}

// Close stops the helper processes the services started.
func (c *Core) Close() {
	if c.Git != nil {
		c.Git.Close()
	}
}
