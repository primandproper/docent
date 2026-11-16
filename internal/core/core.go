// Package core assembles the services behind both faces of Docent. The app
// and the CLI build the same Core for a folder, which is what keeps `docent
// validate` consistent with what the app shows.
package core

import (
	"fmt"

	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/links"
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
	}
)

// New loads the folder's configuration and wires the services up.
func New(opts *Options) (*Core, error) {
	userPath := opts.UserConfigPath
	if userPath == "" {
		var err error
		if userPath, err = workspace.UserConfigPath(); err != nil {
			return nil, fmt.Errorf("locating user config: %w", err)
		}
	}

	config, err := workspace.Load(opts.Root, userPath)
	if err != nil {
		return nil, fmt.Errorf("loading configuration: %w", err)
	}

	git := gitsrc.NewGit(opts.Logger, opts.TracerProvider)

	c := &Core{
		Config: config,
		Vars:   links.NewVars(config, opts.VarFlags),
		Store:  tour.NewStore(opts.Logger, opts.TracerProvider),
		Source: git,
		Git:    git,
		Repos:  workspace.NewResolver(config, opts.Logger, opts.TracerProvider),
	}

	c.Walker = walk.NewWalker(&walk.Dependencies{
		Source:         c.Source,
		Git:            c.Git,
		Repos:          c.Repos,
		Vars:           c.Vars,
		Logger:         opts.Logger,
		TracerProvider: opts.TracerProvider,
	})
	c.Validator = validate.NewValidator(c.Store, c.Walker, opts.Logger, opts.TracerProvider)

	return c, nil
}

// Close stops the helper processes the services started.
func (c *Core) Close() {
	if c.Git != nil {
		c.Git.Close()
	}
}
