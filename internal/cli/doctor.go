package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/primandproper/docent/internal/editor"
	"github.com/primandproper/docent/internal/library"
	"github.com/primandproper/docent/internal/logs"
	"github.com/primandproper/docent/internal/version"
	"github.com/primandproper/docent/internal/workspace"

	"github.com/spf13/cobra"
)

func (a *application) newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor [path]",
		Short: "Show what Docent sees: config, repos, editors, git and the webview.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			return a.doctor(cmd.Context(), path)
		},
	}
}

// doctor prints a diagnosis of the folder at path. It reports and never
// fails: a doctor that exits on the first thing wrong can't describe the rest.
func (a *application) doctor(ctx context.Context, path string) error {
	var b strings.Builder

	line := func(label, value string) {
		fmt.Fprintf(&b, "  %-16s %s\n", label, value)
	}

	root, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving %q: %w", path, err)
	}

	b.WriteString("docent\n")
	line("version", version.Version)
	line("platform", runtime.GOOS+"/"+runtime.GOARCH+", "+command(ctx, "sw_vers", "-productVersion"))
	line("git", strings.TrimPrefix(command(ctx, "git", "--version"), "git version "))
	line("webview", "WebKit "+command(ctx, "plutil", "-extract", "CFBundleVersion", "raw", "/System/Library/Frameworks/WebKit.framework/Resources/Info.plist"))

	if dir, dirErr := logs.DefaultDir(); dirErr == nil {
		if a.logDir != "" {
			dir = a.logDir
		}

		line("logs", filepath.Join(dir, "docent.log"))
	}

	services, err := a.coreFor(root)
	if err != nil {
		fmt.Fprintf(&b, "\nconfig\n  %v\n", err)

		_, writeErr := io.WriteString(a.streams.Stdout, b.String())

		return writeErr
	}
	defer services.Close()

	cfg := services.Config

	b.WriteString("\nconfig\n")
	line("folder", root)
	line("workspace file", orNone(cfg.WorkspacePath))
	line("user config", describeUserConfig(a.userConfigPath, cfg.UserPath))
	line("repo roots", strings.Join(cfg.RepoRoots(), ", "))
	line("tour paths", orNone(strings.Join(cfg.TourPaths(), ", ")))

	// The app opens these beside whatever folder it is given. Commands like
	// validate and stale never look at them.
	b.WriteString("\nlibrary\n")

	libraryPath := a.libraryPath
	if libraryPath == "" {
		libraryPath, _ = library.Path() //nolint:errcheck // an empty path reads as an empty library
	}

	line("file", libraryPath)

	saved, libErr := library.NewStore(libraryPath, a.log(), a.tracerProvider).Folders(ctx)

	switch {
	case libErr != nil:
		line("error", libErr.Error())
	case len(saved) == 0:
		line("folders", "(none)")
	default:
		for _, folder := range saved {
			note := ""
			if info, statErr := os.Stat(folder); statErr != nil || !info.IsDir() {
				note = " (not there)"
			}

			line("folder", folder+note)
		}
	}

	b.WriteString("\neditors\n")

	detected := editor.NewLauncher(cfg, a.log(), a.tracerProvider).Detect(ctx)
	names := make([]string, len(detected))

	for i := range detected {
		names[i] = detected[i].ID
	}

	line("installed", orNone(strings.Join(names, ", ")))
	line("default", orNone(cfg.DefaultEditor()))
	line("rules", fmt.Sprintf("%d in the workspace file, %d in the user config", len(cfg.Workspace.Editors.Rules), len(cfg.User.Editors.Rules)))

	b.WriteString("\ntours\n")

	paths, err := services.Store.Discover(ctx, root, cfg.TourPaths()...)
	if err != nil {
		line("error", err.Error())
	}

	if len(paths) == 0 && err == nil {
		b.WriteString("  none found (looked in .tours, .vscode/tours, .github/tours and the tour paths)\n")
	}

	for _, tourPath := range paths {
		t, diags, loadErr := services.Store.Load(ctx, tourPath)
		if loadErr != nil || t == nil {
			fmt.Fprintf(&b, "  %s\n    can't be read (%d problems)\n", tourPath, len(diags))
			continue
		}

		fmt.Fprintf(&b, "  %s\n", tourPath)

		for i := range t.Repos {
			state, repoErr := services.Walker.Repo(ctx, t, t.Repos[i].Name)
			if repoErr != nil {
				fmt.Fprintf(&b, "    %-12s error: %v\n", t.Repos[i].Name, repoErr)
				continue
			}

			if !state.Found {
				fmt.Fprintf(&b, "    %-12s missing; %s\n", state.Name, strings.Join(state.CloneCommand, " "))
				continue
			}

			checkout, _ := services.Repos.Resolve(ctx, &t.Repos[i]) //nolint:errcheck // Repo above already resolved it

			reading := "the working tree"
			if state.Ref != nil && state.Ref.Commit != "" {
				reading = fmt.Sprintf("%s at %s", state.Ref.Name, shortSHA(state.Ref.Commit))
			}

			fmt.Fprintf(&b, "    %-12s %s (by %s), reading %s\n", state.Name, state.Root, how(checkout), reading)
		}
	}

	_, err = io.WriteString(a.streams.Stdout, b.String())

	return err
}

func how(checkout *workspace.Checkout) string {
	if checkout == nil {
		return "unknown"
	}

	return string(checkout.How)
}

func describeUserConfig(override, found string) string {
	path := override
	if path == "" {
		if p, err := workspace.UserConfigPath(); err == nil {
			path = p
		}
	}

	if found == "" {
		return path + " (not present)"
	}

	return found
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}

	return s
}

// command runs a diagnostic command and returns its first line, or says it
// couldn't.
func command(ctx context.Context, name string, args ...string) string {
	out, err := exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // fixed commands with fixed arguments
	if err != nil {
		if _, lookErr := exec.LookPath(name); lookErr != nil {
			return "not found (" + name + " isn't on the PATH)"
		}

		return "unknown"
	}

	first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")

	return first
}
