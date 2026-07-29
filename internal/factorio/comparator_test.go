// This file proves recursive comparisons retain Go's branch semantics.
package factorio

import (
	"encoding/json"
	"fmt"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecursiveComparatorsUseCanonicalEncoding verifies the original JSON,
// before Draftsman can normalise comparator aliases.
func TestRecursiveComparatorsUseCanonicalEncoding(t *testing.T) {
	t.Parallel()
	result := generate(
		t,
		"../testdata/fibonacci.go",
		"fibonacci",
		WithParams(map[string]int{"n": 10}),
	)
	raw, err := json.Marshal(result.bp)
	require.NoError(t, err)

	encoded := string(raw)
	assert.NotContains(t, encoded, `"comparator":"!="`)
	assert.NotContains(t, encoded, `"comparator":">="`)
	assert.Contains(t, encoded, `"comparator":"≠"`)
	assert.Contains(t, encoded, `"comparator":"≥"`)
}

// TestRecursiveNotEqualComparison proves both outputs of an ASCII-alias
// comparison remain complementary after canonicalisation.
func TestRecursiveNotEqualComparison(t *testing.T) {
	t.Parallel()
	program := planRecursiveSource(t, `package p
func isZero(n int) bool {
	if n > 0 {
		return isZero(n-1)
	}
	return n != 0
}
`, "isZero")

	for _, tc := range []struct {
		name     string
		argument int
		expected int
	}{
		{name: "equal", argument: 2, expected: -1},
		{name: "not equal", argument: -1, expected: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRecursiveMachineTestRig(t, program, tc.argument)
			require.Equal(t, tc.expected, rig.run(100))
		})
	}
}

// TestRecursiveMachineSwapsConstantLeftComparators proves emitted deciders
// preserve all Go comparisons when their constant operand moves to the right.
func TestRecursiveMachineSwapsConstantLeftComparators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		operator   string
		token      token.Token
		trueValue  int
		falseValue int
	}{
		{
			name: "equal", operator: "==", token: token.EQL,
			trueValue: 5, falseValue: 4,
		},
		{
			name: "not equal", operator: "!=", token: token.NEQ,
			trueValue: 4, falseValue: 5,
		},
		{
			name: "less", operator: "<", token: token.LSS,
			trueValue: 6, falseValue: 5,
		},
		{
			name: "less or equal", operator: "<=", token: token.LEQ,
			trueValue: 5, falseValue: 4,
		},
		{
			name: "greater", operator: ">", token: token.GTR,
			trueValue: 4, falseValue: 5,
		},
		{
			name: "greater or equal", operator: ">=", token: token.GEQ,
			trueValue: 5, falseValue: 6,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			program := planRecursiveSource(t, fmt.Sprintf(`package p
func compare(depth, value int) bool {
	if depth == 0 {
		return 5 %s value
	}
	return compare(depth-1, value)
}
`, test.operator), "compare")

			matched := false
			for _, instruction := range program.instructions {
				if instruction.operator != test.token ||
					!instruction.x.isConstant {
					continue
				}
				matched = true
				require.Equal(t, 5, instruction.x.constant)
				require.False(t, instruction.y.isConstant)
			}
			require.True(t, matched)

			for _, result := range []struct {
				name     string
				value    int
				expected int
			}{
				{name: "true boundary", value: test.trueValue, expected: 1},
				{name: "false boundary", value: test.falseValue, expected: -1},
			} {
				t.Run(result.name, func(t *testing.T) {
					rig := newRecursiveMachineTestRig(t, program, 1, result.value)
					require.Equal(t, result.expected, rig.run(100))
				})
			}
		})
	}
}
