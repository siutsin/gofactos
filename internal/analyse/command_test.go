// This file protects the public behaviour of the SSA analysis command.
package analyse

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// runCommand isolates command execution so CLI tests share one faithful path.
func runCommand(t *testing.T, args []string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := &cli.Command{
		Name:           "gofactos",
		Commands:       []*cli.Command{NewCommand()},
		Writer:         &buf,
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
	}
	err := cmd.Run(context.Background(), args)
	return buf.String(), err
}

// TestCommandAnalyse verifies that the analyse subcommand prints the
// raw SSA dump.
func TestCommandAnalyse(t *testing.T) {
	output, err := runCommand(t, []string{"gofactos", "analyse", "../testdata/add.go"})

	require.NoError(t, err)
	assert.Contains(t, output, "func  add")
	assert.Contains(t, output, "t0 = a + b")
	assert.Contains(t, output, "return t0")
}

// TestCommandAnalyseAllFunctions verifies that an unfiltered command prints
// every function in a multi-function source.
func TestCommandAnalyseAllFunctions(t *testing.T) {
	output, err := runCommand(t, []string{
		"gofactos", "analyse", "../testdata/loader.go",
	})

	require.NoError(t, err)
	assert.Contains(t, output, "func double")
	assert.Contains(t, output, "func triple")
}

// TestCommandMissingFile verifies that the analyse subcommand without a file
// argument returns an error about insufficient arguments.
func TestCommandMissingFile(t *testing.T) {
	_, err := runCommand(t, []string{"gofactos", "analyse"})

	assert.ErrorContains(t, err, "sufficient count of arg file not provided")
}

// TestCommandSourceLoadFailure reports source-loader errors without writing a
// partial analysis.
func TestCommandSourceLoadFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.go")
	output, err := runCommand(t, []string{
		"gofactos", "analyse", missing,
	})

	require.ErrorContains(t, err, "packages contain errors")
	assert.Empty(t, output)
}

// TestCommandMissingFunc reports the requested function without exiting the
// test process.
func TestCommandMissingFunc(t *testing.T) {
	output, err := runCommand(t, []string{
		"gofactos",
		"analyse",
		"--func",
		"missing",
		"../testdata/loader.go",
	})

	require.EqualError(t, err, `function "missing" not found`)
	assert.Empty(t, output)
}

// TestCommandFuncFilter verifies that the --func flag filters the
// output to only the named function.
func TestCommandFuncFilter(t *testing.T) {
	output, err := runCommand(t, []string{
		"gofactos", "analyse", "--func", "abs", "../testdata/abs.go", "../testdata/iseven.go",
	})

	require.NoError(t, err)
	assert.Contains(t, output, "func abs")
	assert.NotContains(t, output, "func isEven")
}
