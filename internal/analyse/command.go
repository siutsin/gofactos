// This file defines the SSA analysis command exposed by the root CLI.
package analyse

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/siutsin/gofactos/internal/ssaloader"
)

// NewCommand returns the analyse subcommand with all flags and arguments configured.
func NewCommand() *cli.Command {
	var files []string

	return &cli.Command{
		Name:  "analyse",
		Usage: "Analyse a Go source file and print its SSA intermediate representation",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "func",
				Usage: "Target a specific function by name",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArgs{
				Name:        "file",
				Min:         1,
				Max:         -1,
				UsageText:   "paths to Go source files",
				Destination: &files,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			pkg, err := ssaloader.Load(files...)
			if err != nil {
				return cli.Exit(err, 1)
			}
			if err = Dump(cmd.Root().Writer, pkg, cmd.String("func")); err != nil {
				return cli.Exit(err, 1)
			}
			return nil
		},
	}
}
