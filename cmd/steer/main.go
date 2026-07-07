package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/juanMaAV92/steer/internal/cli"
)

var version = "dev" // sobrescrito por GoReleaser con -ldflags

func main() {
	// Contexto raíz sensible a señales: SIGINT/SIGTERM cancelan ctx en lugar de
	// matar el proceso abruptamente, permitiendo que loops de watch y la TUI
	// limpien su estado antes de salir.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.NewRootCmd(version)
	root.AddCommand(cli.NewConfigCmd())
	root.AddCommand(cli.NewServiceCmd())
	root.AddCommand(cli.NewImageCmd())
	root.AddCommand(cli.NewTuiCmd())
	if err := root.ExecuteContext(ctx); err != nil {
		// Una cancelación por señal es una salida limpia (Ctrl+C), no un error real.
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "error:", cli.FriendlyError(err))
		os.Exit(1)
	}
}
