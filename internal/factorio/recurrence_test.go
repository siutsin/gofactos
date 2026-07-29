// This file guards the accepted recurrence shape and its timing limits.
package factorio

import (
	"bytes"
	"go/constant"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// analyseRecurrenceLoop applies the default clock budget in recurrence tests.
func analyseRecurrenceLoop(
	fn *ssa.Function,
	header *ssa.BasicBlock,
	cmp *ssa.BinOp,
	bound ssa.Value,
	counter *ssa.Phi,
) (recurrenceLoop, error) {
	return analyseRecurrenceLoopWithBudget(
		fn,
		header,
		cmp,
		bound,
		counter,
		clockedStateSettleBudgetFor(clockPeriod),
	)
}

// TestAnalyseRecurrenceLoop describes the committed Fibonacci test case in SSA
// order, including its direct and additive next-state values.
func TestAnalyseRecurrenceLoop(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, "../testdata/fib.go", "fib")
	header, cmp, bound, counter := testLoopParts(t, fn)

	got, err := analyseRecurrenceLoop(
		fn,
		header,
		cmp,
		bound,
		counter,
	)

	require.NoError(t, err)
	require.Same(t, bound, got.bound)
	require.Same(t, counter, got.counter)
	require.Equal(t, "t0", got.result.Name())
	require.Equal(t, []string{"t0", "t1", "t2"}, statePhiNames(got))
	require.Equal(t, []int{0, 1, 0}, stateInitials(got))
	require.Equal(t, []string{"t1", "t4", "t5"}, stateNextNames(got))
	require.Equal(t, []string{"t4", "t5"}, bodyOpNames(got))
}

// TestAnalyseRecurrenceLoopAcceptsGeneralOperands covers negative initials and
// recurrence additions with parameter and integer-constant operands.
func TestAnalyseRecurrenceLoopAcceptsGeneralOperands(t *testing.T) {
	t.Parallel()
	fn := parseLoopTestFunction(t, "", `package main
func general(n, step int) int {
	a, b := -2, 3
	for i := 0; i < n; i++ {
		a, b = b+step, a+4
	}
	return a
}
`, "general")
	header, cmp, bound, counter := testLoopParts(t, fn)

	got, err := analyseRecurrenceLoop(
		fn,
		header,
		cmp,
		bound,
		counter,
	)

	require.NoError(t, err)
	require.Equal(t, []int{-2, 3, 0}, stateInitials(got))
	require.True(t, bodyOpsUseParameter(got.bodyOps, fn.Params[1]))
	require.True(t, bodyOpsUseConstant(got.bodyOps, 4))
}

// TestAnalyseRecurrenceLoopAcceptsChaining proves acyclic body additions are
// ordered before their consumers and retained in the descriptor.
func TestAnalyseRecurrenceLoopAcceptsChaining(t *testing.T) {
	t.Parallel()
	fn := parseChainedRecurrence(t)
	header, cmp, bound, counter := testLoopParts(t, fn)

	got, err := analyseRecurrenceLoop(
		fn,
		header,
		cmp,
		bound,
		counter,
	)

	require.NoError(t, err)
	require.Len(t, got.states, 4)
	require.Len(t, got.bodyOps, 3)
	require.Same(t, got.bodyOps[0], got.bodyOps[1].X)
}

// TestAnalyseRecurrenceLoopRejectsUnsupportedShape covers deterministic
// failures for unsupported CFG, state, instruction, and result forms.
func TestAnalyseRecurrenceLoopRejectsUnsupportedShape(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name: "wrong CFG",
			source: `package main
func bad(n int) int {
	a, b := 0, 1
	for i := 0; i < n; i++ {
		if a > 0 {
			a = b
		}
		b = a + b
	}
	return a
}
`,
			wantErr: "recurrence loop has unsupported control flow",
		},
		{
			name: "unsupported operator",
			source: `package main
func bad(n int) int {
	a, b := 1, 1
	for i := 0; i < n; i++ {
		a, b = b, a*b
	}
	return a
}
`,
			wantErr: "recurrence body operator * is unsupported",
		},
		{
			name: "non-constant initial",
			source: `package main
func bad(n int) int {
	a, b := 0, n
	for i := 0; i < n; i++ {
		a, b = b, a+b
	}
	return a
}
`,
			wantErr: "phi initial value must be an integer constant",
		},
		{
			name: "boolean initial",
			source: `package main
func bad(n int) bool {
	a, b := false, true
	for i := 0; i < n; i++ {
		a, b = b, a
	}
	return a
}
`,
			wantErr: "recurrence state type bool is unsupported",
		},
		{
			name: "call in body",
			source: `package main
func step(a, b int) int { return a + b }
func bad(n int) int {
	a, b := 0, 1
	for i := 0; i < n; i++ {
		a, b = b, step(a, b)
	}
	return a
}
`,
			wantErr: "body instruction *ssa.Call is unsupported",
		},
		{
			name: "computed exit result",
			source: `package main
func bad(n int) int {
	a, b := 0, 1
	for i := 0; i < n; i++ {
		a, b = b, a+b
	}
	return a + b
}
`,
			wantErr: "recurrence exit must return a header phi",
		},
		{
			name: "computed preheader bound",
			source: `package main
func bad(n int) int {
	bound := n + 1
	a, b := 0, 1
	for i := 0; i < bound; i++ {
		a, b = b, a+b
	}
	return a
}
`,
			wantErr: "preheader instruction *ssa.BinOp is unsupported",
		},
		{
			name: "reversed condition",
			source: `package main
func bad(n int) int {
	a, b := 0, 1
	for i := 0; n > i; i++ {
		a, b = b, a+b
	}
	return a
}
`,
			wantErr: "recurrence comparator > is unsupported",
		},
		{
			name: "dead recurrence state",
			source: `package main
func bad(n int) int {
	a, b, c, d := 0, 1, 2, 3
	for i := 0; i < n; i++ {
		a, b = b, a+b
		c, d = d, c+d
	}
	return a
}
`,
			wantErr: "recurrence state",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseLoopTestFunction(t, "", tc.source, "bad")
			header, cmp, bound, counter := testLoopParts(t, fn)

			_, err := analyseRecurrenceLoop(
				fn,
				header,
				cmp,
				bound,
				counter,
			)

			require.ErrorContains(t, err, tc.wantErr)
			first := err.Error()
			_, err = analyseRecurrenceLoop(
				fn,
				header,
				cmp,
				bound,
				counter,
			)
			require.EqualError(t, err, first)
		})
	}
}

// TestAnalyseRecurrenceLoopRejectsMalformedDependencies covers malformed phi
// edges and a self-referential body operation.
func TestAnalyseRecurrenceLoopRejectsMalformedDependencies(t *testing.T) {
	t.Parallel()
	t.Run("malformed phi edges", func(t *testing.T) {
		fn := parseTestFile(t, "../testdata/fib.go", "fib")
		header, cmp, bound, counter := testLoopParts(t, fn)
		phi, ok := header.Instrs[0].(*ssa.Phi)
		require.True(t, ok)
		phi.Edges = phi.Edges[:1]

		_, err := analyseRecurrenceLoop(
			fn,
			header,
			cmp,
			bound,
			counter,
		)

		require.ErrorContains(t, err, "recurrence phi has 1 edges")
	})

	t.Run("self body dependency", func(t *testing.T) {
		fn := parseChainedRecurrence(t)
		header, cmp, bound, counter := testLoopParts(t, fn)
		ops := testBodyOps(t, header)
		ops[0].X = ops[0]

		_, err := analyseRecurrenceLoop(
			fn,
			header,
			cmp,
			bound,
			counter,
		)

		require.ErrorContains(t, err, "must precede its use")
	})
}

// TestAnalyseRecurrenceLoopRejectsAmbiguousControlFlow covers each ambiguity
// guard with a stable, useful error.
func TestAnalyseRecurrenceLoopRejectsAmbiguousControlFlow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		mutate  func(*testing.T, *ssa.BasicBlock)
		wantErr string
	}{
		{
			name: "entry edges",
			mutate: func(t *testing.T, header *ssa.BasicBlock) {
				t.Helper()
				var preheader *ssa.BasicBlock
				for _, pred := range header.Preds {
					if !header.Dominates(pred) {
						preheader = pred
					}
				}
				require.NotNil(t, preheader)
				header.Preds[0] = preheader
				header.Preds[1] = preheader
			},
			wantErr: "ambiguous entry edges",
		},
		{
			name: "back edges",
			mutate: func(t *testing.T, header *ssa.BasicBlock) {
				t.Helper()
				var backedge *ssa.BasicBlock
				for _, pred := range header.Preds {
					if header.Dominates(pred) {
						backedge = pred
					}
				}
				require.NotNil(t, backedge)
				header.Preds[0] = backedge
				header.Preds[1] = backedge
			},
			wantErr: "ambiguous back edges",
		},
		{
			name: "branch condition",
			mutate: func(t *testing.T, header *ssa.BasicBlock) {
				t.Helper()
				ifi, ok := lastIf(header)
				require.True(t, ok)
				ifi.Cond = ssa.NewConst(
					constant.MakeBool(true),
					types.Typ[types.Bool],
				)
			},
			wantErr: "header if uses an ambiguous condition",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, "../testdata/fib.go", "fib")
			header, cmp, bound, counter := testLoopParts(t, fn)
			tc.mutate(t, header)

			_, err := analyseRecurrenceLoop(
				fn,
				header,
				cmp,
				bound,
				counter,
			)

			require.ErrorContains(t, err, tc.wantErr)
			first := err.Error()
			_, err = analyseRecurrenceLoop(
				fn,
				header,
				cmp,
				bound,
				counter,
			)
			require.EqualError(t, err, first)
		})
	}
}

// TestAnalyseRecurrenceLoopIsPureAndDeterministic proves repeated analysis
// leaves SSA unchanged and returns stable header and body order.
func TestAnalyseRecurrenceLoopIsPureAndDeterministic(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, "../testdata/fib.go", "fib")
	header, cmp, bound, counter := testLoopParts(t, fn)
	before := writeSSAFunction(fn)
	first, err := analyseRecurrenceLoop(
		fn,
		header,
		cmp,
		bound,
		counter,
	)
	require.NoError(t, err)
	require.Equal(t, before, writeSSAFunction(fn))

	second, err := analyseRecurrenceLoop(
		fn,
		header,
		cmp,
		bound,
		counter,
	)
	require.NoError(t, err)
	require.Equal(t, before, writeSSAFunction(fn))
	require.Equal(
		t,
		append(statePhiNames(first), bodyOpNames(first)...),
		append(statePhiNames(second), bodyOpNames(second)...),
	)
}

// parseChainedRecurrence provides one shared multi-stage recurrence for shape
// and dependency assertions.
func parseChainedRecurrence(t *testing.T) *ssa.Function {
	t.Helper()
	return parseLoopTestFunction(t, "", `package main
func chained(n int) int {
	a, b, c := 0, 1, 2
	for i := 0; i < n; i++ {
		sum := a + b
		a, b, c = b, c, sum+c
	}
	return a
}
`, "chained")
}

// statePhiNames exposes stable state ordering without coupling tests to
// complete SSA values.
func statePhiNames(loop recurrenceLoop) []string {
	names := make([]string, 0, len(loop.states))
	for _, state := range loop.states {
		names = append(names, state.phi.Name())
	}
	return names
}

// stateInitials makes the recurrence's source-level seeds directly assertable.
func stateInitials(loop recurrenceLoop) []int {
	initials := make([]int, 0, len(loop.states))
	for _, state := range loop.states {
		initials = append(initials, state.initial)
	}
	return initials
}

// stateNextNames exposes deterministic next-value ordering for comparison.
func stateNextNames(loop recurrenceLoop) []string {
	names := make([]string, 0, len(loop.states))
	for _, state := range loop.states {
		names = append(names, state.next.Name())
	}
	return names
}

// bodyOpNames keeps body-order assertions concise and source-correlated.
func bodyOpNames(loop recurrenceLoop) []string {
	names := make([]string, 0, len(loop.bodyOps))
	for _, op := range loop.bodyOps {
		names = append(names, op.Name())
	}
	return names
}

// bodyOpsUseParameter proves loop equations can retain a root input dependency.
func bodyOpsUseParameter(ops []*ssa.BinOp, parameter *ssa.Parameter) bool {
	for _, op := range ops {
		if op.X == parameter || op.Y == parameter {
			return true
		}
	}
	return false
}

// bodyOpsUseConstant proves loop equations retain literal operands.
func bodyOpsUseConstant(ops []*ssa.BinOp, want int) bool {
	for _, op := range ops {
		for _, operand := range []ssa.Value{op.X, op.Y} {
			if isConstInt(operand, want) {
				return true
			}
		}
	}
	return false
}

// testBodyOps extracts the accepted body equations for focused validator tests.
func testBodyOps(t *testing.T, header *ssa.BasicBlock) []*ssa.BinOp {
	t.Helper()
	require.Len(t, header.Succs, 2)
	var ops []*ssa.BinOp
	for _, instr := range header.Succs[0].Instrs {
		if op, ok := instr.(*ssa.BinOp); ok {
			ops = append(ops, op)
		}
	}
	require.GreaterOrEqual(t, len(ops), 2)
	return ops
}

// writeSSAFunction snapshots SSA so purity tests can detect hidden mutation.
func writeSSAFunction(fn *ssa.Function) string {
	var out bytes.Buffer
	ssa.WriteFunction(&out, fn)
	return out.String()
}
