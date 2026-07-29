// This file assembles the reusable root command for every executable entry.
package app

import (
	"github.com/urfave/cli/v3"

	"github.com/siutsin/gofactos/internal/analyse"
	"github.com/siutsin/gofactos/internal/blueprint"
)

// NewCommand returns the root CLI command with all subcommands configured.
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:                  "gofactos",
		Usage:                 "A CLI tool",
		Version:               versionString(),
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			analyse.NewCommand(),
			blueprint.NewCommand(),
		},
	}
}
