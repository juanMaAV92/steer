package main

import (
	"fmt"
	"os"

	"github.com/juanMaAV92/steer/internal/cli"
)

var version = "dev" // sobrescrito por GoReleaser con -ldflags

func main() {
	root := cli.NewRootCmd(version)
	root.AddCommand(cli.NewConfigCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
