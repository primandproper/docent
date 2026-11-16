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
	"github.com/primandproper/docent/internal/core"
)

type fixture struct {
	_     struct{}                   `json:"-"`
	Views map[string]*app.TourView   `json:"views"`
	Steps map[string][]*app.StepView `json:"steps"`
	// Picked holds each tour again for every choice of every variable that
	// has choices, keyed "path|name=value".
	Picked    map[string]picked `json:"picked"`
	Workspace app.Workspace     `json:"workspace"`
	Tours     []app.TourSummary `json:"tours"`
}

type picked struct {
	_     struct{}        `json:"-"`
	View  *app.TourView   `json:"view"`
	Steps []*app.StepView `json:"steps"`
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

	// The user's own config must not leak into a recorded fixture.
	services, err := core.New(&core.Options{Root: root, UserConfigPath: filepath.Join(root, "no-user-config.json")})
	if err != nil {
		return fmt.Errorf("setting up: %w", err)
	}

	service := app.NewService(root, services, nil, nil, nil)

	out := fixture{
		Workspace: service.Workspace(),
		Views:     map[string]*app.TourView{},
		Steps:     map[string][]*app.StepView{},
		Picked:    map[string]picked{},
	}

	if out.Tours, err = service.ListTours(ctx); err != nil {
		return fmt.Errorf("listing tours: %w", err)
	}

	for i := range out.Tours {
		path := out.Tours[i].Path
		if out.Tours[i].Unreadable {
			continue
		}

		view, steps, recErr := record(ctx, service, path)
		if recErr != nil {
			return recErr
		}

		out.Views[path], out.Steps[path] = view, steps

		for v := range view.Vars {
			name := view.Vars[v].Name

			for _, choice := range view.Vars[v].Choices {
				if _, err = service.SetVar(ctx, path, name, choice); err != nil {
					return fmt.Errorf("picking %s=%s: %w", name, choice, err)
				}

				pickedView, pickedSteps, pickErr := record(ctx, service, path)
				if pickErr != nil {
					return pickErr
				}

				out.Picked[path+"|"+name+"="+choice] = picked{View: pickedView, Steps: pickedSteps}

				if _, err = service.SetVar(ctx, path, name, ""); err != nil {
					return fmt.Errorf("clearing %s: %w", name, err)
				}
			}
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

// record opens a tour and resolves every step of it.
func record(ctx context.Context, service *app.Service, path string) (*app.TourView, []*app.StepView, error) {
	view, err := service.OpenTour(ctx, path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening %q: %w", path, err)
	}

	steps := make([]*app.StepView, 0, len(view.Tour.Steps))

	for index := range view.Tour.Steps {
		step, stepErr := service.Step(ctx, path, index)
		if stepErr != nil {
			return nil, nil, fmt.Errorf("resolving step %d of %q: %w", index, path, stepErr)
		}

		steps = append(steps, step)
	}

	return view, steps, nil
}
