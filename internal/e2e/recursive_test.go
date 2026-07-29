// This file exercises recursive results, reset, completion, and overflow.
package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type doneObservation struct {
	seen     bool
	display  int
	panels   int
	tick     int
	pulses   int
	cadence  bool
	badDelta int
}

type recursiveStatusState struct {
	running  int
	done     int
	overflow int
}

// runRecursiveCycle verifies one complete recursive execution.
func runRecursiveCycle(
	t *testing.T,
	server *factorioServer,
	surface string,
	want int,
	period int,
) circuitState {
	t.Helper()
	output := server.run(t, prepareDoneMonitorLua(surface, period))
	require.Equal(t, "monitor=armed", output)
	output = server.run(t, setStartLua(surface, true))
	require.Equal(t, "start=on", output)
	state := waitForState(
		t,
		server,
		surface,
		90*time.Second,
		func(state circuitState) bool {
			return state.mode == recursiveDoneMode &&
				state.result == want &&
				state.output == want
		},
	)
	assert.Zero(t, state.ghosts)
	if want < 0 {
		return state
	}
	assert.Equal(t, want, state.display)
	observation := waitForDoneObservation(t, server, 2*time.Second)
	require.Equal(t, want, observation.display)
	require.Equal(t, e2eDisplayDigits, observation.panels)
	if period > 0 {
		require.GreaterOrEqual(t, observation.pulses, 3)
		require.True(
			t,
			observation.cadence,
			"unexpected clock interval: %d ticks",
			observation.badDelta,
		)
	}
	return state
}

// runNegativeRecursiveCase proves the state probe preserves a signed result
// even though the decimal display has no negative-value contract.
func runNegativeRecursiveCase(
	t *testing.T,
	server *factorioServer,
	testCase blueprintCase,
	want int,
) {
	t.Helper()
	const surface = "gofactos-e2e-negative-recursive"
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		"",
	)
	require.Zero(t, deferred)
	require.Zero(t, remaining)
	assertMachineStoppedFor(t, server, surface, false)
	state := runRecursiveCycle(t, server, surface, want, e2eFastPeriod)
	require.Positive(t, state.resultSignals)
	require.Positive(t, state.outputSignals)
}

// runZeroRecursiveCase proves DONE distinguishes successful zero from an
// unstarted machine when Factorio omits the zero-valued result signal.
func runZeroRecursiveCase(
	t *testing.T,
	server *factorioServer,
	testCase blueprintCase,
) {
	t.Helper()
	const surface = "gofactos-e2e-zero-recursive"
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		"",
	)
	require.Zero(t, deferred)
	require.Zero(t, remaining)
	assertMachineStoppedFor(t, server, surface, false)
	state := runRecursiveCycle(t, server, surface, 0, 0)
	require.Zero(t, state.resultSignals)
	require.Zero(t, state.outputSignals)
}

// runMultiArgRecursiveCase drives a multi-argument recursion through the real
// engine and verifies its numeric result, covering cross-frame argument
// ordering, remainder passing, and a tail call that single-argument factorial
// leaves unexercised in Factorio.
func runMultiArgRecursiveCase(
	t *testing.T,
	server *factorioServer,
	surface string,
	testCase blueprintCase,
	want int,
) {
	t.Helper()
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		"",
	)
	require.Zero(t, deferred)
	require.Zero(t, remaining)
	assertMachineStoppedFor(t, server, surface, false)
	runRecursiveCycle(t, server, surface, want, e2eFastPeriod)
}

// runRecursiveBooleanCase verifies a Boolean result crosses recursive frames,
// drives its icon, and clears with the machine's terminal status on reset.
func runRecursiveBooleanCase(
	t *testing.T,
	server *factorioServer,
	testCase blueprintCase,
	want int,
) {
	t.Helper()
	const surface = "gofactos-e2e-recursive-boolean"
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		"",
	)
	require.Zero(t, deferred)
	require.Zero(t, remaining)
	assertMachineStoppedFor(t, server, surface, false)
	assertRecursiveBooleanResetFor(t, server, surface)

	output := server.run(t, setStartLua(surface, true))
	require.Equal(t, "start=on", output)
	waitForBooleanDisplay(t, server, surface, want)
	assertRecursiveStatus(
		t,
		server,
		surface,
		recursiveStatusState{done: 1},
	)

	output = server.run(t, setStartLua(surface, false))
	require.Equal(t, "start=off", output)
	assertResetAfterOff(t, server, surface)
	assertRecursiveBooleanResetFor(t, server, surface)
}

// assertRecursiveBooleanResetFor waits for the Boolean display and recursive
// status panels to clear, then proves they stay clear for four clock periods.
func assertRecursiveBooleanResetFor(
	t *testing.T,
	server *factorioServer,
	surface string,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var display booleanDisplayState
	var status recursiveStatusState
	for time.Now().Before(deadline) {
		display = queryBooleanDisplay(t, server, surface)
		status = queryRecursiveStatus(t, server, surface)
		if recursiveBooleanReset(display, status) {
			break
		}
		time.Sleep(e2ePollInterval)
	}
	require.True(
		t,
		recursiveBooleanReset(display, status),
		"display: %+v, status: %+v",
		display,
		status,
	)

	startTick := display.tick
	for display.tick-startTick < e2eStableTicks {
		if time.Now().After(deadline) {
			t.Fatalf(
				"recursive Boolean reset did not remain stable: "+
					"display: %+v, status: %+v",
				display,
				status,
			)
		}
		time.Sleep(e2ePollInterval)
		display = queryBooleanDisplay(t, server, surface)
		status = queryRecursiveStatus(t, server, surface)
		require.True(
			t,
			recursiveBooleanReset(display, status),
			"display: %+v, status: %+v",
			display,
			status,
		)
	}
}

// recursiveBooleanReset identifies a cleared Boolean display and terminal
// status group.
func recursiveBooleanReset(
	display booleanDisplayState,
	status recursiveStatusState,
) bool {
	return display.value == 0 && display.panels == 1 &&
		display.checkActive == 0 && display.denyActive == 0 &&
		status == (recursiveStatusState{})
}

// runRecursiveOverflowCase verifies overflow is terminal, visible, and resettable.
func runRecursiveOverflowCase(
	t *testing.T,
	server *factorioServer,
	testCase blueprintCase,
) {
	t.Helper()
	const surface = "gofactos-e2e-recursive-overflow"
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		"",
	)
	require.Zero(t, deferred)
	require.Zero(t, remaining)
	assertMachineStoppedFor(t, server, surface, false)
	output := server.run(t, setStartLua(surface, true))
	require.Equal(t, "start=on", output)
	waitForState(
		t,
		server,
		surface,
		30*time.Second,
		isRecursiveOverflowState,
	)
	assertStateForTicks(t, server, surface, isRecursiveOverflowState)
	assertRecursiveStatus(t, server, surface, recursiveStatusState{overflow: 1})

	output = server.run(t, setStartLua(surface, false))
	require.Equal(t, "start=off", output)
	assertResetAfterOff(t, server, surface)
	assertRecursiveStatus(t, server, surface, recursiveStatusState{})
}

// isRecursiveOverflowState identifies a settled machine with no public result.
func isRecursiveOverflowState(state circuitState) bool {
	return state.mode == recursiveOverflowMode &&
		state.result == 0 && state.output == 0 && state.display == 0 &&
		state.resultSignals == 0 && state.outputSignals == 0 &&
		state.activity == 0 && state.ghosts == 0 && state.stack > 0
}

// assertRecursiveStatus checks the three user-visible recursive status panels.
func assertRecursiveStatus(
	t *testing.T,
	server *factorioServer,
	surface string,
	want recursiveStatusState,
) {
	t.Helper()
	require.Equal(t, want, queryRecursiveStatus(t, server, surface))
}

// queryRecursiveStatus reads which recursive status panel conditions are true.
func queryRecursiveStatus(
	t *testing.T,
	server *factorioServer,
	surface string,
) recursiveStatusState {
	t.Helper()
	output := server.run(t, queryRecursiveStatusLua(surface))
	var state recursiveStatusState
	_, err := fmt.Sscanf(
		output,
		"running=%d done=%d overflow=%d",
		&state.running,
		&state.done,
		&state.overflow,
	)
	require.NoError(t, err, output)
	return state
}

// waitForDoneObservation catches the DONE tick even when later state changes.
func waitForDoneObservation(
	t *testing.T,
	server *factorioServer,
	timeout time.Duration,
) doneObservation {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var observation doneObservation
	for time.Now().Before(deadline) {
		output := server.run(t, queryDoneMonitorLua())
		_, err := fmt.Sscanf(
			output,
			"seen=%t display=%d panels=%d tick=%d pulses=%d "+
				"cadence=%t bad_delta=%d",
			&observation.seen,
			&observation.display,
			&observation.panels,
			&observation.tick,
			&observation.pulses,
			&observation.cadence,
			&observation.badDelta,
		)
		require.NoError(t, err, output)
		if observation.seen {
			return observation
		}
		time.Sleep(e2ePollInterval)
	}
	t.Fatalf("Factorio did not observe DONE: %+v", observation)
	return doneObservation{}
}

// installDoneMonitorLua records the display on the exact tick DONE appears.
func installDoneMonitorLua() string {
	return luaCommand(
		"doneinstall.lua",
		luaInt("done_mode", recursiveDoneMode),
	)
}

// prepareDoneMonitorLua resets the completion and clock-cadence monitor.
func prepareDoneMonitorLua(surface string, period int) string {
	return luaCommand(
		"doneprepare.lua",
		luaString("surface_name", surface),
		luaInt("period", period),
	)
}

// queryDoneMonitorLua exposes the captured completion tick over RCON.
func queryDoneMonitorLua() string {
	return luaCommand("donequery.lua")
}

// queryRecursiveStatusLua reads the active recursive status panel conditions.
func queryRecursiveStatusLua(surface string) string {
	return luaCommand(
		"recursivestatus.lua",
		luaString("surface_name", surface),
	)
}
