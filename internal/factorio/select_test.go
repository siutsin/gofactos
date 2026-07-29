// This file protects instruction selection and its supported-language boundary.
package factorio

import (
	"fmt"
	"go/constant"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// TestClassifyScalarLoopResult covers the valid, invalid, and recurrence
// branches without changing existing scalar-loop errors.
func TestClassifyScalarLoopResult(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		path          string
		source        string
		function      string
		wantMatched   bool
		wantIncrement int
		wantErr       string
	}{
		{
			name:          "scalar for",
			path:          "../testdata/fori.go",
			function:      "forI",
			wantMatched:   true,
			wantIncrement: 2,
		},
		{
			name:          "scalar range",
			path:          "../testdata/forrange.go",
			function:      "forRange",
			wantMatched:   true,
			wantIncrement: 2,
		},
		{
			name: "constant first scalar increment",
			source: `package main
func constantFirst(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result = 2 + result
	}
	return result
}
`,
			function:      "constantFirst",
			wantMatched:   true,
			wantIncrement: 2,
		},
		{
			name: "zero scalar increment",
			source: `package main
func zeroIncrement(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result = result + 0
	}
	return result
}
`,
			function:      "zeroIncrement",
			wantMatched:   true,
			wantIncrement: 0,
		},
		{
			name: "negative scalar increment",
			source: `package main
func negativeIncrement(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result += -2
	}
	return result
}
`,
			function:      "negativeIncrement",
			wantMatched:   true,
			wantIncrement: -2,
		},
		{
			name:        "non-zero scalar initial",
			path:        "../testdata/negative/forinit.go",
			function:    "forInit",
			wantMatched: true,
			wantErr:     "loop accumulator must start at 0",
		},
		{
			name:        "constant scalar result",
			path:        "../testdata/negative/loopconst.go",
			function:    "loopConst",
			wantMatched: true,
			wantErr:     "loop result must be a loop-carried accumulator",
		},
		{
			name:        "Fibonacci recurrence",
			path:        "../testdata/fib.go",
			function:    "fib",
			wantMatched: false,
		},
		{
			name: "recurrence returning updated state",
			source: `package main
func currentResult(n int) int {
	a, b := 0, 1
	for i := 0; i < n; i++ {
		a, b = b, a+b
	}
	return b
}
`,
			function:    "currentResult",
			wantMatched: false,
		},
		{
			name: "parameter scalar increment",
			source: `package main
func parameterIncrement(n, step int) int {
	result := 0
	for i := 0; i < n; i++ {
		result += step
	}
	return result
}
`,
			function:    "parameterIncrement",
			wantMatched: true,
			wantErr:     "only `result += constant` is supported",
		},
		{
			name: "unrelated counter plus constant",
			source: `package main
func unrelatedCounter(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result = i + 2
	}
	return result
}
`,
			function:    "unrelatedCounter",
			wantMatched: true,
			wantErr:     "only `result += constant` is supported",
		},
		{
			name: "unrelated constant plus counter",
			source: `package main
func unrelatedCounter(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result = 2 + i
	}
	return result
}
`,
			function:    "unrelatedCounter",
			wantMatched: true,
			wantErr:     "only `result += constant` is supported",
		},
		{
			name: "unrelated pre-loop phi plus constant",
			source: `package main
func unrelatedPhi(n, choose int) int {
	other := 1
	if choose > 0 {
		other = 3
	}
	result := 0
	for i := 0; i < n; i++ {
		result = other + 2
	}
	return result
}
`,
			function:    "unrelatedPhi",
			wantMatched: true,
			wantErr:     "only `result += constant` is supported",
		},
		{
			name: "non-constant scalar initial",
			source: `package main
func nonConstantInitial(n, start int) int {
	result := start
	for i := 0; i < n; i++ {
		result += 2
	}
	return result
}
`,
			function:    "nonConstantInitial",
			wantMatched: true,
			wantErr:     "loop accumulator must start at 0",
		},
		{
			name: "computed scalar result",
			source: `package main
func computedResult(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result += 2
	}
	return result + n
}
`,
			function:    "computedResult",
			wantMatched: true,
			wantErr:     "loop result must be a loop-carried accumulator",
		},
		{
			name:          "counter result",
			path:          "../testdata/forcounter.go",
			function:      "forCounter",
			wantMatched:   true,
			wantIncrement: 1,
		},
		{
			name: "constant zero result",
			source: `package main
func zeroResult(n int) int {
	for i := 0; i < n; i++ {
	}
	return 0
}
`,
			function:      "zeroResult",
			wantMatched:   true,
			wantIncrement: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseLoopTestFunction(
				t,
				tc.path,
				tc.source,
				tc.function,
			)
			header, _, _, counter := testLoopParts(t, fn)

			got, err := classifyScalarLoopResult(
				fn,
				header,
				counter,
			)

			require.Equal(t, tc.wantMatched, got.matched)
			require.Equal(t, tc.wantIncrement, got.increment)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

// TestSelectRejectsUnsupportedScalarLoopControlFlow protects fixed-count
// hardware from source CFGs whose iteration count it cannot represent.
func TestSelectRejectsUnsupportedScalarLoopControlFlow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source, function string
	}{
		{
			name: "conditional exit",
			source: `package main
func conditionalBreak(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		if i == 2 {
			break
		}
		result += 2
	}
	return result
}
`,
			function: "conditionalBreak",
		},
		{
			name: "reversed guard",
			source: `package main
func reversedGuard(n int) int {
	result, i := 0, 0
	for {
		if i < n {
			break
		}
		result += 2
		i++
	}
	return result
}
`,
			function: "reversedGuard",
		},
		{
			name: "shifted guard",
			source: `package main
func shiftedGuard(n int) int {
	result := 0
	for i := 0; i+1 < n; i++ {
		result += 2
	}
	return result
}
`,
			function: "shiftedGuard",
		},
		{
			name: "conditional entry",
			source: `package main
func conditionalEntry(n int, run bool) int {
	result, i := 0, 0
	if run {
		goto loop
	}
	goto done
loop:
	result += 2
	i++
	if i < n {
		goto loop
	}
done:
	return result
}
`,
			function: "conditionalEntry",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseLoopTestFunction(
				t,
				"",
				tc.source,
				tc.function,
			)

			_, err := selectFunc(fn)

			require.ErrorContains(
				t,
				err,
				"unsupported scalar loop control flow",
			)
		})
	}
}

// TestSelectAcceptsConstantIntegerRange preserves the rotated range CFG when
// the bound is an integer constant rather than a parameter.
func TestSelectAcceptsConstantIntegerRange(t *testing.T) {
	t.Parallel()
	fn := parseLoopTestFunction(t, "", `package main
func constantRange() int {
	result := 0
	for range 3 {
		result += 2
	}
	return result
}
`, "constantRange")

	_, err := selectFunc(fn)

	require.NoError(t, err)
}

// TestSelectAcceptsPhiThenBranch proves a phi merge can precede an independent
// two-return branch.
func TestSelectAcceptsPhiThenBranch(t *testing.T) {
	t.Parallel()
	fn := parseLoopTestFunction(t, "", `package main
func mixed(n int) int {
	x := n
	if n < 0 {
		x = -n
	}
	if x > 10 {
		return x
	}
	return x + 1
}
`, "mixed")

	_, err := selectFunc(fn)

	require.NoError(t, err)
}

// TestCompareOpMarksBooleanOperand proves instruction selection records whether
// a constant comparison is Boolean, so the panel label can tell `n == 1` from
// `flag == true` even though both bake the same 1 sentinel.
func TestCompareOpMarksBooleanOperand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		source      string
		function    string
		wantBoolean bool
	}{
		{
			name: "integer equality",
			source: `package main
func eqOne(n int) bool { return n == 1 }
`,
			function:    "eqOne",
			wantBoolean: false,
		},
		{
			name: "boolean equality",
			source: `package main
func eqTrue(flag bool) bool { return flag == true }
`,
			function:    "eqTrue",
			wantBoolean: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fn := parseLoopTestFunction(t, "", tc.source, tc.function)

			sel, err := selectFunc(fn)

			require.NoError(t, err)
			var compares []*compare
			for _, in := range sel.insts {
				if c, ok := in.comp.(*compare); ok {
					compares = append(compares, c)
				}
			}
			require.Len(t, compares, 1)
			require.Equal(t, tc.wantBoolean, compares[0].boolean)
		})
	}
}

// TestSelectRejectsReversedCounterPhiEdges binds zero to entry and the unit
// step to the back edge rather than accepting them in either order.
func TestSelectRejectsReversedCounterPhiEdges(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, "../testdata/fori.go", "forI")
	header, _, _, counter := testLoopParts(t, fn)
	entry, back := scalarPhiEdgeIndexes(t, header)
	counter.Edges[entry], counter.Edges[back] =
		counter.Edges[back], counter.Edges[entry]

	_, err := selectFunc(fn)

	require.ErrorContains(t, err, "unsupported scalar loop control flow")
}

// TestSelectRejectsScalarLoopSideEffects proves synthesised loop hardware
// cannot silently discard an observable operation after the loop.
func TestSelectRejectsScalarLoopSideEffects(t *testing.T) {
	t.Parallel()
	fn := parseLoopTestFunction(t, "", `package main
var sink int
func storeResult(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result += 2
	}
	sink = result
	return result
}
`, "storeResult")

	_, err := selectFunc(fn)

	require.EqualError(
		t,
		err,
		"select: scalar loop instruction *ssa.Store is unsupported",
	)
}

// TestSelectRejectsUnsupportedScalarLoopOperator proves synthesised loop
// hardware cannot silently discard a reachable unsupported computation.
func TestSelectRejectsUnsupportedScalarLoopOperator(t *testing.T) {
	t.Parallel()
	fn := parseLoopTestFunction(t, "", `package main
func shiftedLoop(n int) int {
	for i := 0; i < n; i++ {
		_ = i << 1
	}
	return 0
}
`, "shiftedLoop")

	_, err := selectFunc(fn)

	require.EqualError(t, err, "select: unsupported operator <<")
}

// TestSelectRejectsIrreducibleCycle proves a two-entry cycle cannot fall
// through to acyclic phi lowering.
func TestSelectRejectsIrreducibleCycle(t *testing.T) {
	t.Parallel()
	fn := parseLoopTestFunction(t, "", `package main
func irreducible(n int) int {
	result := 0
	if n > 0 {
		result = 1
	}
	if n == 0 {
		goto left
	}
	goto right
left:
	if n < 0 {
		return result
	}
	goto right
right:
	if n > 1 {
		return result
	}
	goto left
}
`, "irreducible")

	require.Nil(t, loopHeader(fn))
	require.True(t, hasControlFlowCycle(fn))
	_, err := selectFunc(fn)
	require.EqualError(
		t,
		err,
		"select: irreducible control-flow cycle is unsupported",
	)
}

// TestClassifyScalarLoopResultUsesPredecessors proves entry and back edges
// follow predecessor slots for both header-return and exit-phi scalar loops.
func TestClassifyScalarLoopResultUsesPredecessors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path, function string
	}{
		{
			name:     "header phi result",
			path:     "../testdata/fori.go",
			function: "forI",
		},
		{
			name:     "exit phi result",
			path:     "../testdata/forrange.go",
			function: "forRange",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, tc.path, tc.function)
			header, _, _, counter := testLoopParts(t, fn)
			header.Preds[0], header.Preds[1] =
				header.Preds[1], header.Preds[0]
			for _, instr := range header.Instrs {
				phi, ok := instr.(*ssa.Phi)
				if ok {
					phi.Edges[0], phi.Edges[1] =
						phi.Edges[1], phi.Edges[0]
				}
			}

			got, err := classifyScalarLoopResult(
				fn,
				header,
				counter,
			)
			require.NoError(t, err)
			require.True(t, got.matched)
			require.Equal(t, 2, got.increment)
		})
	}
}

// TestClassifyScalarLoopResultMapsExitPhiPredecessors requires forRange's exit
// values to stay paired with their entry and back-edge predecessors.
func TestClassifyScalarLoopResultMapsExitPhiPredecessors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		swapPreds bool
		wantErr   string
	}{
		{name: "paired predecessor and edge reorder", swapPreds: true},
		{
			name: "edge-only reorder",
			wantErr: "select: loop result must be a loop-carried " +
				"accumulator or constant 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(
				t,
				"../testdata/forrange.go",
				"forRange",
			)
			header, _, _, counter := testLoopParts(t, fn)
			value, err := loopReturnValue(fn)
			require.NoError(t, err)
			result, ok := value.(*ssa.Phi)
			require.True(t, ok)
			require.Len(t, result.Block().Preds, 2)
			require.Len(t, result.Edges, 2)
			result.Edges[0], result.Edges[1] =
				result.Edges[1], result.Edges[0]
			if tc.swapPreds {
				result.Block().Preds[0], result.Block().Preds[1] =
					result.Block().Preds[1], result.Block().Preds[0]
			}

			got, err := classifyScalarLoopResult(
				fn,
				header,
				counter,
			)
			require.True(t, got.matched)
			if tc.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, 2, got.increment)
				return
			}
			require.EqualError(t, err, tc.wantErr)
		})
	}
}

// TestClassifyScalarLoopResultRejectsMalformedConstants pins integer type and
// non-nil value checks at the scalar classifier boundary.
func TestClassifyScalarLoopResultRejectsMalformedConstants(
	t *testing.T,
) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		target  string
		value   *ssa.Const
		wantErr string
	}{
		{
			name:   "bool initial",
			target: "initial",
			value: ssa.NewConst(
				constant.MakeBool(false),
				types.Typ[types.Bool],
			),
			wantErr: "select: loop accumulator must start at 0",
		},
		{
			name:    "nil initial",
			target:  "initial",
			value:   ssa.NewConst(nil, types.Typ[types.Invalid]),
			wantErr: "select: loop accumulator must start at 0",
		},
		{
			name:   "float initial",
			target: "initial",
			value: ssa.NewConst(
				constant.MakeFloat64(0),
				types.Typ[types.Float64],
			),
			wantErr: "select: loop accumulator must start at 0",
		},
		{
			name:   "bool increment",
			target: "increment",
			value: ssa.NewConst(
				constant.MakeBool(true),
				types.Typ[types.Bool],
			),
			wantErr: "select: unsupported loop body, only " +
				"`result += constant` is supported",
		},
		{
			name:   "nil increment",
			target: "increment",
			value:  ssa.NewConst(nil, types.Typ[types.Invalid]),
			wantErr: "select: unsupported loop body, only " +
				"`result += constant` is supported",
		},
		{
			name:   "float increment",
			target: "increment",
			value: ssa.NewConst(
				constant.MakeFloat64(2),
				types.Typ[types.Float64],
			),
			wantErr: "select: unsupported loop body, only " +
				"`result += constant` is supported",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, "../testdata/fori.go", "forI")
			header, _, _, counter := testLoopParts(t, fn)
			value, err := loopReturnValue(fn)
			require.NoError(t, err)
			state, ok := value.(*ssa.Phi)
			require.True(t, ok)
			entryIndex, backIndex := scalarPhiEdgeIndexes(
				t,
				header,
			)
			if tc.target == "initial" {
				state.Edges[entryIndex] = tc.value
			} else {
				update, ok := state.Edges[backIndex].(*ssa.BinOp)
				require.True(t, ok)
				if update.X == state {
					update.Y = tc.value
				} else {
					update.X = tc.value
				}
			}

			got, err := classifyScalarLoopResult(
				fn,
				header,
				counter,
			)
			require.True(t, got.matched)
			require.EqualError(t, err, tc.wantErr)
		})
	}
}

// scalarPhiEdgeIndexes distinguishes entry and feedback values without relying
// on predecessor ordering.
func scalarPhiEdgeIndexes(
	t *testing.T,
	header *ssa.BasicBlock,
) (entry, back int) {
	t.Helper()
	entry, back = -1, -1
	for i, pred := range header.Preds {
		if header.Dominates(pred) {
			back = i
		} else {
			entry = i
		}
	}
	require.NotEqual(t, -1, entry)
	require.NotEqual(t, -1, back)
	return entry, back
}

// TestSelectRejectsUnsupportedIntegerSignatures proves non-int widths fail at
// the shared signature gate before lowering can lose their Go semantics.
func TestSelectRejectsUnsupportedIntegerSignatures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path, function, wantType string
	}{
		{
			name:     "narrow",
			path:     "../testdata/negative/recurrencenarrow.go",
			function: "recurrenceNarrow",
			wantType: "int8",
		},
		{
			name:     "unsigned",
			path:     "../testdata/negative/recurrenceunsigned.go",
			function: "recurrenceUnsigned",
			wantType: "uint",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, tc.path, tc.function)
			_, err := selectFunc(fn)
			require.EqualError(t, err,
				"select: unsupported result type "+tc.wantType)
		})
	}
}

// TestGenerateEnforcesRecurrenceLimits distinguishes the allocatable 7-step
// chain from the theoretical 58-tick settling ceiling.
func TestGenerateEnforcesRecurrenceLimits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		depth   int
		opts    []Option
		wantErr string
	}{
		{name: "allocatable", depth: 7},
		{
			name:  "signal bank exhausted",
			depth: 8,
			wantErr: "allocate deep: intermediate signal bank exhausted: " +
				"23 nets, 21 signals",
		},
		{
			name:  "over timing ceiling",
			depth: 59,
			wantErr: "select: recurrence body addition depth 59 " +
				"exceeds 58-tick settling budget",
		},
		{
			name:  "fast timing ceiling",
			depth: 14,
			opts:  []Option{WithFastClock()},
			wantErr: "select: recurrence body addition depth 14 " +
				"exceeds 13-tick settling budget",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseLoopTestFunction(
				t,
				"",
				recurrenceChainSource(tc.depth),
				"deep",
			)
			_, err := compileFunction(fn, tc.opts...)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// recurrenceChainSource creates arbitrary dependency depth so the selected
// clock's safety ceiling remains testable.
func recurrenceChainSource(depth int) string {
	var source strings.Builder
	source.WriteString(`package main
func deep(n int) int {
	a, b := 0, 1
	for i := 0; i < n; i++ {
		next := a + b
`)
	for range depth - 1 {
		source.WriteString("\t\tnext = next + 1\n")
	}
	source.WriteString(`		a, b = b, next
	}
	return a
}
`)
	return source.String()
}

// TestLoopPhiEdgesUsesPredecessors proves edge roles follow predecessor slots,
// not the incidental order in which the SSA builder emitted them.
func TestLoopPhiEdgesUsesPredecessors(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, "../testdata/fib.go", "fib")
	header := loopHeader(fn)
	require.NotNil(t, header)
	require.Len(t, header.Preds, 2)
	header.Preds[0], header.Preds[1] = header.Preds[1], header.Preds[0]
	for _, instr := range header.Instrs {
		phi, ok := instr.(*ssa.Phi)
		if !ok {
			continue
		}
		phi.Edges[0], phi.Edges[1] = phi.Edges[1], phi.Edges[0]
	}
	_, cmp, bound, counter := testLoopParts(t, fn)

	got, err := analyseRecurrenceLoop(
		fn,
		header,
		cmp,
		bound,
		counter,
	)

	require.NoError(t, err)
	require.Equal(t, []int{0, 1, 0}, stateInitials(got))
	require.Equal(t, []string{"t1", "t4", "t5"}, stateNextNames(got))
}

// TestFeasiblePhiEdgesDistinguishesDuplicateSlots proves constant branches use
// edge occurrence, not only block identity, when both CFG slots share blocks.
func TestFeasiblePhiEdgesDistinguishesDuplicateSlots(t *testing.T) {
	t.Parallel()
	for _, condition := range []bool{false, true} {
		t.Run(fmt.Sprintf("condition %t", condition), func(t *testing.T) {
			assertFeasiblePhiDuplicateSlot(t, condition)
		})
	}
}

// assertFeasiblePhiDuplicateSlot mutates one branch into duplicate CFG edges.
func assertFeasiblePhiDuplicateSlot(t *testing.T, condition bool) {
	t.Helper()
	fn := parseTestFile(t, writeTempGo(t, `package main
func choose(a bool) int {
	x := 1
	if a { x = 2 }
	return x
}
`), "choose")
	branch := fn.Blocks[0]
	ifi, ok := lastIf(branch)
	require.True(t, ok)
	var phi *ssa.Phi
	for _, block := range fn.Blocks {
		for _, instruction := range block.Instrs {
			candidate, isPhi := instruction.(*ssa.Phi)
			if isPhi {
				phi = candidate
			}
		}
	}
	require.NotNil(t, phi)
	merge := phi.Block()
	require.Len(t, phi.Edges, 2)
	branch.Succs = []*ssa.BasicBlock{merge, merge}
	merge.Preds = []*ssa.BasicBlock{branch, branch}
	ifi.Cond = ssa.NewConst(
		constant.MakeBool(condition),
		types.Typ[types.Bool],
	)

	wantIndex := 1
	if condition {
		wantIndex = 0
	}
	edges := feasiblePhiEdges(phi)
	require.Len(t, edges, 1)
	require.Equal(t, phi.Edges[wantIndex], edges[0].value)
	require.True(t, feasiblePredecessorEdge(merge, wantIndex))
	require.False(t, feasiblePredecessorEdge(merge, 1-wantIndex))
}

// TestSelectFuncFiltersCompileTimeDeadControlFlow proves x/tools SSA blocks on
// an impossible constant branch do not expand the supported language boundary.
func TestSelectFuncFiltersCompileTimeDeadControlFlow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		source    string
		function  string
		wantErr   string
		wantNoPhi bool
	}{
		{
			name: "dead return",
			source: `package main
func choose(n int) int {
	if true { return n }
	return n << 1
}
`,
			function: "choose",
		},
		{
			name: "single feasible phi edge",
			source: `package main
func choose(n int) int {
	result := n
	if false { result = n << 1 }
	return result
}
`,
			function:  "choose",
			wantNoPhi: true,
		},
		{
			name: "nested constant return",
			source: `package main
func choose(n int, first bool) int {
	if first {
		if true { return n }
	}
	return n + 1
}
`,
			function: "choose",
		},
		{
			name: "dead static call",
			source: `package main
func unsupported(n int) int { return n << 1 }
func choose(n int) int {
	if false { return unsupported(n) }
	return n
}
`,
			function: "choose",
		},
		{
			name: "dead loop",
			source: `package main
func choose(n int) int {
	if false {
		for i := 0; i < n; i++ { n = n << 1 }
	}
	return n
}
`,
			function: "choose",
		},
		{
			name: "callee dead return",
			source: `package main
func helper(n int) int {
	if true { return n }
	return n << 1
}
func choose(n int) int { return helper(n) }
`,
			function: "choose",
		},
		{
			name: "recursive dead return",
			source: `package main
func choose(n int) int {
	if false { return n << 1 }
	if n <= 0 { return 0 }
	return choose(n - 1)
}
`,
			function: "choose",
		},
		{
			name: "runtime reachable shift",
			source: `package main
func choose(n int, first bool) int {
	if first { return n }
	return n << 1
}
`,
			function: "choose",
			wantErr:  "unsupported operator <<",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, writeTempGo(t, tc.source), tc.function)
			selected, err := selectFunc(fn)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.wantNoPhi {
				for _, instance := range selected.insts {
					require.NotEqual(t, "phi", instance.comp.kind())
				}
			}
		})
	}
}

// TestCompileUsesFeasiblePhiControlFlow proves feasible dominators and block
// order preserve runtime phi selection through the full pipeline.
func TestCompileUsesFeasiblePhiControlFlow(t *testing.T) {
	t.Parallel()
	const runtimePhi = `package main
func choose(a bool) int {
	x := 0
	if true {
		if a { x = 1 }
	}
	return x
}
`
	const calleePhi = `package main
func helper(a bool) int {
	x := 0
	if true {
		if a { x = 1 }
	}
	return x
}
func choose(a bool) int { return helper(a) }
`
	for _, tc := range []struct {
		name   string
		source string
		params []int
		want   int
	}{
		{name: "runtime false", source: runtimePhi, params: []int{0}, want: 0},
		{name: "runtime true", source: runtimePhi, params: []int{1}, want: 1},
		{name: "callee false", source: calleePhi, params: []int{0}, want: 0},
		{name: "callee true", source: calleePhi, params: []int{1}, want: 1},
		{
			name: "computed live edge",
			source: `package main
func choose(n int) int {
	x := 0
	if true {
		if true { x = n + 1 }
	}
	return x
}
`,
			params: []int{4},
			want:   5,
		},
		{
			name: "indirect runtime false arm",
			source: `package main
func choose(n int, a bool) int {
	x := 0
	if a {
		if true { x = n + 1 }
	}
	return x
}
`,
			params: []int{4, 0},
			want:   0,
		},
		{
			name: "indirect runtime true arm",
			source: `package main
func choose(n int, a bool) int {
	x := 0
	if a {
		if true { x = n + 1 }
	}
	return x
}
`,
			params: []int{4, 1},
			want:   5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempGo(t, tc.source)
			require.Equal(
				t,
				tc.want,
				simulateParams(t, path, "choose", tc.params...),
			)
		})
	}
}

// TestCompileLoopsIgnoreConstantDeadControlFlow proves supported loop lowering
// ignores impossible prefix and body arms through the full pipeline.
func TestCompileLoopsIgnoreConstantDeadControlFlow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		source string
		n      int
		want   int
	}{
		{
			name: "scalar prefix",
			source: `package main
func choose(n int) int {
	if false { return n << 1 }
	result := 0
	for i := 0; i < n; i++ { result += 2 }
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "scalar bound alias",
			source: `package main
func choose(n int) int {
	bound := n
	if false { bound = n << 1 }
	result := 0
	for i := 0; i < bound; i++ { result += 2 }
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "scalar nested prefix aliases",
			source: `package main
func choose(n int) int {
	result := 0
	if false { result = n << 1 }
	if false { return result << 1 }
	for i := 0; i < n; i++ { result += 2 }
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "scalar dead bound mutation",
			source: `package main
func choose(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		if false { n-- }
		result += 2
	}
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "range prefix",
			source: `package main
func choose(n int) int {
	if false { return n << 1 }
	result := 0
	for range n { result += 2 }
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "range bound alias",
			source: `package main
func choose(n int) int {
	bound := n
	if false { bound = n << 1 }
	result := 0
	for range bound { result += 2 }
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "range alias before update",
			source: `package main
func choose(n int) int {
	result := 0
	for range n {
		if false { result = result << 1 }
		result += 2
	}
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "range dead break",
			source: `package main
func choose(n int) int {
	result := 0
	for range n {
		if false {
			result = result << 1
			break
		}
		result += 2
	}
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "range dead continue",
			source: `package main
func choose(n int) int {
	result := 0
	for range n {
		if false {
			result = result << 1
			continue
		}
		result += 2
	}
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "scalar body",
			source: `package main
func choose(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result += 2
		if false { result = result << 1 }
	}
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "scalar alias before update",
			source: `package main
func choose(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		if false { result = result << 1 }
		result += 2
	}
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "scalar dead break",
			source: `package main
func choose(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		if false { break }
		result += 2
	}
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "scalar dead continue",
			source: `package main
func choose(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		if false {
			result = result << 1
			continue
		}
		result += 2
	}
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "scalar dead state",
			source: `package main
func choose(n int) int {
	result, dead := 0, 0
	for i := 0; i < n; i++ {
		if false { dead++ }
		result += 2
	}
	_ = dead
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "scalar returned dead state",
			source: `package main
func choose(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		if false { result = n << 1 }
	}
	return result
}
`,
			n: 5, want: 0,
		},
		{
			name: "scalar exit alias",
			source: `package main
func choose(n int) int {
	result := 0
	for i := 0; i < n; i++ { result += 2 }
	if false { result = result << 1 }
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "range returned dead state",
			source: `package main
func choose(n int) int {
	result := 0
	for range n {
		if false { result = n << 1 }
	}
	return result
}
`,
			n: 5, want: 0,
		},
		{
			name: "range exit alias",
			source: `package main
func choose(n int) int {
	result := 0
	for range n { result += 2 }
	if false { result = result << 1 }
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "recurrence prefix",
			source: `package main
func choose(n int) int {
	if false { return n << 1 }
	previous, current := 0, 1
	for i := 0; i < n; i++ {
		previous, current = current, previous + current
	}
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence prefix alias",
			source: `package main
func choose(n int) int {
	previous, current := 0, 1
	if false { previous = n << 1 }
	for i := 0; i < n; i++ {
		previous, current = current, previous + current
	}
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence bound alias",
			source: `package main
func choose(n int) int {
	bound := n
	if false { bound = n << 1 }
	previous, current := 0, 1
	for i := 0; i < bound; i++ {
		previous, current = current, previous + current
	}
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence nested prefix aliases",
			source: `package main
func choose(n int) int {
	previous, current := 0, 1
	if false { previous = n << 1 }
	if false { return previous << 1 }
	for i := 0; i < n; i++ {
		previous, current = current, previous + current
	}
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence dead bound mutation",
			source: `package main
func choose(n int) int {
	previous, current := 0, 1
	for i := 0; i < n; i++ {
		if false { n-- }
		previous, current = current, previous + current
	}
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence body",
			source: `package main
func choose(n int) int {
	previous, current := 0, 1
	for i := 0; i < n; i++ {
		previous, current = current, previous + current
		if false { previous = previous << 1 }
	}
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence alias before update",
			source: `package main
func choose(n int) int {
	previous, current := 0, 1
	for i := 0; i < n; i++ {
		if false { previous = previous << 1 }
		previous, current = current, previous + current
	}
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence dead break",
			source: `package main
func choose(n int) int {
	previous, current := 0, 1
	for i := 0; i < n; i++ {
		if false { break }
		previous, current = current, previous + current
	}
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence dead continue",
			source: `package main
func choose(n int) int {
	previous, current := 0, 1
	for i := 0; i < n; i++ {
		if false {
			previous = previous << 1
			continue
		}
		previous, current = current, previous + current
	}
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence dead state",
			source: `package main
func choose(n int) int {
	previous, current, dead := 0, 1, 0
	for i := 0; i < n; i++ {
		if false { dead++ }
		previous, current = current, previous + current
	}
	_ = dead
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence exit alias",
			source: `package main
func choose(n int) int {
	previous, current := 0, 1
	for i := 0; i < n; i++ {
		previous, current = current, previous + current
	}
	if false { previous = previous << 1 }
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence dead Boolean state",
			source: `package main
func choose(n int) int {
	previous, current, dead := 0, 1, false
	for i := 0; i < n; i++ {
		if false { dead = true }
		previous, current = current, previous + current
	}
	_ = dead
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence dead parameter state",
			source: `package main
func choose(n int) int {
	previous, current, dead := 0, 1, n
	for i := 0; i < n; i++ {
		if false { dead++ }
		previous, current = current, previous + current
	}
	_ = dead
	return previous
}
`,
			n: 5, want: 5,
		},
		{
			name: "recurrence identity dependency",
			source: `package main
func choose(n int) int {
	result, step := 0, 2
	for i := 0; i < n; i++ {
		if false { step++ }
		result += step
	}
	return result
}
`,
			n: 5, want: 10,
		},
		{
			name: "recurrence reachable parameter alias",
			source: `package main
func choose(step int) int {
	previous, current := 0, 1
	for i := 0; i < 5; i++ {
		if false { step++ }
		previous, current = current, previous + current + step
	}
	return previous
}
`,
			n: 5, want: 40,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempGo(t, tc.source)
			require.Equal(
				t,
				tc.want,
				simulateLoopSettles(t, path, "choose", tc.n),
			)
		})
	}
}

// TestSelectFuncErrorPaths proves unsupported SSA shapes fail in Select with
// clear errors before the later pipeline phases can emit a misleading blueprint.
func TestSelectFuncErrorPaths(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name: "float parameter",
			source: `package main
func bad(x float64) int { return 0 }
`,
			wantErr: "unsupported parameter type float64",
		},
		{
			name: "struct result",
			source: `package main
type pair struct{ x int }
func bad() pair { return pair{} }
`,
			wantErr: "unsupported result type",
		},
		{
			name: "pointer parameter",
			source: `package main
func bad(x *int) int { return 0 }
`,
			wantErr: "unsupported parameter type *int",
		},
		{
			name: "multiple loops",
			source: `package main
func bad(n int) int {
	x := 0
	for i := 0; i < n; i++ { x++ }
	for j := 0; j < n; j++ { x++ }
	return x
}
`,
			wantErr: "more than one loop is unsupported",
		},
		{
			name: "negative constant loop bound",
			source: `package main
func bad() int {
	x := 0
	for i := 0; i < -1; i++ { x++ }
	return x
}
`,
			wantErr: "loop bound must be non-negative",
		},
		{
			name: "bitwise operator",
			source: `package main
func bad(a, b int) int { return a & b }
`,
			wantErr: "unsupported operator &",
		},
		{
			name: "shift operator",
			source: `package main
func bad(a int) int { return a << 1 }
`,
			wantErr: "unsupported operator <<",
		},
		{
			name: "inclusive loop comparator",
			source: `package main
func bad(n int) int {
	x := 0
	for i := 0; i <= n; i++ {
		x++
	}
	return x
}
`,
			wantErr: "unsupported loop comparator",
		},
		{
			name: "reversed loop operands",
			source: `package main
func bad(n int) int {
	x := 0
	for i := 0; n < i; i++ {
		x++
	}
	return x
}
`,
			wantErr: "loop condition must be counter < bound",
		},
		{
			name: "early loop return",
			source: `package main
func bad(n int) int {
	x := 0
	for i := 0; i < n; i++ {
		if i == 2 {
			return x
		}
		x++
	}
	return x
}
`,
			wantErr: "loop must have exactly one single-value return",
		},
		{
			name: "non-unit loop step",
			source: `package main
func bad(n int) int {
	x := 0
	for i := 0; i < n; i += 2 {
		x++
	}
	return x
}
`,
			wantErr: "step by 1",
		},
		{
			name: "non-zero loop start",
			source: `package main
func bad(n int) int {
	x := 0
	for i := 1; i < n; i++ {
		x++
	}
	return x
}
`,
			wantErr: "start at 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, writeTempGo(t, tc.source), "bad")
			_, err := selectFunc(fn)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// TestSelectRejectsComputedScalarLoopBound proves fixed-count scalar hardware
// accepts only parameter or constant bounds.
func TestSelectRejectsComputedScalarLoopBound(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, writeTempGo(t, `package main
func bad(n int) int {
	bound := n + 1
	result := 0
	for i := 0; i < bound; i++ {
		result += 2
	}
	return result
}
`), "bad")

	_, err := selectFunc(fn)

	require.EqualError(
		t,
		err,
		"select: scalar loop bound must be an integer parameter or constant",
	)
}

// TestSelectRejectsOutOfRangeConstants prevents values that Factorio cannot
// represent as signed 32-bit circuit constants from reaching Emit.
func TestSelectRejectsOutOfRangeConstants(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, value string
	}{
		{name: "above maximum", value: "2147483648"},
		{name: "below minimum", value: "-2147483649"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "package main\nfunc bad() int { return " +
				tc.value + " }\n"
			fn := parseTestFile(t, writeTempGo(t, source), "bad")
			_, err := selectFunc(fn)
			require.ErrorContains(
				t,
				err,
				"outside Factorio signed 32-bit range",
			)
		})
	}
}

// TestSelectorUseRejectsNilValue proves malformed SSA input returns an error
// before label generation can dereference it.
func TestSelectorUseRejectsNilValue(t *testing.T) {
	t.Parallel()
	s := &selector{selectorFrame: selectorFrame{
		producer: map[ssa.Value]*port{},
	}}

	err := s.use(nil, &port{})

	require.ErrorContains(t, err, "nil value has no producer")
}

// TestValidateLoopBoundAllowsZero proves constant and runtime zero bounds share
// zero-iteration semantics while negative constants remain unsupported.
func TestValidateLoopBoundAllowsZero(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		bound   int64
		wantErr bool
	}{
		{name: "negative", bound: -1, wantErr: true},
		{name: "zero", bound: 0, wantErr: false},
		{name: "positive", bound: 3, wantErr: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := ssa.NewConst(constant.MakeInt64(tc.bound), types.Typ[types.Int])
			err := validateLoopBound(c)
			if tc.wantErr {
				require.ErrorContains(t, err, "loop bound must be non-negative")
			} else {
				require.NoError(t, err)
			}
		})
	}

	for _, tc := range []struct {
		name, source, function string
	}{
		{
			name: "scalar",
			source: `package main
func zeroBound() int {
	result := 0
	for i := 0; i < 0; i++ {
		result += 2
	}
	return result
}
`,
			function: "zeroBound",
		},
		{
			name: "recurrence",
			source: `package main
func zeroRecurrence() int {
	a, b := 0, 1
	for i := 0; i < 0; i++ {
		a, b = b, a+b
	}
	return a
}
`,
			function: "zeroRecurrence",
		},
	} {
		t.Run("select "+tc.name, func(t *testing.T) {
			fn := parseLoopTestFunction(
				t,
				"",
				tc.source,
				tc.function,
			)
			_, err := selectFunc(fn)
			require.NoError(t, err)
		})
	}
}

// parseLoopTestFunction lets loop tests share checked-in and inline test cases.
func parseLoopTestFunction(
	t *testing.T,
	path, source, function string,
) *ssa.Function {
	t.Helper()
	if path == "" {
		path = writeTempGo(t, source)
	}
	return parseTestFile(t, path, function)
}

// testLoopParts extracts the canonical loop values required by focused
// analyser and selector tests.
func testLoopParts(
	t *testing.T,
	fn *ssa.Function,
) (*ssa.BasicBlock, *ssa.BinOp, ssa.Value, *ssa.Phi) {
	t.Helper()
	header := loopHeader(fn)
	require.NotNil(t, header)
	ifi, ok := lastIf(header)
	require.True(t, ok)
	cmp, ok := ifi.Cond.(*ssa.BinOp)
	require.True(t, ok)
	bound, ok := loopBound(cmp, header)
	require.True(t, ok)
	counterValue := cmp.X
	if !definedInLoop(counterValue, header) {
		counterValue = cmp.Y
	}
	counter := loopCounterPhi(counterValue)
	require.NotNil(t, counter)
	return header, cmp, bound, counter
}

// writeTempGo gives generated edge cases a real path for SSA loading.
func writeTempGo(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "case.go")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	return path
}
