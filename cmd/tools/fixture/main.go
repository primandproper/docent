// Command fixture runs the app's real service against a workspace and writes
// everything the frontend would have asked it as one JSON file. The browser
// tests load that file in place of the Wails bindings, so they exercise the
// real frontend against real answers without a webview.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/primandproper/docent/internal/app"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/validate"
	"github.com/primandproper/docent/internal/walk"
)

type fixture struct {
	_         struct{}                   `json:"-"`
	Views     map[string]*app.TourView   `json:"views"`
	Steps     map[string][]*app.StepView `json:"steps"`
	Workspace app.Workspace              `json:"workspace"`
	Tours     []app.TourSummary          `json:"tours"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: fixture <workspace> <output.json>")
		os.Exit(2)
	}

	if err := run(context.Background(), os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "fixture: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, workspace, output string) error {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return fmt.Errorf("resolving workspace: %w", err)
	}

	if root, err = filepath.EvalSymlinks(root); err != nil {
		return fmt.Errorf("resolving workspace: %w", err)
	}

	store := tour.NewStore(nil, nil)
	walker := walk.NewWalker(gitsrc.NewWorkingTree(nil, nil), nil, nil)
	service := app.NewService(root, store, walker, validate.NewValidator(store, walker, nil, nil), nil, nil)

	out := fixture{
		Workspace: service.Workspace(),
		Views:     map[string]*app.TourView{},
		Steps:     map[string][]*app.StepView{},
	}

	if out.Tours, err = service.ListTours(ctx); err != nil {
		return fmt.Errorf("listing tours: %w", err)
	}

	for i := range out.Tours {
		path := out.Tours[i].Path
		if out.Tours[i].Unreadable {
			continue
		}

		view, openErr := service.OpenTour(ctx, path)
		if openErr != nil {
			return fmt.Errorf("opening %q: %w", path, openErr)
		}

		out.Views[path] = view

		for index := range view.Tour.Steps {
			step, stepErr := service.Step(ctx, path, index)
			if stepErr != nil {
				return fmt.Errorf("resolving step %d of %q: %w", index, path, stepErr)
			}

			out.Steps[path] = append(out.Steps[path], step)
		}
	}

	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding fixture: %w", err)
	}

	//nolint:gosec // a developer tool writing where its own caller said to
	if err = os.MkdirAll(filepath.Dir(output), 0o750); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	//nolint:gosec // a developer tool writing where its own caller said to
	if err = os.WriteFile(output, encoded, 0o600); err != nil {
		return fmt.Errorf("writing fixture: %w", err)
	}

	return nil
}
