// Command docent is both the desktop app and its CLI: `docent [path]` opens a
// window, and any subcommand runs headless.
package main

import (
	"context"
	"embed"
	"os"
	"os/signal"
	"syscall"

	"github.com/primandproper/docent/internal/cli"
)

// The frontend is built into frontend/dist and embedded, so the binary is
// self-contained.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return cli.Execute(ctx, &cli.Options{
		Assets:  assets,
		Args:    os.Args[1:],
		Streams: cli.Streams{Stdout: os.Stdout, Stderr: os.Stderr},
	})
}
