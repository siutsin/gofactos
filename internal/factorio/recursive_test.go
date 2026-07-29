// This file runs representative recursive programmes through the generic
// runtime.
package factorio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRecursiveCasesSimulate proves checked-in recursive programmes execute
// to their representative Go results in the emitted circuit.
func TestRecursiveCasesSimulate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		path      string
		function  string
		arguments []int
		want      int
	}{
		{
			name: "Fibonacci", path: "../testdata/fibonacci.go",
			function: "fibonacci", arguments: []int{10}, want: 55,
		},
		{
			name: "factorial", path: "../testdata/recursive/factorial.go",
			function: "factorial", arguments: []int{5}, want: 120,
		},
		{
			name: "greatest common divisor",
			path: "../testdata/recursive/gcd.go", function: "gcd",
			arguments: []int{48, 18}, want: 6,
		},
		{
			name: "reaches zero", path: "../testdata/recursive/reacheszero.go",
			function: "reachesZero", arguments: []int{5}, want: 1,
		},
		{
			name:     "starts below zero",
			path:     "../testdata/recursive/reacheszero.go",
			function: "reachesZero", arguments: []int{-1}, want: -1,
		},
		{
			name:     "weighted alternate phi merge",
			path:     "../testdata/recursive/weighted.go",
			function: "weighted", arguments: []int{4, -1, 9}, want: 4,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, tc.path, tc.function)
			program, err := planRecursiveProgram(fn)
			require.NoError(t, err)
			rig := newRecursiveMachineTestRig(t, program, tc.arguments...)
			require.Equal(t, tc.want, rig.run(2_000))
		})
	}
}
