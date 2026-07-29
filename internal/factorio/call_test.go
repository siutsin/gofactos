// This file protects call validation, planning, and expansion boundaries.
package factorio

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// TestOrdinaryCallsSimulate proves direct, repeated, pass-through, and
// branching calls expand into the caller while preserving Go results.
func TestOrdinaryCallsSimulate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		path   string
		fn     string
		params []int
		want   int
	}{
		{
			name:   "repeated call",
			path:   "../testdata/calls.go",
			fn:     "sumSquares",
			params: []int{3, -4},
			want:   25,
		},
		{
			name:   "pass through calls share a net",
			path:   "../testdata/calls.go",
			fn:     "sumIdentities",
			params: []int{-7},
			want:   -14,
		},
		{
			name:   "caller and callee branch",
			path:   "../testdata/branchcall.go",
			fn:     "branchCall",
			params: []int{-3},
			want:   3,
		},
		{
			name:   "zero branch",
			path:   "../testdata/branchcall.go",
			fn:     "branchCall",
			params: []int{0},
			want:   1,
		},
		{
			name:   "positive branch",
			path:   "../testdata/branchcall.go",
			fn:     "branchCall",
			params: []int{4},
			want:   4,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(
				t,
				tc.want,
				simulateParams(t, tc.path, tc.fn, tc.params...),
			)
		})
	}
}

// TestPlannedRootSupportsChainedMerges proves call expansion preserves phi
// handling for multiple assignment joins in the selected root.
func TestPlannedRootSupportsChainedMerges(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, `package main
func identity(n int) int { return n }
func a(n int) int {
	if n < 0 { n = 0 }
	if n > 10 { n = 10 }
	return identity(n)
}
`)
	require.Equal(t, 10, simulateParams(t, path, "a", 12))
}

// TestGenerateSumSquaresValidates checks an expanded ordinary-call blueprint
// imports in factorio-draftsman.
func TestGenerateSumSquaresValidates(t *testing.T) {
	t.Parallel()
	generate(
		t,
		"../testdata/calls.go",
		"sumSquares",
	).validateWithDraftsman()
}

// TestGenerateBranchCallValidates checks caller and callee branches import in
// factorio-draftsman after both call sites expand.
func TestGenerateBranchCallValidates(t *testing.T) {
	t.Parallel()
	generate(
		t,
		"../testdata/branchcall.go",
		"branchCall",
	).validateWithDraftsman()
}

// TestGenerateRecursiveCasesValidate checks every generic recursion shape
// imports in factorio-draftsman with non-trivial inputs.
func TestGenerateRecursiveCasesValidate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path, function string
		params               map[string]int
	}{
		{
			name: "Fibonacci", path: "../testdata/fibonacci.go",
			function: "fibonacci", params: map[string]int{"n": 10},
		},
		{
			name: "factorial", path: "../testdata/recursive/factorial.go",
			function: "factorial", params: map[string]int{"n": 5},
		},
		{
			name:     "greatest common divisor",
			path:     "../testdata/recursive/gcd.go",
			function: "gcd", params: map[string]int{"a": 48, "b": 18},
		},
		{
			name:     "boolean recursion",
			path:     "../testdata/recursive/reacheszero.go",
			function: "reachesZero", params: map[string]int{"n": 5},
		},
		{
			name: "phi merge", path: "../testdata/recursive/weighted.go",
			function: "weighted",
			params:   map[string]int{"n": 4, "double": 1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			generate(
				t,
				tc.path,
				tc.function,
				WithParams(tc.params),
			).validateWithDraftsman()
		})
	}
}

// TestOrdinaryCallsSupportScalarSignatures proves boolean parameters and
// results, multiple arguments, and constant arguments cross call boundaries.
func TestOrdinaryCallsSupportScalarSignatures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		source string
		params []int
		want   int
	}{
		{
			name: "boolean call",
			source: `package main
func invert(v bool) bool {
	if v { return false }
	return true
}
func a(v bool) bool { return invert(v) }
`,
			params: []int{1},
			want:   -1,
		},
		{
			name: "multiple and constant arguments",
			source: `package main
func subtract(a, b int) int { return a - b }
func a(n int) int { return subtract(n, 2) }
`,
			params: []int{-5},
			want:   -7,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempGo(t, tc.source)
			require.Equal(
				t,
				tc.want,
				simulateParams(t, path, "a", tc.params...),
			)
		})
	}
}

// TestNestedCallsUseExpansionPaths proves a source callee expanded twice gets
// fresh nested invocations and deterministic physical paths.
func TestNestedCallsUseExpansionPaths(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, `package main
func c(n int) int { return n + 1 }
func b(n int) int { return c(n) * 2 }
func a(n int) int { return b(n) + b(n) }
`)
	fn := parseTestFile(t, path, "a")
	program, err := planCallProgram(fn)
	require.NoError(t, err)
	require.NotNil(t, program)

	var paths []string
	for _, child := range program.root.calls {
		paths = append(paths, child.path)
		for _, nested := range child.calls {
			paths = append(paths, nested.path)
		}
	}
	require.ElementsMatch(t, []string{
		"b#1",
		"b#1/c#1",
		"b#2",
		"b#2/c#1",
	}, paths)
	var rootCalls []*ssa.Call
	for _, block := range fn.Blocks {
		for _, instruction := range block.Instrs {
			if call, ok := instruction.(*ssa.Call); ok {
				rootCalls = append(rootCalls, call)
			}
		}
	}
	require.Len(t, rootCalls, 2)
	first := program.root.calls[rootCalls[0]]
	second := program.root.calls[rootCalls[1]]
	require.Equal(t, "b#1", first.path)
	require.Equal(t, "b#2", second.path)
	for _, child := range first.calls {
		require.Equal(t, "b#1/c#1", child.path)
	}
	for _, child := range second.calls {
		require.Equal(t, "b#2/c#1", child.path)
	}

	require.Equal(t, 16, simulateParams(t, path, "a", 3))
}

// TestCallExpansionLimit accepts exactly 1,024 physical invocations and rejects
// 1,025 before the over-limit invocation plans are allocated.
func TestCallExpansionLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		depth     int
		leafCalls int
		wantCount int
		wantErr   string
	}{
		{
			name:      "1,024 accepted",
			depth:     9,
			leafCalls: 1,
			wantCount: 1_024,
		},
		{
			name:      "1,025 rejected",
			depth:     9,
			leafCalls: 2,
			wantErr:   "call expansion exceeds 1024 physical invocations",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := repeatedCallChain(t, tc.depth, tc.leafCalls)
			name := fmt.Sprintf("f%d", tc.depth)
			fn := parseTestFile(t, writeTempGo(t, source), name)
			program, err := planCallProgram(fn)
			if tc.wantErr != "" {
				require.Nil(t, program)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, program)
			require.Equal(
				t,
				tc.wantCount,
				invocationPlanCount(program.root),
			)
		})
	}
}

// repeatedCallChain creates predictable fan-out so expansion limits can be
// tested without maintaining a large test case.
func repeatedCallChain(t *testing.T, depth, leafCalls int) string {
	t.Helper()
	var source strings.Builder
	_, err := fmt.Fprintln(&source, "package main")
	require.NoError(t, err)
	_, err = fmt.Fprintln(&source, "func f0(n int) int { return n }")
	require.NoError(t, err)
	_, err = fmt.Fprintln(&source, "func leaf(n int) int { return n }")
	require.NoError(t, err)
	for level := 1; level <= depth; level++ {
		extraCalls := ""
		if level == depth {
			extraCalls = strings.Repeat(" + leaf(n)", leafCalls)
		}
		_, err = fmt.Fprintf(
			&source,
			"func f%d(n int) int { return f%d(n) + f%d(n)%s }\n",
			level,
			level-1,
			level-1,
			extraCalls,
		)
		require.NoError(t, err)
	}
	return source.String()
}

// invocationPlanCount makes the physical expansion cost directly assertable.
func invocationPlanCount(plan *invocationPlan) int {
	count := 1
	for _, child := range plan.calls {
		count += invocationPlanCount(child)
	}
	return count
}

// TestOrdinaryCallsDoNotAddRuntimeState proves compile-time expansion adds no
// clock or register component and retains only root input sources.
func TestOrdinaryCallsDoNotAddRuntimeState(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, "../testdata/calls.go", "sumSquares")
	selected, err := selectFunc(fn)
	require.NoError(t, err)

	var sources, arithmetic, clocks, registers, displays int
	for _, instance := range selected.insts {
		switch instance.comp.(type) {
		case *constSrc:
			sources++
		case *arith:
			arithmetic++
		case *clockDiv:
			clocks++
		case *register:
			registers++
		case *digitDisplay:
			displays++
		}
	}
	require.Equal(t, 2, sources)
	require.Equal(t, 3, arithmetic)
	require.Zero(t, clocks)
	require.Zero(t, registers)
	require.Equal(t, 1, displays)

	var inputs int
	for _, net := range selected.nets {
		if net.isInput {
			inputs++
		}
	}
	require.Equal(t, 2, inputs)
}

// TestPassThroughCallsReuseCallerNet proves identity calls add neither a
// module nor an artificial return net, even when both call values are read.
func TestPassThroughCallsReuseCallerNet(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, "../testdata/calls.go", "sumIdentities")
	selected, err := selectFunc(fn)
	require.NoError(t, err)

	var arithmetic *instance
	for _, instance := range selected.insts {
		if _, ok := instance.comp.(*arith); ok {
			require.Nil(t, arithmetic)
			arithmetic = instance
		}
	}
	require.NotNil(t, arithmetic)
	require.Same(t, arithmetic.port("a").net, arithmetic.port("b").net)
	require.True(t, arithmetic.port("a").net.isInput)
	require.Len(t, selected.nets, 2)
}

// TestCallPreflightDiagnostics classifies unsupported call provenance and body
// shapes before ordinary lowering starts.
func TestCallPreflightDiagnostics(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name: "built-in",
			source: `package main
func a(n int) int { println(n); return n }
`,
			wantErr: "built-in call println in a",
		},
		{
			name: "go statement",
			source: `package main
func b(n int) {}
func a(n int) int {
	next := n + 1
	go b(next)
	return next
}
`,
			wantErr: "non-ordinary call in a",
		},
		{
			name: "defer statement",
			source: `package main
func b(n int) {}
func a(n int) int {
	next := n + 1
	defer b(next)
	return next
}
`,
			wantErr: "non-ordinary call in a",
		},
		{
			name: "dynamic",
			source: `package main
func b(n int) int { return n }
func c(n int) int { return -n }
func a(n int) int {
	f := b
	if n > 0 { f = c }
	return f(n)
}
`,
			wantErr: "dynamic call",
		},
		{
			name: "external",
			source: `package main
import "strconv"
func a(n int) int { _ = strconv.Itoa(n); return n }
`,
			wantErr: "external call Itoa in a",
		},
		{
			name: "method",
			source: `package main
type number int
func (v number) add(n int) int { return int(v) + n }
func a(n int) int { return number(1).add(n) }
`,
			wantErr: "method or closure call add in a",
		},
		{
			name: "closure",
			source: `package main
func a(n int) int {
	f := func(v int) int { return v + 1 }
	return f(n)
}
`,
			wantErr: "method or closure call",
		},
		{
			name: "generic",
			source: `package main
func identity[T ~int](n T) T { return n }
func a(n int) int { return identity(n) }
`,
			wantErr: "generic or variadic call",
		},
		{
			name: "variadic",
			source: `package main
func first(values ...int) int { return values[0] }
func a(n int) int { return first(n) }
`,
			wantErr: "generic or variadic call first in a",
		},
		{
			name: "unsupported signature",
			source: `package main
func b(n float64) int { return int(n) }
func a(n int) int { return b(float64(n)) }
`,
			wantErr: "unsupported parameter or result signature b",
		},
		{
			name: "narrow integer parameter",
			source: `package main
func b(n int8) int { return int(n) }
func a(n int) int { return b(int8(n)) }
`,
			wantErr: "unsupported parameter type int8",
		},
		{
			name: "unsigned integer result",
			source: `package main
func b(n int) uint { return uint(n) }
func a(n int) int { return int(b(n)) }
`,
			wantErr: "unsupported result type uint",
		},
		{
			name: "zero results",
			source: `package main
func b(n int) {}
func a(n int) int { b(n); return n }
`,
			wantErr: "unsupported parameter or result signature b",
		},
		{
			name: "multiple results",
			source: `package main
func b(n int) (int, int) { return n, n }
func a(n int) int { first, _ := b(n); return first }
`,
			wantErr: "unsupported parameter or result signature b",
		},
		{
			name: "unsupported callee body",
			source: `package main
func b(n int) int { return n << 1 }
func a(n int) int { return b(n) }
`,
			wantErr: "unsupported callee body b",
		},
		{
			name: "out of range call argument",
			source: `package main
func b(n int) int { return n }
func a() int { return b(2147483648) }
`,
			wantErr: "outside Factorio signed 32-bit range",
		},
		{
			name: "callee loop",
			source: `package main
func b(n int) int {
	result := 0
	for i := 0; i < n; i++ { result++ }
	return result
}
func a(n int) int { return b(n) }
`,
			wantErr: "calls involving loops are unsupported: b",
		},
		{
			name: "caller loop",
			source: `package main
func b(n int) int { return n }
func a(n int) int {
	result := 0
	for i := 0; i < n; i++ { result += b(i) }
	return result
}
`,
			wantErr: "calls involving loops are unsupported: a",
		},
		{
			name: "mutual recursion",
			source: `package main
func a(n int) int { if n <= 0 { return n }; return b(n - 1) }
func b(n int) int { if n <= 0 { return n }; return a(n - 1) }
`,
			wantErr: "mutual recursion is unsupported: a, b",
		},
		{
			name: "recursive wrapper",
			source: `package main
func fib(n int) int {
	if n <= 1 { return n }
	return fib(n - 1) + fib(n - 2)
}
func a(n int) int { return fib(n) }
`,
			wantErr: "recursive function must be the selected root: fib",
		},
		{
			name: "invalid recursive wrapper body",
			source: `package main
func fib(n int) int {
	if n <= 1 { return n }
	return fib(n - 1) + fib(n - 2)
}
func a(n int) int { return fib(n) << 1 }
`,
			wantErr: "unsupported root body a: select: unsupported operator <<",
		},
		{
			name: "two recursive machines",
			source: `package main
func first(n int) int {
	if n <= 1 { return n }
	return first(n - 1) + first(n - 2)
}
func second(n int) int {
	if n <= 1 { return n }
	return second(n - 1) + second(n - 2)
}
func a(n int) int { return first(n) + second(n) }
`,
			wantErr: "more than one recursive machine: first, second",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, writeTempGo(t, tc.source), "a")
			s := &selector{selectorFrame: selectorFrame{
				producer: map[ssa.Value]*port{},
			}}
			selected, err := s.selectRoot(fn)
			require.Nil(t, selected)
			require.ErrorContains(t, err, tc.wantErr)
			require.Empty(t, s.insts)
			require.Empty(t, s.nets)
		})
	}
}

// TestCallPreflightMetadataDiagnostics proves provenance stored only in SSA
// metadata is classified before body validation or lowering.
func TestCallPreflightMetadataDiagnostics(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		mutate  func(*ssa.Function)
		wantErr string
	}{
		{
			name: "synthetic",
			mutate: func(fn *ssa.Function) {
				fn.Synthetic = "test wrapper"
				fn.Pkg = &ssa.Package{}
			},
			wantErr: "synthetic call b in a",
		},
		{
			name: "bodyless",
			mutate: func(fn *ssa.Function) {
				fn.Blocks = nil
			},
			wantErr: "bodyless call b in a",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, writeTempGo(t, `package main
func b(n int) int { return n }
func a(n int) int { return b(n) }
`), "a")
			var call *ssa.Call
			for _, instruction := range fn.Blocks[0].Instrs {
				if candidate, ok := instruction.(*ssa.Call); ok {
					call = candidate
					break
				}
			}
			require.NotNil(t, call)
			callee := call.Common().StaticCallee()
			require.NotNil(t, callee)
			tc.mutate(callee)

			s := &selector{selectorFrame: selectorFrame{
				producer: map[ssa.Value]*port{},
			}}
			selected, err := s.selectRoot(fn)
			require.Nil(t, selected)
			require.ErrorContains(t, err, tc.wantErr)
			require.Empty(t, s.insts)
			require.Empty(t, s.nets)
		})
	}
}

// TestCallErrorsNameTheOffendingFunction keeps diagnostics useful when a
// nested helper, rather than the selected root, violates the body contract.
func TestCallErrorsNameTheOffendingFunction(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, writeTempGo(t, `package main
func c(n int) int { return n & 1 }
func b(n int) int { return c(n) }
func a(n int) int { return b(n) }
`), "a")
	_, err := selectFunc(fn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "callee body c")
}
