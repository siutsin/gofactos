// This file runs the generated-blueprint suite in Factorio.
package e2e

import (
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	// The recursive machine publishes its state on signal-M. These values
	// mirror recursiveModeDone and recursiveModeOverflow in internal/factorio.
	recursiveDoneMode     = 6
	recursiveOverflowMode = 7

	// Factorio normally advances at 60 ticks per second. The E2E server runs
	// at 100x speed, while --fast emits one circuit pulse every 15 ticks.
	e2eGameSpeed  = 100
	e2eFastPeriod = 15
	// Four fast-clock periods distinguish a stable state from a transient one.
	e2eStableTicks = 4 * e2eFastPeriod
	// RCON polling is wall-clock throttling, independent of simulation speed.
	e2ePollInterval = 20 * time.Millisecond

	// factorial(11) exercises ten suspended calls without overflowing int32 or
	// the recursive machine's depth of twelve.
	e2eFactorialN      = 11
	e2eFactorialResult = 39_916_800
	// forI(10) adds two on each iteration.
	e2eScalarN         = 10
	e2eScalarResult    = 20
	e2eScalarBodyAdder = "scalar-body-adder"
	// Numeric results use eight decimal display panels.
	e2eDisplayDigits = 8

	// signedArithmetic(-7) applies Go's truncating -7/3 and -7%3 rules.
	e2eSignedInput     = -7
	e2eDivisionResult  = -2
	e2eRemainderResult = -1
	// fib(10) reaches 55 after enough iterations to exercise recurrence state.
	e2eRecurrenceN      = 10
	e2eRecurrenceResult = 55
	// gcd(48, 18) walks four frames, exercising two recursive arguments,
	// remainder, cross-frame argument ordering, and a tail-position call.
	e2eGCDInputA = 48
	e2eGCDInputB = 18
	e2eGCDResult = 6
	// These inputs take the true and false arms of isEven respectively.
	e2eBooleanTrueInput  = 4
	e2eBooleanFalseInput = 7
	// wide(1, 2, 3, 4) routes the fourth input through a relay pole.
	e2eWideInputA = 1
	e2eWideInputB = 2
	e2eWideInputC = 3
	e2eWideInputD = 4
	e2eWideResult = e2eWideInputA + e2eWideInputB +
		e2eWideInputC + e2eWideInputD
	// reachesZero(5) carries a true result back through six recursive frames.
	e2eReachesZeroN = 5
	// reachesZero(-1) returns a negative integer from the recursive machine.
	e2eNegativeRecursiveN = -1
	// factorial(14) needs thirteen nested calls, exceeding the depth of twelve.
	e2eOverflowN = 14
	// Circuit Booleans are encoded as 1 for true and -1 for false.
	e2eTrueValue  = 1
	e2eFalseValue = -1
	// Factorio arithmetic is signed int32, so math.MaxInt32 + 1 wraps.
	e2eWrapAddend = 1
	// Two runs exercise one complete OFF/reset/rerun cycle.
	e2eResetCycles = 2
)

// TestBlueprintsInFactorio runs generated circuits in Factorio's real engine
// across clocks, construction orders, resets, displays, terminal states,
// multi-argument and Boolean recursion, relay routing, and signed arithmetic.
func TestBlueprintsInFactorio(t *testing.T) {
	if os.Getenv(factorioE2EEnv) != "1" {
		t.Skip("set GOFACTOS_FACTORIO_E2E=1 or run make test-e2e")
	}
	paths := resolveE2EPaths(t)
	// Generate every fixture through the CLI before paying the Factorio startup
	// cost, so a compiler failure stops the test without launching the server.
	fastCase := generateCase(
		t, paths, "recursive/factorial.go", "factorial",
		e2eFactorialN, true,
	)
	fastScalarCase := generateCase(
		t, paths, "fori.go", "forI", e2eScalarN, true,
	)
	fastRecurrenceCase := generateCase(
		t, paths, "fib.go", "fib", e2eRecurrenceN, true,
	)
	signedCase := generateCase(
		t, paths, "e2e/signedarith.go", "signedArithmetic",
		e2eSignedInput, false,
	)
	zeroCase := generateCase(
		t, paths, "fibonacci.go", "fibonacci", 0, true,
	)
	gcdCase := generateCaseWithSets(
		t, paths, "recursive/gcd.go", "gcd", true,
		"a="+strconv.Itoa(e2eGCDInputA),
		"b="+strconv.Itoa(e2eGCDInputB),
	)
	trueCase := generateCase(
		t, paths, "iseven.go", "isEven", e2eBooleanTrueInput, false,
	)
	falseCase := generateCase(
		t, paths, "iseven.go", "isEven", e2eBooleanFalseInput, false,
	)
	wideCase := generateCaseWithSets(
		t,
		paths,
		"wide.go",
		"wide",
		false,
		"a="+strconv.Itoa(e2eWideInputA),
		"b="+strconv.Itoa(e2eWideInputB),
		"c="+strconv.Itoa(e2eWideInputC),
		"d="+strconv.Itoa(e2eWideInputD),
	)
	recursiveBooleanCase := generateCase(
		t,
		paths,
		"recursive/reacheszero.go",
		"reachesZero",
		e2eReachesZeroN,
		true,
	)
	negativeRecursiveCase := generateCase(
		t,
		paths,
		"recursive/reacheszero.go",
		"reachesZero",
		e2eNegativeRecursiveN,
		true,
	)
	overflowCase := generateCase(
		t, paths, "recursive/factorial.go", "factorial", e2eOverflowN, true,
	)
	wrapCase := generateCaseWithSets(
		t,
		paths,
		"add.go",
		"add",
		false,
		"a="+strconv.Itoa(math.MaxInt32),
		"b="+strconv.Itoa(e2eWrapAddend),
	)
	// Keep one sequential server: startup is expensive, and the DONE monitor is
	// process-global state that parallel subtests would overwrite.
	server := startFactorio(t, paths)
	require.Equal(t, "monitor=ready", server.run(t, installDoneMonitorLua()))
	require.Equal(
		t,
		"speed="+strconv.Itoa(e2eGameSpeed),
		server.run(t, setGameSpeedLua()),
	)

	// Delay constants and state cells in turn to cover two partial-construction
	// shapes before START is allowed to advance the recursive machine.
	for _, schedule := range []struct {
		name      string
		deferName string
		testCase  blueprintCase
		period    int
		cycles    int
	}{
		{
			name: "constants last fast", deferName: "constant-combinator",
			testCase: fastCase, period: e2eFastPeriod,
			cycles: e2eResetCycles,
		},
		{
			name:      "state cells last fast",
			deferName: "arithmetic-combinator", testCase: fastCase,
			period: e2eFastPeriod, cycles: 1,
		},
	} {
		t.Run(schedule.name, func(t *testing.T) {
			runConstructionSchedule(
				t,
				server,
				schedule.testCase,
				schedule.deferName,
				schedule.period,
				schedule.cycles,
			)
		})
	}

	// Scalar loops use a different readiness gate, so defer their body adder
	// separately from the recursive construction cases above.
	t.Run("scalar body adder last", func(t *testing.T) {
		runScalarConstructionSchedule(t, server, fastScalarCase)
	})

	// Simultaneous phi updates stress recurrence state; a second cycle proves
	// OFF clears that state before the rerun.
	t.Run("fast recurrence", func(t *testing.T) {
		runCompleteClockedCase(
			t,
			server,
			"gofactos-e2e-fast-recurrence",
			fastRecurrenceCase,
			e2eRecurrenceResult,
			e2eResetCycles,
		)
	})

	// Read the source arithmetic cells directly because the decimal display is
	// not a sufficient oracle for signed division and remainder.
	t.Run("signed arithmetic", func(t *testing.T) {
		runSignedArithmeticCase(t, server, signedCase)
	})

	// Factorio omits zero-valued signals, so DONE must distinguish success from
	// a machine that never started.
	t.Run("zero recursive result", func(t *testing.T) {
		runZeroRecursiveCase(t, server, zeroCase)
	})

	// The state query must not clamp a negative-only signal to zero.
	t.Run("negative recursive result", func(t *testing.T) {
		runNegativeRecursiveCase(
			t,
			server,
			negativeRecursiveCase,
			e2eNegativeRecursiveN,
		)
	})

	// Two arguments drive cross-frame ordering, remainder passing, and a tail
	// call through the real engine, which single-argument factorial cannot.
	t.Run("multi-argument recursion", func(t *testing.T) {
		runMultiArgRecursiveCase(
			t, server, "gofactos-e2e-gcd-recursive", gcdCase, e2eGCDResult,
		)
	})

	// The fourth input crosses a relay pole before the sum reaches the display.
	t.Run("wide relay", func(t *testing.T) {
		runWideRelayCase(t, server, wideCase, e2eWideResult)
	})

	// A true result must survive recursive return wiring, drive the Boolean
	// display, and clear with the recursive status panels on reset.
	t.Run("recursive Boolean result", func(t *testing.T) {
		runRecursiveBooleanCase(
			t,
			server,
			recursiveBooleanCase,
			e2eTrueValue,
		)
	})

	// Even and odd inputs cover both branch/phi arms and opposite display icons.
	for _, test := range []struct {
		name     string
		testCase blueprintCase
		want     int
	}{
		{
			name: "boolean branch true", testCase: trueCase,
			want: e2eTrueValue,
		},
		{
			name: "boolean branch false", testCase: falseCase,
			want: e2eFalseValue,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runBooleanDisplayCase(t, server, test.testCase, test.want)
		})
	}

	// A thirteenth nested call must enter a visible terminal overflow state.
	t.Run("recursive stack overflow", func(t *testing.T) {
		runRecursiveOverflowCase(t, server, overflowCase)
	})

	// MaxInt32 + 1 checks Factorio's native signed-int32 wrapping directly.
	t.Run("signed int32 wraparound", func(t *testing.T) {
		runInt32WrapCase(t, server, wrapCase)
	})
}
