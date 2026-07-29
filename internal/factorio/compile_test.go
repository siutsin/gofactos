// This file protects end-to-end lowering from SSA to executable circuits.
package factorio

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/siutsin/gofactos/internal/ssaloader"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// parseTestFile loads a Go source file and returns the named SSA function.
func parseTestFile(t *testing.T, path, funcName string) *ssa.Function {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	require.NoError(t, err)
	pkg, _, err := ssautil.BuildPackage(
		&types.Config{Importer: importer.Default()},
		fset,
		types.NewPackage("gofactos.test", ""),
		[]*ast.File{file},
		ssa.SanityCheckFunctions,
	)
	require.NoError(t, err)
	for _, member := range pkg.Members {
		if fn, ok := member.(*ssa.Function); ok && fn.Name() == funcName {
			return fn
		}
	}
	t.Fatalf("function %q not found in %s", funcName, path)
	return nil
}

// TestGenerateStraightLine proves Select + the pipeline end to end on
// straight-line arithmetic: parameters default to 1, so the simulator's result
// is the Go function evaluated at all-ones.
func TestGenerateStraightLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		path string
		fn   string
		want int // f(1, 1, ...)
	}{
		{name: "add", path: "../testdata/add.go", fn: "add", want: 1 + 1},
		{name: "constFirst", path: "../testdata/constfirst.go", fn: "constFirst", want: 1*1 + 7*1},
		{name: "answer", path: "../testdata/answer.go", fn: "answer", want: 42},
		{name: "unusedParam", path: "../testdata/unusedparam.go", fn: "unusedParam", want: 1 + 1},
		{name: "deadExpr", path: "../testdata/deadexpr.go", fn: "deadExpr", want: 1 + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, simulateParams(t, tc.path, tc.fn))
		})
	}
}

// TestGenerateUnsupported confirms unsupported paths fail with a clear error
// rather than a panic, so the pipeline degrades cleanly.
func TestGenerateUnsupported(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path, fn, wantErr string
	}{
		{"three-way branch", "../testdata/negative/sign.go", "sign", "more than one branch is unsupported"},
		{"non-integer parameter", "../testdata/negative/half.go", "half", "unsupported parameter type"},
		{"non-zero loop accumulator", "../testdata/negative/forinit.go", "forInit", "accumulator must start at 0"},
		{"loop returns a constant", "../testdata/negative/loopconst.go", "loopConst", "loop result must be a loop-carried accumulator"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, tc.path, tc.fn)
			_, err := Compile(fn)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// TestGenerateRejectsInvalidScalarState prevents scalar-shaped loops from
// generating circuits when their initial or next state violates the contract.
func TestGenerateRejectsInvalidScalarState(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name: "counter plus constant",
			source: `package main
func invalid(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result = i + 2
	}
	return result
}
`,
			wantErr: "select: unsupported loop body, only " +
				"`result += constant` is supported",
		},
		{
			name: "constant plus counter",
			source: `package main
func invalid(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result = 2 + i
	}
	return result
}
`,
			wantErr: "select: unsupported loop body, only " +
				"`result += constant` is supported",
		},
		{
			name: "parameter plus constant",
			source: `package main
func invalid(n, step int) int {
	result := 0
	for i := 0; i < n; i++ {
		result = step + 2
	}
	return result
}
`,
			wantErr: "select: unsupported loop body, only " +
				"`result += constant` is supported",
		},
		{
			name: "pre-loop phi plus constant",
			source: `package main
func invalid(n, choose int) int {
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
			wantErr: "select: unsupported loop body, only " +
				"`result += constant` is supported",
		},
		{
			name: "parameter initial",
			source: `package main
func invalid(n, start int) int {
	result := start
	for i := 0; i < n; i++ {
		result += 2
	}
	return result
}
`,
			wantErr: "select: loop accumulator must start at 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(
				t,
				writeTempGo(t, tc.source),
				"invalid",
			)
			_, err := Compile(fn)
			require.EqualError(t, err, tc.wantErr)
		})
	}
}

// TestGenerateNegativeScalarIncrement proves a valid negative increment keeps
// the scalar lowering and settles on increment times bound.
func TestGenerateNegativeScalarIncrement(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, `package main
func negativeIncrement(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result += -2
	}
	return result
}
`)
	require.Equal(
		t,
		-6,
		simulateLoopSettles(
			t,
			path,
			"negativeIncrement",
			3,
		),
	)
}

// TestSelectRejectsMethodsAndClosures proves Select rejects function shapes the
// backend does not support: methods (a receiver) and closures (a parent), both
// of which the loader's CollectFunctions can still return.
func TestSelectRejectsMethodsAndClosures(t *testing.T) {
	t.Parallel()
	pkg, err := ssaloader.Load("../testdata/negative/methods.go")
	require.NoError(t, err)

	for _, name := range []string{"Increment", "Value"} {
		t.Run(name, func(t *testing.T) {
			fns := ssaloader.CollectFunctions(pkg, name)
			require.Len(t, fns, 1)
			_, err := selectFunc(fns[0])
			require.ErrorContains(t, err, "methods are unsupported")
		})
	}

	t.Run("closure", func(t *testing.T) {
		makers := ssaloader.CollectFunctions(pkg, "MakeAdder")
		require.Len(t, makers, 1)
		require.NotEmpty(t, makers[0].AnonFuncs)
		_, err := selectFunc(makers[0].AnonFuncs[0])
		require.ErrorContains(t, err, "closures are unsupported")
	})
}

// blueprintResult wraps a generated blueprint for draftsman validation.
type blueprintResult struct {
	t  *testing.T
	bp *BlueprintWrapper
}

// generate builds a blueprint from a test file for fluent assertions.
func generate(t *testing.T, path, funcName string, opts ...Option) blueprintResult {
	t.Helper()
	fn := parseTestFile(t, path, funcName)
	bp, err := Compile(fn, opts...)
	require.NoError(t, err)
	return blueprintResult{t: t, bp: bp}
}

// draftsmanScript validates a blueprint string with factorio-draftsman.
const draftsmanScript = `
import sys, warnings
warnings.filterwarnings("ignore")
from draftsman.blueprintable import get_blueprintable_from_string
bp = get_blueprintable_from_string(sys.stdin.read())
for e in bp.entities:
    print(e.name)
print("OK")
`

// validateWithDraftsman encodes the blueprint and requires factorio-draftsman
// to import it. A missing or broken validator is a test failure, not a skip.
func (r blueprintResult) validateWithDraftsman() {
	r.t.Helper()
	encoded, err := Encode(r.bp)
	require.NoError(r.t, err)
	cmd := exec.CommandContext(
		r.t.Context(),
		"uv",
		"run",
		"--locked",
		"python",
		"-c",
		draftsmanScript,
	)
	cmd.Stdin = strings.NewReader(encoded)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("draftsman validation failed: %v\n%s", err, out)
	}
	assert.Contains(r.t, string(out), "OK")
}

// TestGenerateAddValidates checks the add blueprint imports in factorio-draftsman.
func TestGenerateAddValidates(t *testing.T) {
	t.Parallel()
	generate(t, "../testdata/add.go", "add").validateWithDraftsman()
}

// TestGenerateWithParams proves --set (WithParams) bakes parameter values into
// the constant sources, so the simulated result is the function at those inputs.
func TestGenerateWithParams(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, "../testdata/add.go", "add")
	g, err := compileFunction(fn, WithParams(map[string]int{"a": 2, "b": 10}))
	require.NoError(t, err)
	s := simulate(t, g.e.entities, g.e.wires, 50)
	b := g.e.bound[g.resultNet.driver]
	require.Equal(t, 12, s.value(int(b.h), b.conn, g.resultNet.signal.Name))
}

// TestGenerateWithParamsUnknown rejects a --set name that matches no parameter.
func TestGenerateWithParamsUnknown(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, "../testdata/add.go", "add")
	_, err := compileFunction(fn, WithParams(map[string]int{"zzz": 1}))
	require.ErrorContains(t, err, "unknown parameter")
}

// TestGenerateWithParamsRejectsOutOfRangeValue prevents --set from emitting a
// constant outside Factorio's signed 32-bit circuit range.
func TestGenerateWithParamsRejectsOutOfRangeValue(t *testing.T) {
	t.Parallel()
	if strconv.IntSize < 64 {
		t.Skip("host int cannot represent a value above int32")
	}
	outside := int(int64(math.MaxInt32) + 1)
	fn := parseTestFile(t, "../testdata/add.go", "add")
	_, err := compileFunction(
		fn,
		WithParams(map[string]int{"a": outside}),
	)
	require.ErrorContains(
		t,
		err,
		"parameter a value "+strconv.Itoa(outside)+" is outside "+
			"Factorio signed 32-bit range",
	)
}

// TestGenerateBoolParam proves a boolean parameter rides the 1/-1 encoding:
// zero emits -1, while every non-zero setting emits 1.
func TestGenerateBoolParam(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		set        map[string]int
		want       int
		wantSource int
	}{
		{name: "default seeds true", want: 5, wantSource: 1},
		{
			name: "set true", set: map[string]int{"b": 1},
			want: 5, wantSource: 1,
		},
		{
			name: "set false", set: map[string]int{"b": 0},
			want: 3, wantSource: -1,
		},
		{
			name: "set negative non-zero", set: map[string]int{"b": -1},
			want: 5, wantSource: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, "../testdata/pick.go", "pick")
			var opts []Option
			if tc.set != nil {
				opts = append(opts, WithParams(tc.set))
			}
			g, err := compileFunction(fn, opts...)
			require.NoError(t, err)
			s := simulate(t, g.e.entities, g.e.wires, 50)
			b := g.e.bound[g.resultNet.driver]
			require.Equal(t, tc.want, s.value(int(b.h), b.conn, g.resultNet.signal.Name))
			require.Equal(
				t,
				tc.wantSource,
				emittedConstantValue(t, g.e.entities, inputSignals[0]),
			)
		})
	}
}

// emittedConstantValue finds the sole emitted source for one allocated signal.
func emittedConstantValue(
	t *testing.T,
	entities []entity,
	signal signalID,
) int {
	t.Helper()
	var values []int
	for _, ent := range entities {
		if ent.Name != constCombinatorName || ent.ControlBehavior == nil ||
			ent.ControlBehavior.Sections == nil {
			continue
		}
		for _, section := range ent.ControlBehavior.Sections.Sections {
			for _, filter := range section.Filters {
				if filter.Type == signal.Type && filter.Name == signal.Name {
					values = append(values, filter.Count)
				}
			}
		}
	}
	require.Len(t, values, 1)
	return values[0]
}

// TestGenerateUsesLegendarySubstationsOnly proves quality is scoped to power
// substations, the only generated entities that benefit from legendary reach.
func TestGenerateUsesLegendarySubstationsOnly(t *testing.T) {
	t.Parallel()
	bp := generate(t, "../testdata/abs.go", "abs").bp
	substations := 0
	for _, ent := range bp.Blueprint.Entities {
		if ent.Name == powerPoleEntityName {
			substations++
			require.Equal(t, powerPoleQuality, ent.Quality)
			continue
		}
		require.Empty(t, ent.Quality, ent.Name)
	}
	require.Positive(t, substations)
}

// bakeParams sets each parameter's constSrc to a given value by signature
// index, so a whole-function test case can be simulated per input tuple
// (parameters lower to editable constSrc inputs). It bakes via the input-tagged
// nets rather than instance position, so it is robust to Select pruning an
// unused parameter, whose net is absent and which the result never reads.
func bakeParams(t *testing.T, sel *selected, params ...int) {
	t.Helper()
	for _, n := range sel.nets {
		if n.isInput && n.inputIndex < len(params) {
			source, ok := n.driver.inst.comp.(*constSrc)
			require.True(t, ok)
			source.value = params[n.inputIndex]
		}
	}
}

// withPositionalParams converts test case arguments to the named parameter
// option used by the complete generation pipeline.
func withPositionalParams(
	t *testing.T,
	fn *ssa.Function,
	params ...int,
) Option {
	t.Helper()
	require.LessOrEqual(t, len(params), len(fn.Params))
	values := make(map[string]int, len(params))
	for i, value := range params {
		values[fn.Params[i].Name()] = value
	}
	return func(s *selector) {
		merged := make(map[string]int, len(s.paramValues)+len(values))
		maps.Copy(merged, s.paramValues)
		maps.Copy(merged, values)
		s.paramValues = merged
	}
}

// simulateParams compiles fn through Select and the pipeline with its
// parameters baked to the given values in source order, then returns the
// simulator's settled result.
func simulateParams(t *testing.T, path, fn string, params ...int) int {
	t.Helper()
	f := parseTestFile(t, path, fn)
	g, err := compileFunction(f, withPositionalParams(t, f, params...))
	require.NoError(t, err)
	s := simulate(t, g.e.entities, g.e.wires, 50)
	b := g.e.bound[g.resultNet.driver]
	return s.value(int(b.h), b.conn, g.resultNet.signal.Name)
}

// TestGenerateAbs proves Select lowers abs's two return arms to one merge: the
// negative input negates and the positive input passes through.
func TestGenerateAbs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		n    int
		want int
	}{
		{name: "negative", n: -5, want: 5},
		{name: "positive", n: 7, want: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, simulateParams(t, "../testdata/abs.go", "abs", tc.n))
		})
	}
}

// TestGenerateIsEven proves the boolean 1/-1 encoding through Select: an even
// input settles the result to 1 (true) and an odd input to -1 (false), the
// values the Boolean display later renders as check and deny icons.
func TestGenerateIsEven(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		n    int
		want int
	}{
		{name: "even", n: 4, want: 1},
		{name: "odd", n: 7, want: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, simulateParams(t, "../testdata/iseven.go", "isEven", tc.n))
		})
	}
}

// TestGenerateClamp proves mid-function phi handling: two chained `*ssa.Phi`
// merges constrain x to [0, 100], below/within/above the bounds.
func TestGenerateClamp(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		x    int
		want int
	}{
		{name: "below", x: -5, want: 0},
		{name: "within", x: 50, want: 50},
		{name: "above", x: 150, want: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, simulateParams(t, "../testdata/clamp.go", "clamp", tc.x))
		})
	}
}

// TestGeneratePhiThenBranch combines a mid-function phi with an independent
// two-return branch and exercises both arms of each merge.
func TestGeneratePhiThenBranch(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, `package main
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
`)
	for _, tc := range []struct {
		name string
		n    int
		want int
	}{
		{name: "negative above bound", n: -20, want: 20},
		{name: "negative below bound", n: -8, want: 9},
		{name: "positive below bound", n: 5, want: 6},
		{name: "positive above bound", n: 15, want: 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, simulateParams(t, path, "mixed", tc.n))
		})
	}
}

// TestGenerateEarlyReturnBeforePhi proves an early-return arm can be merged
// with a later return whose value passes through another branch and phi.
func TestGenerateEarlyReturnBeforePhi(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, `package main
func nested(a, b bool) int {
	if a {
		return 1
	}
	x := 2
	if b {
		x = 3
	}
	return x
}
`)
	for _, tc := range []struct {
		name       string
		a, b, want int
	}{
		{name: "early return", a: 1, b: 0, want: 1},
		{name: "early return ignores later branch", a: 1, b: 1, want: 1},
		{name: "later phi false arm", a: 0, b: 0, want: 2},
		{name: "later phi true arm", a: 0, b: 1, want: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, simulateParams(t, path, "nested", tc.a, tc.b))
		})
	}
}

// TestGenerateRejectsNondominatingResultBranch prevents an inner condition
// from selecting a return when its enclosing branch never executed.
func TestGenerateRejectsNondominatingResultBranch(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, `package main
func nested(a, b bool) int {
	x := 0
	if a {
		if b { return 1 }
		x = 2
	}
	return x
}
`)
	fn := parseTestFile(t, path, "nested")
	_, err := Compile(fn)
	require.ErrorContains(
		t,
		err,
		"result branch must dominate both returns",
	)
}

// TestGenerateRecursiveFivePhiMoves protects the widest supported recursive
// action from producing an over-reach private wire.
func TestGenerateRecursiveFivePhiMoves(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, `package main
func five(n bool) bool {
	a, b, c, d, e := false, false, false, false, false
	if n {
		a, b, c, d, e = true, true, true, true, true
	}
	if !a { return false }
	if !b { return false }
	if !c { return false }
	if !d { return false }
	if !e { return false }
	return five(e)
}
`)

	result := generate(t, path, "five")

	require.NotEmpty(t, result.bp.Blueprint.Entities)
}

// TestGenerateDivByZero pins the Factorio-compatible zero-divisor result:
// division by zero settles to 0 instead of panicking.
func TestGenerateDivByZero(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, simulateParams(t, "../testdata/divzero.go", "divZero", 7))
}

// TestGenerateMax proves a phi that merges two parameters. The `>=` comparison
// selects the larger input. Other test cases negate an arm or merge a bound.
func TestGenerateMax(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		a, b, want int
	}{
		{"first", 9, 4, 9},
		{"second", 4, 9, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, simulateParams(t, "../testdata/max.go", "max", tc.a, tc.b))
		})
	}
}

// TestGenerateBoolCompare proves a boolean returned straight from a comparison,
// with no if/else merge: the result settles to the 1/-1 condition, 1 for true
// and -1 for false, the encoding the boolDisplay shows as a check or deny icon.
func TestGenerateBoolCompare(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		a, b, want int
	}{
		{"true", 5, 3, 1},
		{"false", 3, 5, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, simulateParams(t, "../testdata/greater.go", "greater", tc.a, tc.b))
		})
	}
}

// TestGenerateBothPos proves a short-circuit `&&` lowers to a boolean merge
// whose false arm is a constant: the result is 1 only when both operands are
// positive and -1 otherwise.
func TestGenerateBothPos(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		a, b, want int
	}{
		{"both positive", 3, 4, 1},
		{"second not positive", 3, -1, -1},
		{"first not positive", -1, 4, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, simulateParams(t, "../testdata/bothpos.go", "bothPos", tc.a, tc.b))
		})
	}
}

// TestGenerateAbsValidates checks the abs blueprint imports in factorio-draftsman.
func TestGenerateAbsValidates(t *testing.T) {
	t.Parallel()
	generate(t, "../testdata/abs.go", "abs").validateWithDraftsman()
}

// TestGenerateClampValidates checks the clamp blueprint imports in draftsman.
func TestGenerateClampValidates(t *testing.T) {
	t.Parallel()
	generate(t, "../testdata/clamp.go", "clamp").validateWithDraftsman()
}

// TestGenerateIsEvenValidates checks the isEven blueprint imports in draftsman.
func TestGenerateIsEvenValidates(t *testing.T) {
	t.Parallel()
	generate(t, "../testdata/iseven.go", "isEven").validateWithDraftsman()
}

// simulateLoopSettles compiles a loop, advances the clock until it stops at the
// bound, and returns the settled result. It asserts the value has stopped
// changing for over a full clock period, proving the loop halts rather than
// cycles.
func simulateLoopSettles(t *testing.T, path, fn string, n int, opts ...Option) int {
	t.Helper()
	f := parseTestFile(t, path, fn)
	opts = append(opts, withPositionalParams(t, f, n))
	g, err := compileFunction(f, opts...)
	require.NoError(t, err)
	configured := &selector{clockPeriod: clockPeriod}
	for _, opt := range opts {
		opt(configured)
	}
	period := effectiveClockPeriod(configured.clockPeriod)
	b := g.e.bound[g.resultNet.driver]
	setClockStarted(t, g.e.entities, true)
	s := newSim(g.e.wires)
	var last, stable int
	for range (n + 4) * period {
		s.advance(g.e.entities)
		v := s.value(int(b.h), b.conn, g.resultNet.signal.Name)
		if v == last {
			stable++
			continue
		}
		stable, last = 0, v
	}
	require.Greater(t, stable, period,
		"loop must settle on a final value, not cycle")
	return last
}

// selectLoopForTest gives timing tests the same selected and clocked loop
// shape used by production generation.
func selectLoopForTest(
	t *testing.T,
	path, fn string,
	n int,
) *selected {
	t.Helper()
	f := parseTestFile(t, path, fn)
	sel, err := selectFunc(f)
	require.NoError(t, err)
	bakeParams(t, sel, n)
	clockPhase(sel)
	return sel
}

// emitSelectedLoop runs Allocate through Emit for focused timing tests that
// inspect component bindings before routing and power are added.
func emitSelectedLoop(t *testing.T, sel *selected) *emitter {
	t.Helper()
	require.NoError(t, allocateSignals(sel.nets))
	require.NoError(t, verifyNetlist(sel.insts, sel.nets))
	place(sel.insts, sel.nets)
	e := emitNetlist(sel.insts, netEdges(sel.nets))
	setClockStarted(t, e.entities, true)
	return e
}

// selectedStopGate locates the unique loop boundary whose timing is under test.
func selectedStopGate(t *testing.T, sel *selected) *instance {
	t.Helper()
	for _, in := range sel.insts {
		if _, ok := in.comp.(*stopGate); ok {
			return in
		}
	}
	t.Fatal("stop gate not found")
	return nil
}

// selectedNetBySSAName connects circuit assertions back to the source SSA
// value that motivated them.
func selectedNetBySSAName(
	t *testing.T,
	sel *selected,
	name string,
) *netlistNet {
	t.Helper()
	for _, net := range sel.nets {
		if net.ssaName == name {
			return net
		}
	}
	t.Fatalf("SSA net %q not found", name)
	return nil
}

// readSelectedNet hides emitter binding details from timing assertions.
func readSelectedNet(
	t *testing.T,
	sim *sim,
	e *emitter,
	net *netlistNet,
) int {
	t.Helper()
	binding, ok := e.bound[net.driver]
	require.True(t, ok)
	return sim.value(
		int(binding.h),
		binding.conn,
		net.signal.Name,
	)
}

// TestGenerateClockStartResetsForRerun proves every clocked loop stays idle
// until START, clears its state while OFF, and computes the same answer after
// restarting. The recurrence case also exercises its private warm-up reset.
func TestGenerateClockStartResetsForRerun(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path, fn string
		n, want        int
	}{
		{
			name: "scalar", path: "../testdata/fori.go",
			fn: "forI", n: 5, want: 10,
		},
		{
			name: "recurrence", path: "../testdata/fib.go",
			fn: "fib", n: 10, want: 55,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := parseTestFile(t, tc.path, tc.fn)
			g, err := compileFunction(f, withPositionalParams(t, f, tc.n))
			require.NoError(t, err)
			binding := g.e.bound[g.resultNet.driver]
			s := newSim(g.e.wires)
			result := func() int {
				return s.value(
					int(binding.h),
					binding.conn,
					g.resultNet.signal.Name,
				)
			}

			for range clockPeriod {
				s.advance(g.e.entities)
				require.Zero(t, result())
			}
			setClockStarted(t, g.e.entities, true)
			for range (tc.n + 4) * clockPeriod {
				s.advance(g.e.entities)
			}
			require.Equal(t, tc.want, result())

			setClockStarted(t, g.e.entities, false)
			for range clockPeriod {
				s.advance(g.e.entities)
			}
			require.Zero(t, result())

			setClockStarted(t, g.e.entities, true)
			for range (tc.n + 4) * clockPeriod {
				s.advance(g.e.entities)
			}
			require.Equal(t, tc.want, result())
		})
	}
}

// TestGenerateForRange proves the for-range self-loop lowers the same way as
// forI: its body `result += 2` gives result = 2*i and settles on 2*n.
func TestGenerateForRange(t *testing.T) {
	t.Parallel()
	const n = 3
	require.Equal(t, 2*n, simulateLoopSettles(t, "../testdata/forrange.go", "forRange", n))
}

// TestGenerateForCounter locks the scalar loop that returns its counter rather
// than a separate accumulator, including the zero-iteration boundary.
func TestGenerateForCounter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		n    int
	}{
		{name: "zero", n: 0},
		{name: "one", n: 1},
		{name: "four", n: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := simulateLoopSettles(
				t,
				"../testdata/forcounter.go",
				"forCounter",
				tc.n,
			)
			require.Equal(t, tc.n, got)
		})
	}
}

// TestGenerateZeroScalarLoopAdvancesAndStops proves a zero result does not hide
// a stalled counter or a gate that continues pulsing after the positive bound.
func TestGenerateZeroScalarLoopAdvancesAndStops(t *testing.T) {
	t.Parallel()
	const n = 3
	path := writeTempGo(t, `package main
func zeroResult(n int) int {
	for i := 0; i < n; i++ {
	}
	return 0
}
`)
	sel := selectLoopForTest(t, path, "zeroResult", n)
	e := emitSelectedLoop(t, sel)
	gate := selectedStopGate(t, sel)
	index := gate.port("index").net
	gated := gate.port("gated").net
	sim := newSim(e.wires)

	reachedBound := false
	for range (n + 2) * clockPeriod {
		sim.advance(e.entities)
		if readSelectedNet(t, sim, e, index) == n {
			reachedBound = true
			break
		}
	}
	require.True(t, reachedBound)
	for tick := range clockPeriod {
		sim.advance(e.entities)
		require.Equal(t, n, readSelectedNet(t, sim, e, index), "tick %d", tick)
		require.Zero(t, readSelectedNet(t, sim, e, gated), "tick %d", tick)
		require.Zero(t, readSelectedNet(t, sim, e, sel.resultNet), "tick %d", tick)
	}
}

// TestGenerateFibonacci proves recurrence state advances in parallel and
// settles on the Go function's result.
func TestGenerateFibonacci(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		n    int
		want int
	}{
		{n: 0, want: 0},
		{n: 1, want: 1},
		{n: 2, want: 1},
		{n: 5, want: 5},
		{n: 10, want: 55},
	} {
		t.Run(strconv.Itoa(tc.n), func(t *testing.T) {
			got := simulateLoopSettles(
				t,
				"../testdata/fib.go",
				"fib",
				tc.n,
			)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestGenerateFibonacciWrapsAtInt32 proves the simulator follows Factorio when
// fib(47) crosses the signed 32-bit boundary.
func TestGenerateFibonacciWrapsAtInt32(t *testing.T) {
	t.Parallel()
	got := simulateLoopSettles(
		t,
		"../testdata/fib.go",
		"fib",
		47,
	)
	require.Equal(t, -1_323_752_223, got)
}

// TestGenerateFibonacciZeroBoundNeverPulses proves a zero bound closes the
// recurrence gate before the clock can update any state.
func TestGenerateFibonacciZeroBoundNeverPulses(t *testing.T) {
	t.Parallel()
	sel := selectLoopForTest(t, "../testdata/fib.go", "fib", 0)
	e := emitSelectedLoop(t, sel)
	gate := selectedStopGate(t, sel)
	binding := e.bound[gate.port("gated")]
	sim := newSim(e.wires)

	for tick := range 2 * clockPeriod {
		sim.advance(e.entities)
		got := sim.value(
			int(binding.h),
			binding.conn,
			gate.port("gated").net.signal.Name,
		)
		require.Zero(t, got, "tick %d", tick)
	}
}

// TestGenerateFibonacciStateProgression proves generated registers read the old
// state together, latch in parallel, and stop after exactly n pulses.
func TestGenerateFibonacciStateProgression(t *testing.T) {
	t.Parallel()
	const n = 5
	sel := selectLoopForTest(t, "../testdata/fib.go", "fib", n)
	e := emitSelectedLoop(t, sel)
	previous := selectedNetBySSAName(t, sel, "t0")
	current := selectedNetBySSAName(t, sel, "t1")
	index := selectedNetBySSAName(t, sel, "t2")
	gated := selectedStopGate(t, sel).port("gated").net
	sim := newSim(e.wires)

	readState := func() [3]int {
		return [3]int{
			readSelectedNet(t, sim, e, previous),
			readSelectedNet(t, sim, e, current),
			readSelectedNet(t, sim, e, index),
		}
	}
	sim.advance(e.entities)
	got := [][3]int{readState()}
	for tick := 0; tick < (n+2)*clockPeriod && len(got) <= n; tick++ {
		sim.advance(e.entities)
		if readSelectedNet(t, sim, e, gated) == 0 {
			continue
		}
		for range 6 {
			sim.advance(e.entities)
		}
		got = append(got, readState())
	}
	require.Equal(t, [][3]int{
		{0, 1, 0},
		{1, 1, 1},
		{1, 2, 2},
		{2, 3, 3},
		{3, 5, 4},
		{5, 8, 5},
	}, got)

	settled := readState()
	for range 2 * clockPeriod {
		sim.advance(e.entities)
		require.Zero(t, readSelectedNet(t, sim, e, gated))
		require.Equal(t, settled, readState())
	}
}

// TestGenerateRecurrenceOperands proves lowering follows the analyser's
// deterministic order for parameters, constants, and acyclic addition chains.
func TestGenerateRecurrenceOperands(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		source string
		fn     string
		n      int
		opts   []Option
		want   int
	}{
		{
			name: "parameter and constant",
			source: `package main
func general(n, step int) int {
	a, b := -2, 3
	for i := 0; i < n; i++ {
		a, b = b+step, a+4
	}
	return a
}
`,
			fn:   "general",
			n:    2,
			opts: []Option{WithParams(map[string]int{"step": 5})},
			want: 7,
		},
		{
			name: "addition chain",
			source: `package main
func chained(n int) int {
	a, b, c := 0, 1, 2
	for i := 0; i < n; i++ {
		sum := a + b
		a, b, c = b, c, sum+c
	}
	return a
}
`,
			fn:   "chained",
			n:    4,
			want: 6,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempGo(t, tc.source)
			got := simulateLoopSettles(
				t,
				path,
				tc.fn,
				tc.n,
				tc.opts...,
			)
			require.Equal(t, tc.want, got)
		})
	}
}

// deepChainSource is shared by the default- and fast-clock recurrence tests.
const deepChainSource = `package main
func deepChain(n int) int {
	a, b, c, d, result := 1, 2, 3, 4, 5
	for i := 0; i < n; i++ {
		ab := a + b
		abc := ab + c
		abcd := abc + d
		a, b, c, d, result = b, c, d, result, abcd+result
	}
	return result
}
`

// TestGenerateDeepRecurrenceChainSettles guards the first write against
// latching a partially propagated four-level body-add chain.
func TestGenerateDeepRecurrenceChainSettles(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, deepChainSource)
	fn := parseTestFile(t, path, "deepChain")
	header, cmp, bound, counter := testLoopParts(t, fn)
	loop, err := analyseRecurrenceLoop(
		fn,
		header,
		cmp,
		bound,
		counter,
	)
	require.NoError(t, err)
	sel := selectLoopForTest(t, path, "deepChain", 1)
	e := emitSelectedLoop(t, sel)
	resultNext := deepChainResultNextNet(t, loop, sel)
	raw := selectedClockNet(t, sel)
	gated := selectedStopGate(t, sel).port("gated").net
	timing := measureDeepChainTiming(
		t,
		e,
		sel,
		resultNext,
		raw,
		gated,
	)
	require.Equal(t, 15, timing.result)
	require.Positive(t, timing.firstRaw)
	require.Positive(t, timing.settled)
	require.Positive(t, timing.firstGated)
	require.Positive(t, timing.firstWrite)
	t.Logf(
		"raw=%d settled=%d gated=%d write=%d",
		timing.firstRaw,
		timing.settled,
		timing.firstGated,
		timing.firstWrite,
	)
	require.GreaterOrEqual(
		t,
		timing.firstGated-timing.firstRaw,
		clockPeriod,
	)
	require.Less(t, timing.settled, timing.firstGated)
	require.Greater(t, timing.firstWrite, timing.firstGated)
}

// deepChainResultNextNet identifies the last equation whose settlement gates
// a safe recurrence write.
func deepChainResultNextNet(
	t *testing.T,
	loop recurrenceLoop,
	sel *selected,
) *netlistNet {
	t.Helper()
	for _, state := range loop.states {
		if state.phi == loop.result {
			return selectedNetBySSAName(t, sel, state.next.Name())
		}
	}
	t.Fatal("deep-chain result state not found")
	return nil
}

// selectedClockNet finds the raw pulse net so recurrence warm-up can be
// compared with the shared clock.
func selectedClockNet(t *testing.T, sel *selected) *netlistNet {
	t.Helper()
	for _, net := range sel.nets {
		if net.driver.inst.comp.kind() == "clockDiv" &&
			net.driver.spec.name == "pulse" {
			return net
		}
	}
	t.Fatal("clock net not found")
	return nil
}

type deepChainTiming struct {
	firstRaw   int
	settled    int
	firstGated int
	firstWrite int
	result     int
}

// measureDeepChainTiming records causal timing landmarks that distinguish a
// correct recurrence warm-up from an accidentally correct result.
func measureDeepChainTiming(
	t *testing.T,
	e *emitter,
	sel *selected,
	resultNext *netlistNet,
	raw *netlistNet,
	gated *netlistNet,
) deepChainTiming {
	t.Helper()
	s := newSim(e.wires)
	timing := deepChainTiming{}
	for tick := 1; tick <= 2*clockPeriod+6; tick++ {
		s.advance(e.entities)
		if timing.firstRaw == 0 &&
			readSelectedNet(t, s, e, raw) != 0 {
			timing.firstRaw = tick
		}
		if timing.settled == 0 &&
			readSelectedNet(t, s, e, resultNext) == 15 {
			timing.settled = tick
		}
		if timing.firstGated == 0 &&
			readSelectedNet(t, s, e, gated) != 0 {
			timing.firstGated = tick
		}
		timing.result = readSelectedNet(t, s, e, sel.resultNet)
		if timing.firstWrite == 0 && timing.result != 5 {
			timing.firstWrite = tick
		}
	}
	return timing
}

// TestGenerateFibonacciSignalBudget names all ten public nets after Clock and
// proves register offsets consume no allocated signal.
func TestGenerateFibonacciSignalBudget(t *testing.T) {
	t.Parallel()
	sel := selectLoopForTest(t, "../testdata/fib.go", "fib", 10)
	require.Same(t, selectedNetBySSAName(t, sel, "t0"), sel.resultNet)
	gates := 0
	for _, in := range sel.insts {
		if _, ok := in.comp.(*stopGate); ok {
			gates++
		}
	}
	require.Equal(t, 1, gates)
	gated := selectedStopGate(t, sel).port("gated").net
	require.Len(t, gated.readers, 3)
	for _, reader := range gated.readers {
		require.Equal(t, "pulse", reader.spec.name)
		_, ok := reader.inst.comp.(*register)
		require.True(t, ok)
	}

	ssaRoles := map[string]string{
		"t0": "previous",
		"t1": "current",
		"t2": "index",
		"t4": "previous plus current",
		"t5": "index plus one",
	}
	roles := make([]string, 0, len(sel.nets))
	for _, net := range sel.nets {
		role := ssaRoles[net.ssaName]
		switch {
		case role != "":
		case net.isInput && net.inputIndex == 0:
			role = "bound"
		case net.litLabel == "1":
			role = "literal one"
		case net.driver.inst.comp.kind() == "stopGate":
			role = "gated pulse"
		case net.driver.inst.comp.kind() == "clockDiv" &&
			net.driver.spec.name == "pulse":
			role = "raw clock pulse"
		case net.driver.inst.comp.kind() == "clockDiv" &&
			net.driver.spec.name == "start":
			role = "clock start"
		}
		require.NotEmpty(t, role, "unclassified public net")
		roles = append(roles, role)
	}
	require.ElementsMatch(t, []string{
		"bound",
		"literal one",
		"index",
		"index plus one",
		"previous",
		"current",
		"previous plus current",
		"gated pulse",
		"raw clock pulse",
		"clock start",
	}, roles)

	require.NoError(t, allocateSignals(sel.nets))
	for _, net := range sel.nets {
		require.NotEqual(t, privateData, net.signal)
		require.NotEqual(t, privateTmp, net.signal)
		require.NotEqual(t, privateInc, net.signal)
	}
}

// TestGenerateFibonacciValidates checks the recurrence blueprint imports in
// factorio-draftsman.
func TestGenerateFibonacciValidates(t *testing.T) {
	t.Parallel()
	generate(
		t,
		"../testdata/fib.go",
		"fib",
		WithParams(map[string]int{"n": 10}),
	).validateWithDraftsman()
}

// TestLoopTerminatesExactly guards the startup off-by-one: a terminating loop
// must run exactly n times for small bounds, where one stray clock pulse skews
// the result. n=1 is the sharp case (an extra iteration settles on 4, not 2). It
// depends on the clock's clean single-tick first pulse (clockdiv.go) and the
// simulator modelling constants as always-on (sim_test.go); either defect alone
// reproduced the in-game 4.
func TestLoopTerminatesExactly(t *testing.T) {
	t.Parallel()
	for _, n := range []int{1, 2, 3, 5} {
		got := simulateLoopSettles(t, "../testdata/fori.go", "forI", n)
		require.Equalf(t, 2*n, got, "forI(%d) must settle on %d", n, 2*n)
	}
}

// TestFastClockLoopShapesSettle proves fast mode preserves both scalar loops
// and the deepest recurrence shape in the checked-in runtime coverage.
func TestFastClockLoopShapesSettle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path, function string
		n, want              int
	}{
		{
			name: "scalar loop", path: "../testdata/fori.go",
			function: "forI", n: 5, want: 10,
		},
		{
			name: "Fibonacci recurrence", path: "../testdata/fib.go",
			function: "fib", n: 10, want: 55,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := simulateLoopSettles(
				t,
				tc.path,
				tc.function,
				tc.n,
				WithFastClock(),
			)
			require.Equal(t, tc.want, got)
		})
	}

	path := writeTempGo(t, deepChainSource)
	require.Equal(t, 15, simulateLoopSettles(
		t,
		path,
		"deepChain",
		1,
		WithFastClock(),
	))
}

// TestFastRecurrenceUsesSelectedTiming proves the recurrence warm-up and
// generated clock use the same fast period.
func TestFastRecurrenceUsesSelectedTiming(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, "../testdata/fib.go", "fib")
	sel, err := selectFunc(fn, WithFastClock())
	require.NoError(t, err)

	gate := selectedStopGate(t, sel)
	gateComponent, ok := gate.comp.(*stopGate)
	require.True(t, ok)
	require.Equal(t, fastClockPeriod, gateComponent.warmupTicks)
	clockPhase(sel)
	clock, ok := selectedClockNet(t, sel).driver.inst.comp.(*clockDiv)
	require.True(t, ok)
	require.Equal(t, fastClockPeriod, clock.period)
	require.Equal(t, "loop clock (4 Hz)", clock.summary)
	require.Equal(
		t,
		fastClockPeriod-2,
		clockedStateSettleBudgetFor(fastClockPeriod),
	)
}

// TestGenerateForIValidates checks the forI blueprint imports in draftsman.
func TestGenerateForIValidates(t *testing.T) {
	t.Parallel()
	generate(t, "../testdata/fori.go", "forI").validateWithDraftsman()
}

// TestClockPinnedLeftAsSummaryUnit proves the clock is a compact unit pinned to
// a reserved leftmost column with a single "1 Hz" summary label, kept out of the
// dependency-layer logic flow: the function's combinators lay out to its right
// and carry no per-combinator clock labels.
func TestClockPinnedLeftAsSummaryUnit(t *testing.T) {
	t.Parallel()
	g, err := compileFunction(parseTestFile(t, "../testdata/fori.go", "forI"))
	require.NoError(t, err)
	assertClockSummaryLabels(t, g.e.entities)
	clockMaxX, logicMinX := clockAndLogicBounds(g.e)
	assert.Less(
		t,
		clockMaxX,
		logicMinX,
		"the clock is pinned left of the logic flow",
	)
}

// assertClockSummaryLabels keeps clock infrastructure readable as one unit.
func assertClockSummaryLabels(t *testing.T, entities []entity) {
	t.Helper()
	summary := 0
	for _, ent := range entities {
		if ent.Name != displayPanelName {
			continue
		}
		if ent.Text == "loop clock (1 Hz)" {
			summary++
		}
		assert.NotContains(t, ent.Text, "clock +", "no per-combinator clock label")
		assert.NotContains(t, ent.Text, "clock %", "no per-combinator clock label")
		assert.NotEqual(t, "pulse", ent.Text, "no per-combinator clock label")
	}
	require.Equal(t, 1, summary, "the clock carries exactly one summary label")
}

// clockAndLogicBounds separates infrastructure from programme logic so their
// relative placement can be asserted without fixed coordinates.
func clockAndLogicBounds(e *emitter) (float64, float64) {
	clockMaxX, logicMinX := -1.0, 1e9
	for _, ent := range e.entities {
		owner := e.owner[ent.EntityNumber]
		if owner == nil {
			continue
		}
		if _, isClock := owner.comp.(*clockDiv); isClock {
			clockMaxX = max(clockMaxX, ent.Position.X)
			continue
		}
		if isLogicCombinator(ent.Name) {
			logicMinX = min(logicMinX, ent.Position.X)
		}
	}
	return clockMaxX, logicMinX
}

// isLogicCombinator excludes panels and power entities from layout bounds.
func isLogicCombinator(name string) bool {
	switch name {
	case arithCombinatorName,
		deciderCombinatorName,
		constCombinatorName:
		return true
	default:
		return false
	}
}

// TestGenerateWide proves Route inserts relay poles for spans past the 9-tile
// reach: wide's params reach late consumers across several columns. The pipeline
// (including the post-Emit reach Verify) must pass, relays must appear, and the
// result must still compute a+b+c+d through the relays (params default to 1).
func TestGenerateWide(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, "../testdata/wide.go", "wide")
	g, err := compileFunction(fn)
	require.NoError(t, err)
	poles := 0
	for _, e := range g.e.entities {
		require.NotEqual(t, "big-electric-pole", e.Name)
		if e.Name == relayPoleEntityName {
			poles++
		}
	}
	require.Positive(t, poles, "wide should need relay poles")
	s := simulate(t, g.e.entities, g.e.wires, 100)
	b := g.e.bound[g.resultNet.driver]
	require.Equal(t, 4, s.value(int(b.h), b.conn, g.resultNet.signal.Name))
}

// TestGenerateWideValidates checks the wide blueprint (with relays) imports in
// draftsman.
func TestGenerateWideValidates(t *testing.T) {
	t.Parallel()
	generate(t, "../testdata/wide.go", "wide").validateWithDraftsman()
}
