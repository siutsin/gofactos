// This file verifies Boolean results and display conditions.
package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type booleanDisplayState struct {
	tick        uint64
	value       int
	panels      int
	checkActive int
	denyActive  int
}

// runBooleanDisplayCase verifies an ordinary phi and its Boolean panel in the
// real circuit network.
func runBooleanDisplayCase(
	t *testing.T,
	server *factorioServer,
	testCase blueprintCase,
	want int,
) {
	t.Helper()
	require.Contains(t, []int{-1, 1}, want)
	suffix := "false"
	if want == 1 {
		suffix = "true"
	}
	surface := "gofactos-e2e-boolean-" + suffix
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		"",
	)
	require.Zero(t, deferred)
	require.Zero(t, remaining)
	waitForBooleanDisplay(t, server, surface, want)
}

// waitForBooleanDisplay requires the selected icon and encoded Boolean value
// to remain stable for four fast-clock periods.
func waitForBooleanDisplay(
	t *testing.T,
	server *factorioServer,
	surface string,
	want int,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var state booleanDisplayState
	var settledTick uint64
	settled := false
	for time.Now().Before(deadline) {
		state = queryBooleanDisplay(t, server, surface)
		if booleanDisplayMatches(state, want) {
			if !settled {
				settled = true
				settledTick = state.tick
			}
			if state.tick-settledTick >= e2eStableTicks {
				return
			}
		} else if settled {
			t.Fatalf("Boolean display became unstable: %+v", state)
		}
		time.Sleep(e2ePollInterval)
	}
	t.Fatalf("Boolean display did not settle: %+v", state)
}

// booleanDisplayMatches checks the panel's encoded value and selected icon.
func booleanDisplayMatches(state booleanDisplayState, want int) bool {
	wantCheck, wantDeny := 0, 1
	if want == 1 {
		wantCheck, wantDeny = 1, 0
	}
	return state.value == want && state.panels == 1 &&
		state.checkActive == wantCheck && state.denyActive == wantDeny
}

// queryBooleanDisplay reads the Boolean result and active message conditions.
func queryBooleanDisplay(
	t *testing.T,
	server *factorioServer,
	surface string,
) booleanDisplayState {
	t.Helper()
	output := server.run(t, queryBooleanDisplayLua(surface))
	var state booleanDisplayState
	_, err := fmt.Sscanf(
		output,
		"tick=%d value=%d panels=%d check=%d deny=%d",
		&state.tick,
		&state.value,
		&state.panels,
		&state.checkActive,
		&state.denyActive,
	)
	require.NoError(t, err, output)
	return state
}

// queryBooleanDisplayLua reads the sole check/deny panel and its live signal.
func queryBooleanDisplayLua(surface string) string {
	return luaCommand(
		"boolean.lua",
		luaString("surface_name", surface),
	)
}
