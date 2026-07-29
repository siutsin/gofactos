// This file verifies Factorio's signed arithmetic behaviour.
package e2e

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type signedArithmeticState struct {
	tick       uint64
	division   int
	divisions  int
	remainder  int
	remainders int
}

type arithmeticOperationState struct {
	tick       uint64
	value      int
	operations int
}

// runSignedArithmeticCase verifies Go's signed division and remainder in
// the output of the real Factorio arithmetic combinators.
func runSignedArithmeticCase(
	t *testing.T,
	server *factorioServer,
	testCase blueprintCase,
) {
	t.Helper()
	const surface = "gofactos-e2e-signed-arithmetic"
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		"",
	)
	require.Zero(t, deferred)
	require.Zero(t, remaining)

	waitForSignedArithmetic(
		t,
		server,
		surface,
		e2eDivisionResult,
		e2eRemainderResult,
	)
	state := queryState(t, server, surface)
	require.Positive(t, state.stack)
	require.False(t, isEmpty(state))
	output := server.run(t, disableSignedArithmeticDivisorsLua(surface))
	require.Equal(t, "divisor_sources=2", output)
	waitForSignedArithmetic(t, server, surface, 0, 0)
}

// runInt32WrapCase verifies Factorio's raw arithmetic result wraps at int32.
func runInt32WrapCase(
	t *testing.T,
	server *factorioServer,
	testCase blueprintCase,
) {
	t.Helper()
	const surface = "gofactos-e2e-int32-wrap"
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		"",
	)
	require.Zero(t, deferred)
	require.Zero(t, remaining)
	waitForArithmeticOperation(t, server, surface, "+", math.MinInt32)
}

// waitForArithmeticOperation requires one source operation to remain stable.
func waitForArithmeticOperation(
	t *testing.T,
	server *factorioServer,
	surface string,
	operation string,
	want int,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var state arithmeticOperationState
	var settledTick uint64
	settled := false
	for time.Now().Before(deadline) {
		state = queryArithmeticOperation(t, server, surface, operation)
		matches := state.operations == 1 && state.value == want
		if matches {
			if !settled {
				settled = true
				settledTick = state.tick
			}
			if state.tick-settledTick >= e2eStableTicks {
				return
			}
		} else if settled {
			t.Fatalf("arithmetic operation became unstable: %+v", state)
		}
		time.Sleep(e2ePollInterval)
	}
	t.Fatalf("arithmetic operation did not settle: %+v", state)
}

// queryArithmeticOperation reads one signal-to-signal arithmetic result.
func queryArithmeticOperation(
	t *testing.T,
	server *factorioServer,
	surface string,
	operation string,
) arithmeticOperationState {
	t.Helper()
	output := server.run(t, queryArithmeticOperationLua(surface, operation))
	var state arithmeticOperationState
	_, err := fmt.Sscanf(
		output,
		"tick=%d value=%d operations=%d",
		&state.tick,
		&state.value,
		&state.operations,
	)
	require.NoError(t, err, output)
	return state
}

// waitForSignedArithmetic requires exactly one source division and remainder
// to remain stable for four fast-clock periods.
func waitForSignedArithmetic(
	t *testing.T,
	server *factorioServer,
	surface string,
	wantDivision int,
	wantRemainder int,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var state signedArithmeticState
	var settledTick uint64
	settled := false
	for time.Now().Before(deadline) {
		state = querySignedArithmetic(t, server, surface)
		matches := state.divisions == 1 && state.remainders == 1 &&
			state.division == wantDivision &&
			state.remainder == wantRemainder
		if matches {
			if !settled {
				settled = true
				settledTick = state.tick
			}
			if state.tick-settledTick >= e2eStableTicks {
				return
			}
		} else if settled {
			t.Fatalf("signed arithmetic became unstable: %+v", state)
		}
		time.Sleep(e2ePollInterval)
	}
	t.Fatalf("signed arithmetic did not settle: %+v", state)
}

// querySignedArithmetic reads the source-level division and remainder cells.
func querySignedArithmetic(
	t *testing.T,
	server *factorioServer,
	surface string,
) signedArithmeticState {
	t.Helper()
	output := server.run(t, querySignedArithmeticLua(surface))
	var state signedArithmeticState
	_, err := fmt.Sscanf(
		output,
		"tick=%d division=%d divisions=%d remainder=%d remainders=%d",
		&state.tick,
		&state.division,
		&state.divisions,
		&state.remainder,
		&state.remainders,
	)
	require.NoError(t, err, output)
	return state
}

// queryArithmeticOperationLua reads one source-level arithmetic operation.
func queryArithmeticOperationLua(surface, operation string) string {
	return luaCommand(
		"operation.lua",
		luaString("surface_name", surface),
		luaString("operation", operation),
	)
}

// querySignedArithmeticLua reads only source operations with signal operands,
// excluding the display's constant division and remainder cells.
func querySignedArithmeticLua(surface string) string {
	return luaCommand("signed.lua", luaString("surface_name", surface))
}

// disableSignedArithmeticDivisorsLua turns both literal divisor sources off.
func disableSignedArithmeticDivisorsLua(surface string) string {
	return luaCommand("disabledivisors.lua", luaString("surface_name", surface))
}
