// This file checks shared circuit state and reset invariants.
package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type circuitState struct {
	tick          uint64
	mode          int
	result        int
	output        int
	display       int
	modeSignals   int
	resultSignals int
	outputSignals int
	stack         int
	panels        int
	activity      int
	ghosts        int
	disabled      int
}

// runCompleteClockedCase verifies a complete clocked test case waits for
// START and reaches the same stable result across the requested clean cycles.
func runCompleteClockedCase(
	t *testing.T,
	server *factorioServer,
	surface string,
	testCase blueprintCase,
	want int,
	cycles int,
) {
	t.Helper()
	require.Positive(t, cycles)
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		"",
	)
	require.Zero(t, deferred)
	require.Zero(t, remaining)
	assertClockStoppedFor(t, server, surface, false)
	for cycle := 1; cycle <= cycles; cycle++ {
		runClockedResultCycle(t, server, surface, want)
		if cycle == cycles {
			continue
		}
		output := server.run(t, setStartLua(surface, false))
		require.Equal(t, "start=off", output)
		assertClockStoppedFor(t, server, surface, false)
	}
}

// runClockedResultCycle starts a clocked test case and verifies its stable
// visible result.
func runClockedResultCycle(
	t *testing.T,
	server *factorioServer,
	surface string,
	want int,
) {
	t.Helper()
	output := server.run(t, setStartLua(surface, true))
	require.Equal(t, "start=on", output)
	waitForState(
		t,
		server,
		surface,
		30*time.Second,
		func(state circuitState) bool { return state.display == want },
	)
	assertDisplayStableFor(t, server, surface, want)
}

// assertDisplayStableFor ensures completion stays visible for four fast clock
// periods.
func assertDisplayStableFor(
	t *testing.T,
	server *factorioServer,
	surface string,
	want int,
) {
	t.Helper()
	assertStateForTicks(t, server, surface, func(state circuitState) bool {
		return state.display == want && state.panels == e2eDisplayDigits &&
			state.ghosts == 0
	})
}

// assertMachineStoppedFor ensures recursive state cannot advance before
// START.
func assertMachineStoppedFor(
	t *testing.T,
	server *factorioServer,
	surface string,
	wantGhosts bool,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		state := queryState(t, server, surface)
		require.True(
			t,
			isEmpty(state) && hasExpectedGhosts(state, wantGhosts),
			"state: %+v",
			state,
		)
		if state.activity == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("activity lamps did not clear: %+v", state)
		}
		time.Sleep(e2ePollInterval)
	}
	assertStableResetFor(t, server, surface, wantGhosts)
}

// assertClockStoppedFor ensures a clocked test case stays visibly reset with
// START off, without assuming which public signals its dataflow uses.
func assertClockStoppedFor(
	t *testing.T,
	server *factorioServer,
	surface string,
	wantGhosts bool,
) {
	t.Helper()
	waitForState(
		t,
		server,
		surface,
		10*time.Second,
		func(state circuitState) bool {
			return isClockStopped(state, wantGhosts)
		},
	)
	assertStateForTicks(t, server, surface, func(state circuitState) bool {
		return isClockStopped(state, wantGhosts)
	})
}

// isClockStopped identifies the externally visible clock reset contract.
func isClockStopped(state circuitState, wantGhosts bool) bool {
	return state.display == 0 && state.activity == 0 &&
		hasExpectedGhosts(state, wantGhosts)
}

// assertResetAfterOff ensures switching off returns a completed machine to its
// reusable initial state.
func assertResetAfterOff(
	t *testing.T,
	server *factorioServer,
	surface string,
) {
	t.Helper()
	waitForState(
		t,
		server,
		surface,
		10*time.Second,
		func(state circuitState) bool {
			return isExpectedReset(state, false)
		},
	)
	assertStableResetFor(t, server, surface, false)
}

// assertStableResetFor ensures an idle or reset machine remains quiescent.
func assertStableResetFor(
	t *testing.T,
	server *factorioServer,
	surface string,
	wantGhosts bool,
) {
	t.Helper()
	assertStateForTicks(t, server, surface, func(state circuitState) bool {
		return isExpectedReset(state, wantGhosts)
	})
}

// assertStateForTicks checks an invariant across four complete fast-clock
// periods while retaining a wall-clock deadline only for stalled Factorio.
func assertStateForTicks(
	t *testing.T,
	server *factorioServer,
	surface string,
	valid func(circuitState) bool,
) {
	t.Helper()
	state := queryState(t, server, surface)
	startTick := state.tick
	deadline := time.Now().Add(10 * time.Second)
	for {
		require.True(t, valid(state), "state: %+v", state)
		if state.tick-startTick >= e2eStableTicks {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Factorio ticks did not advance: %+v", state)
		}
		time.Sleep(e2ePollInterval)
		state = queryState(t, server, surface)
	}
}

// isExpectedReset combines machine reset and construction-state expectations.
func isExpectedReset(state circuitState, wantGhosts bool) bool {
	return isReset(state) && hasExpectedGhosts(state, wantGhosts)
}

// hasExpectedGhosts distinguishes partial construction from a complete idle
// machine.
func hasExpectedGhosts(state circuitState, wantGhosts bool) bool {
	if wantGhosts {
		return state.ghosts > 0
	}
	return state.ghosts == 0 && state.disabled == 1
}

// isEmpty identifies a machine whose externally visible values are cleared.
func isEmpty(state circuitState) bool {
	if state.mode != 0 || state.result != 0 ||
		state.output != 0 || state.display != 0 || state.modeSignals != 0 ||
		state.resultSignals != 0 || state.outputSignals != 0 || state.stack != 0 {
		return false
	}
	return true
}

// TestIsEmptyRejectsHiddenCircuitState verifies that negative-only circuit
// values cannot pass reset checks after maximum-value aggregation.
func TestIsEmptyRejectsHiddenCircuitState(t *testing.T) {
	assert.True(t, isEmpty(circuitState{}))
	assert.False(t, isEmpty(circuitState{modeSignals: 1}))
	assert.False(t, isEmpty(circuitState{resultSignals: 1}))
	assert.False(t, isEmpty(circuitState{outputSignals: 1}))
	assert.False(t, isEmpty(circuitState{stack: 1}))
}

// isReset also requires every progress lamp to be inactive.
func isReset(state circuitState) bool {
	return isEmpty(state) && state.activity == 0
}

// waitForState bounds asynchronous Factorio progress with useful final-state
// diagnostics.
func waitForState(
	t *testing.T,
	server *factorioServer,
	surface string,
	timeout time.Duration,
	done func(circuitState) bool,
) circuitState {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var state circuitState
	for time.Now().Before(deadline) {
		state = queryState(t, server, surface)
		if done(state) {
			return state
		}
		time.Sleep(e2ePollInterval)
	}
	t.Fatalf("Factorio state did not settle: %+v", state)
	return circuitState{}
}

// queryState gives Go assertions one machine snapshot without assuming whether
// the result uses numeric or Boolean display panels.
func queryState(
	t *testing.T,
	server *factorioServer,
	surface string,
) circuitState {
	t.Helper()
	output := server.run(t, queryStateLua(surface))
	var state circuitState
	_, err := fmt.Sscanf(
		output,
		"tick=%d mode=%d result=%d output=%d display=%d activity=%d "+
			"mode_signals=%d result_signals=%d output_signals=%d "+
			"stack=%d panels=%d ghosts=%d disabled=%d",
		&state.tick,
		&state.mode,
		&state.result,
		&state.output,
		&state.display,
		&state.activity,
		&state.modeSignals,
		&state.resultSignals,
		&state.outputSignals,
		&state.stack,
		&state.panels,
		&state.ghosts,
		&state.disabled,
	)
	require.NoError(t, err, output)
	return state
}

// queryStateLua aggregates machine, display, lamp, and construction state.
func queryStateLua(surface string) string {
	return luaCommand("state.lua", luaString("surface_name", surface))
}
