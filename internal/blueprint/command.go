// This file defines blueprint generation as a user-facing CLI command.
package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/tools/go/ssa"

	"github.com/siutsin/gofactos/internal/factorio"
	"github.com/siutsin/gofactos/internal/ssaloader"
)

// parseSet turns repeated --set name=value entries into a parameter map. Each
// entry must be name=integer; a missing '=', empty name, or non-integer value
// is an error.
func parseSet(entries []string) (map[string]int, error) {
	out := make(map[string]int, len(entries))
	for _, e := range entries {
		name, val, ok := strings.Cut(e, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid --set %q: want name=value", e)
		}
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return nil, fmt.Errorf("invalid --set %q: value must be an integer", e)
		}
		out[name] = n
	}
	return out, nil
}

// NewCommand returns the blueprint subcommand with its flags and arguments.
func NewCommand() *cli.Command {
	var files []string

	return &cli.Command{
		Name:  "blueprint",
		Usage: "Generate a Factorio blueprint from a Go source file",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "fast",
				Usage: "Use the fastest timing-safe runtime clock",
			},
			&cli.StringFlag{
				Name:  "func",
				Usage: "Target a specific function by name",
			},
			&cli.BoolFlag{
				Name: "json",
				Usage: "Print the blueprint as pretty-printed JSON " +
					"(before zlib/base64 encoding)",
			},
			&cli.StringSliceFlag{
				Name:  "set",
				Usage: "Set a parameter's initial value, e.g. --set n=4 (repeatable)",
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
			return runCommand(cmd, files)
		},
	}
}

// runCommand keeps loading, function selection, and compilation failures clear.
func runCommand(cmd *cli.Command, files []string) error {
	pkg, parseErr := ssaloader.Load(files...)
	if parseErr != nil {
		return cli.Exit(parseErr, 1)
	}
	funcFilter := cmd.String("func")
	fns := ssaloader.CollectFunctions(pkg, funcFilter)
	fn, selectErr := selectBlueprintFunction(fns, funcFilter)
	if selectErr != nil {
		return selectErr
	}

	opts, optionsErr := commandOptions(cmd)
	if optionsErr != nil {
		return cli.Exit(optionsErr, 1)
	}
	bp, compileErr := factorio.Compile(fn, opts...)
	if compileErr != nil {
		return cli.Exit(compileErr, 1)
	}
	return writeBlueprint(cmd, bp)
}

// selectBlueprintFunction applies the command's root-selection policy.
func selectBlueprintFunction(
	fns []*ssa.Function,
	funcFilter string,
) (*ssa.Function, error) {
	// Without --func, automatic selection considers package functions only.
	if funcFilter == "" {
		fns = topLevelFunctions(fns)
	}
	if len(fns) == 0 {
		if funcFilter == "" {
			return nil, cli.Exit("no functions found", 1)
		}
		return nil, cli.Exit(
			fmt.Sprintf("function %q not found", funcFilter),
			1,
		)
	}
	if len(fns) == 1 {
		return fns[0], nil
	}
	if funcFilter != "" {
		if topLevel := uniqueTopLevelFunction(fns); topLevel != nil {
			return topLevel, nil
		}
		return nil, cli.Exit(fmt.Sprintf(
			"multiple functions named %q found; "+
				"--func cannot disambiguate by name",
			funcFilter,
		), 1)
	}
	names := make([]string, len(fns))
	for i, fn := range fns {
		names[i] = fn.Name()
	}
	return nil, cli.Exit(fmt.Sprintf(
		"multiple functions found, use --func to select one, e.g. [%s]",
		strings.Join(names, ", "),
	), 1)
}

// topLevelFunctions removes methods and closures from implicit root selection.
func topLevelFunctions(fns []*ssa.Function) []*ssa.Function {
	roots := make([]*ssa.Function, 0, len(fns))
	for _, fn := range fns {
		// A receiver marks a method; a parent marks a nested closure.
		if fn.Signature.Recv() == nil && fn.Parent() == nil {
			roots = append(roots, fn)
		}
	}
	return roots
}

// uniqueTopLevelFunction returns the sole package function among name matches.
func uniqueTopLevelFunction(fns []*ssa.Function) *ssa.Function {
	var found *ssa.Function
	for _, fn := range fns {
		if fn.Signature.Recv() != nil || fn.Parent() != nil {
			continue
		}
		if found != nil {
			return nil
		}
		found = fn
	}
	return found
}

// commandOptions keeps CLI settings aligned with generation options.
func commandOptions(cmd *cli.Command) ([]factorio.Option, error) {
	params, err := parseSet(cmd.StringSlice("set"))
	if err != nil {
		return nil, err
	}
	var opts []factorio.Option
	if len(params) > 0 {
		opts = append(opts, factorio.WithParams(params))
	}
	if cmd.Bool("fast") {
		opts = append(opts, factorio.WithFastClock())
	}
	return opts, nil
}

// writeBlueprint preserves the encoded and diagnostic JSON output contracts.
func writeBlueprint(
	cmd *cli.Command,
	bp *factorio.BlueprintWrapper,
) error {
	if cmd.Bool("json") {
		encoder := json.NewEncoder(cmd.Root().Writer)
		encoder.SetIndent("", "  ")
		if encodeErr := encoder.Encode(bp); encodeErr != nil {
			return cli.Exit(encodeErr, 1)
		}
		return nil
	}

	encoded, encodeErr := factorio.Encode(bp)
	if encodeErr != nil {
		return cli.Exit(encodeErr, 1)
	}
	_, writeErr := fmt.Fprintln(cmd.Root().Writer, encoded)
	if writeErr != nil {
		return cli.Exit(writeErr, 1)
	}
	return nil
}
