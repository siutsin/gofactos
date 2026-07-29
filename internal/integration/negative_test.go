// This file protects clear CLI failures for unsupported source shapes.
package integration

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/siutsin/gofactos/internal/app"
)

type negativeCase struct {
	name    string
	file    string
	fn      string
	wantErr string
}

// assertNegativeCaseInventory verifies that every negative source file has at
// least one CLI integration rejection case.
func assertNegativeCaseInventory(
	t *testing.T,
	cases []negativeCase,
) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(
		projectRoot(t),
		"internal",
		"testdata",
		"negative",
		"*.go",
	))
	require.NoError(t, err)

	expectedSet := make(map[string]struct{}, len(cases))
	for _, tc := range cases {
		expectedSet[tc.file] = struct{}{}
	}
	expected := make([]string, 0, len(expectedSet))
	for name := range expectedSet {
		expected = append(expected, name)
	}
	actual := make([]string, len(paths))
	for i, path := range paths {
		actual[i] = filepath.Base(path)
	}
	assert.ElementsMatch(t, expected, actual)
}

// TestNegativeCasesRejected drives the complete blueprint command path against
// every case in testdata/negative and asserts the CLI surfaces the
// expected Select error and writes no blueprint. The cases complement the
// unit-level TestGenerateUnsupported.
// ExitErrHandler is stubbed because the command reports failure through
// cli.Exit, which would otherwise os.Exit the test process.
func TestNegativeCasesRejected(t *testing.T) {
	root := projectRoot(t)
	cases := []negativeCase{
		{
			name:    "three-way branch",
			file:    "sign.go",
			wantErr: "more than one branch is unsupported",
		},
		{
			name:    "mutual recursion",
			file:    "mutual.go",
			fn:      "a",
			wantErr: "mutual recursion is unsupported: a, b",
		},
		{
			name:    "recursive helper call",
			file:    "recursivehelper.go",
			fn:      "recursiveHelper",
			wantErr: "call to decrement is unsupported",
		},
		{
			name:    "loop inside recursive function",
			file:    "recursiveloop.go",
			fn:      "recursiveLoop",
			wantErr: "calls involving loops are unsupported: recursiveLoop",
		},
		{
			name: "irreducible loop inside recursive function",
			file: "recursiveirreducible.go",
			fn:   "recursiveIrreducible",
			wantErr: "calls involving loops are unsupported: " +
				"recursiveIrreducible",
		},
		{
			name:    "unsupported recursive operator",
			file:    "recursiveshift.go",
			fn:      "recursiveShift",
			wantErr: "binary operator << is unsupported",
		},
		{
			name:    "non-integer parameter",
			file:    "half.go",
			wantErr: "unsupported parameter type float64",
		},
		{
			name:    "narrow call signature",
			file:    "callnarrow.go",
			fn:      "callNarrow",
			wantErr: "unsupported parameter type int8",
		},
		{
			name:    "non-zero loop accumulator",
			file:    "forinit.go",
			wantErr: "loop accumulator must start at 0",
		},
		{
			name:    "loop returns a constant",
			file:    "loopconst.go",
			wantErr: "loop result must be a loop-carried accumulator",
		},
		{
			name:    "pointer receiver method",
			file:    "methods.go",
			fn:      "Increment",
			wantErr: "methods are unsupported",
		},
		{
			name:    "value receiver method",
			file:    "methods.go",
			fn:      "Value",
			wantErr: "methods are unsupported",
		},
		{
			name:    "closure factory",
			file:    "methods.go",
			fn:      "MakeAdder",
			wantErr: "unsupported result type func(int) int",
		},
		{
			name:    "narrow recurrence state",
			file:    "recurrencenarrow.go",
			wantErr: "unsupported result type int8",
		},
		{
			name:    "unsigned recurrence state",
			file:    "recurrenceunsigned.go",
			wantErr: "unsupported result type uint",
		},
	}
	assertNegativeCaseInventory(t, cases)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			command := app.NewCommand()
			command.Writer = &buf
			command.ExitErrHandler = func(context.Context, *cli.Command, error) {}

			args := []string{
				"gofactos",
				"blueprint",
			}
			if tc.fn != "" {
				args = append(args, "--func", tc.fn)
			}
			args = append(
				args,
				filepath.Join(
					root,
					"internal",
					"testdata",
					"negative",
					tc.file,
				),
			)
			err := command.Run(context.Background(), args)

			require.ErrorContains(t, err, tc.wantErr)
			assert.Empty(
				t,
				buf.Bytes(),
				"a rejected test case must not emit a blueprint",
			)
		})
	}
}
