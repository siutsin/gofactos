// This file protects blueprint CLI flags, errors, and output contracts.
package blueprint

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/siutsin/gofactos/internal/factorio"
)

// executeCommand gives tests the same urfave command path as the executable.
func executeCommand(t *testing.T, args []string) ([]byte, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := &cli.Command{
		Name:           "gofactos",
		Commands:       []*cli.Command{NewCommand()},
		Writer:         &buf,
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
	}
	err := cmd.Run(context.Background(), args)
	return buf.Bytes(), err
}

// runSourceCommand executes one testdata source through the real command.
func runSourceCommand(
	t *testing.T,
	filename string,
	options ...string,
) ([]byte, error) {
	t.Helper()
	args := append([]string{"gofactos", "blueprint"}, options...)
	args = append(args, filepath.Join("..", "testdata", filename))
	return executeCommand(t, args)
}

// TestCommandDefaultBlueprint verifies that the blueprint subcommand without
// --json outputs a Factorio blueprint string starting with "0".
func TestCommandDefaultBlueprint(t *testing.T) {
	t.Parallel()
	output, err := runSourceCommand(t, "add.go")

	require.NoError(t, err)
	assert.True(
		t,
		strings.HasPrefix(strings.TrimSpace(string(output)), "0"),
	)
}

// TestCommandJSON verifies that --json returns the requested blueprint.
func TestCommandJSON(t *testing.T) {
	t.Parallel()
	output, err := runSourceCommand(t, "add.go", "--json")

	require.NoError(t, err)
	var doc factorio.BlueprintWrapper
	require.NoError(t, json.Unmarshal(output, &doc))
	assert.Equal(t, "add", doc.Blueprint.Label)
	require.NotEmpty(t, doc.Blueprint.Entities)
	require.NotEmpty(t, doc.Blueprint.Wires)
}

// TestCommandMissingFile verifies that the blueprint subcommand without a file
// argument returns an error about insufficient arguments.
func TestCommandMissingFile(t *testing.T) {
	t.Parallel()
	output, err := executeCommand(
		t,
		[]string{"gofactos", "blueprint"},
	)

	require.ErrorContains(t, err, "sufficient count of arg file not provided")
	assert.Empty(t, output)
}

// TestCommandSourceLoadFailure reports source-loader errors without writing a
// partial blueprint.
func TestCommandSourceLoadFailure(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.go")
	output, err := executeCommand(t, []string{
		"gofactos", "blueprint", missing,
	})

	require.ErrorContains(t, err, "packages contain errors")
	assert.Empty(t, output)
}

// TestCommandNoFunctions reports a valid source file with no compile target.
func TestCommandNoFunctions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "empty.go")
	require.NoError(t, os.WriteFile(
		path,
		[]byte("package main\n\nconst answer = 42\n"),
		0o600,
	))
	output, err := executeCommand(t, []string{
		"gofactos", "blueprint", path,
	})

	require.EqualError(t, err, "no functions found")
	assert.Empty(t, output)
}

// TestCommandMultipleFunctionsRequiresFilter reports every available root
// when the source needs an explicit --func selection.
func TestCommandMultipleFunctionsRequiresFilter(t *testing.T) {
	t.Parallel()
	output, err := runSourceCommand(t, "loader.go")

	require.EqualError(
		t,
		err,
		"multiple functions found, use --func to select one, "+
			"e.g. [double, triple]",
	)
	assert.Empty(t, output)
}

// TestCommandFuncFilter verifies that the --func flag filters the
// output to only the named function.
func TestCommandFuncFilter(t *testing.T) {
	t.Parallel()
	output, err := runSourceCommand(
		t,
		"loader.go",
		"--json",
		"--func",
		"double",
	)

	require.NoError(t, err)
	var doc factorio.BlueprintWrapper
	require.NoError(t, json.Unmarshal(output, &doc))
	assert.Equal(t, "double", doc.Blueprint.Label)
}

// TestCommandMissingFunc reports the requested function when it is absent.
func TestCommandMissingFunc(t *testing.T) {
	t.Parallel()
	output, err := executeCommand(t, []string{
		"gofactos",
		"blueprint",
		"--func",
		"missing",
		"../testdata/loader.go",
	})

	require.EqualError(t, err, `function "missing" not found`)
	assert.Empty(t, output)
}

// TestCommandFuncPrefersTopLevel resolves a method-name collision to the
// package function explicitly requested by the user.
func TestCommandFuncPrefersTopLevel(t *testing.T) {
	t.Parallel()
	source := writeFunctionCollisionSource(t)
	output, err := executeCommand(t, []string{
		"gofactos", "blueprint", "--json", "--func", "Value", source,
	})

	require.NoError(t, err)
	var doc factorio.BlueprintWrapper
	require.NoError(t, json.Unmarshal(output, &doc))
	assert.Equal(t, "Value", doc.Blueprint.Label)
	require.NotEmpty(t, doc.Blueprint.Entities)
}

// TestCommandImplicitRootIgnoresMethodsAndClosures keeps unsupported members
// from making a single package function appear ambiguous.
func TestCommandImplicitRootIgnoresMethodsAndClosures(t *testing.T) {
	t.Parallel()
	source := writeFunctionCollisionSource(t)
	output, err := executeCommand(t, []string{
		"gofactos", "blueprint", "--json", source,
	})

	require.NoError(t, err)
	var doc factorio.BlueprintWrapper
	require.NoError(t, json.Unmarshal(output, &doc))
	assert.Equal(t, "Value", doc.Blueprint.Label)
}

// TestCommandFuncReportsNameAmbiguity explains when a bare method name cannot
// identify one function.
func TestCommandFuncReportsNameAmbiguity(t *testing.T) {
	t.Parallel()
	source := writeFunctionCollisionSource(t)
	output, err := executeCommand(t, []string{
		"gofactos", "blueprint", "--func", "Clash", source,
	})

	require.EqualError(
		t,
		err,
		`multiple functions named "Clash" found; `+
			`--func cannot disambiguate by name`,
	)
	assert.Empty(t, output)
}

// writeFunctionCollisionSource creates top-level and method name collisions.
func writeFunctionCollisionSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "collision.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main

type first int
type second int

func Value(n int) int { return n + 1 }
func (first) Value() int { return 1 }
func (first) Clash() int { return 1 }
func (second) Clash() int { return 2 }
func (first) WithClosure() int {
	f := func() int { return 3 }
	return f()
}
`), 0o600))
	return path
}

// TestParseSet verifies the --set parser: name=value pairs become a map, and a
// missing '=', empty name, or non-integer value is rejected.
func TestParseSet(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		entries []string
		want    map[string]int
		wantErr string
	}{
		{name: "empty", entries: nil, want: map[string]int{}},
		{name: "single", entries: []string{"n=4"}, want: map[string]int{"n": 4}},
		{name: "multiple", entries: []string{"a=2", "b=10"}, want: map[string]int{"a": 2, "b": 10}},
		{name: "negative value", entries: []string{"x=-3"}, want: map[string]int{"x": -3}},
		{name: "spaces trimmed", entries: []string{" a = 2 "}, want: map[string]int{"a": 2}},
		{name: "missing equals", entries: []string{"a"}, wantErr: "want name=value"},
		{name: "empty name", entries: []string{"=2"}, wantErr: "want name=value"},
		{name: "non-integer", entries: []string{"a=x"}, wantErr: "must be an integer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSet(tc.entries)
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestCommandSetParam verifies --set bakes parameter values, shown on the
// parameter panels.
func TestCommandSetParam(t *testing.T) {
	t.Parallel()
	output, err := runSourceCommand(
		t,
		"add.go",
		"--json",
		"--set",
		"a=2",
		"--set",
		"b=10",
	)

	require.NoError(t, err)
	text := string(output)
	assert.Contains(t, text, `"A = 2"`)
	assert.Contains(t, text, `"B = 10"`)
}

// TestCommandRejectsMalformedSet verifies option parsing through the public
// command path and prevents partial output on failure.
func TestCommandRejectsMalformedSet(t *testing.T) {
	t.Parallel()
	output, err := runSourceCommand(t, "add.go", "--set", "a")

	require.EqualError(t, err, `invalid --set "a": want name=value`)
	assert.Empty(t, output)
}

// TestCommandCompileFailure reports an unsupported source shape without
// writing a partial blueprint.
func TestCommandCompileFailure(t *testing.T) {
	t.Parallel()
	output, err := runSourceCommand(t, "negative/half.go")

	require.ErrorContains(t, err, "unsupported parameter type float64")
	assert.Empty(t, output)
}

// TestCommandClockModes verifies the CLI selects the documented clock period
// and rate label while preserving the default.
func TestCommandClockModes(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	source := filepath.Join(
		root,
		"internal/testdata/recursive/factorial.go",
	)
	for _, tc := range []struct {
		name, label string
		period      int
		fast        bool
	}{
		{name: "default", label: "recursion clock (1 Hz)", period: 60},
		{name: "fast", label: "recursion clock (4 Hz)", period: 15, fast: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := []string{
				"gofactos", "blueprint", "--json",
				"--func", "factorial", "--set", "n=5",
			}
			if tc.fast {
				args = append(args, "--fast")
			}
			args = append(args, source)
			output, runErr := executeCommand(t, args)
			require.NoError(t, runErr)
			assertCommandClock(t, output, tc.label, tc.period)
		})
	}
}

// assertCommandClock proves a CLI clock mode reaches the emitted divider.
func assertCommandClock(
	t *testing.T,
	output []byte,
	label string,
	period int,
) {
	t.Helper()
	var doc factorio.BlueprintWrapper
	require.NoError(t, json.Unmarshal(output, &doc))
	labelCount := 0
	var periods []int
	for _, ent := range doc.Blueprint.Entities {
		if ent.Text == label {
			labelCount++
		}
		behavior := ent.ControlBehavior
		if behavior == nil || behavior.ArithmeticConditions == nil {
			continue
		}
		conditions := behavior.ArithmeticConditions
		if conditions.Operation != "%" ||
			conditions.FirstSignal == nil ||
			conditions.FirstSignal.Name != "signal-dot" ||
			conditions.OutputSignal == nil ||
			conditions.OutputSignal.Name != "signal-dot" {
			continue
		}
		require.NotNil(t, conditions.SecondConstant)
		periods = append(periods, *conditions.SecondConstant)
	}
	require.Equal(t, 1, labelCount)
	require.Equal(t, []int{period}, periods)
}

// TestCommandFastClocklessNoOp proves --fast does not change a blueprint that
// has no runtime clock.
func TestCommandFastClocklessNoOp(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	source := filepath.Join(root, "internal/testdata/add.go")
	base := []string{"gofactos", "blueprint", "--json"}

	defaultOutput, defaultErr := executeCommand(
		t,
		append(append([]string{}, base...), source),
	)
	require.NoError(t, defaultErr)
	fastOutput, fastErr := executeCommand(
		t,
		append(append([]string{}, base...), "--fast", source),
	)
	require.NoError(t, fastErr)
	require.Equal(t, defaultOutput, fastOutput)
}

// TestBlueprintCommandFlags locks the complete public flag inventory.
func TestBlueprintCommandFlags(t *testing.T) {
	t.Parallel()
	var names []string
	for _, flag := range NewCommand().Flags {
		names = append(names, flag.Names()...)
	}

	assert.Equal(t, []string{"fast", "func", "json", "set"}, names)
}
