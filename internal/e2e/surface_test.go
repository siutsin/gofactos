// This file builds and controls isolated Factorio test surfaces.
package e2e

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// setupCaseSurface builds and powers one test case and parses its ghost count.
func setupCaseSurface(
	t *testing.T,
	server *factorioServer,
	surface string,
	testCase blueprintCase,
	deferName string,
) (int, int) {
	t.Helper()
	output := server.run(t, setupSurfaceLua(surface, testCase, deferName))
	var deferred, remaining int
	_, err := fmt.Sscanf(
		output,
		"deferred=%d remaining=%d",
		&deferred,
		&remaining,
	)
	require.NoError(t, err, output)
	return deferred, remaining
}

// setGameSpeedLua accelerates only the isolated E2E server simulation.
func setGameSpeedLua() string {
	return luaCommand("speed.lua", luaInt("speed", e2eGameSpeed))
}

// setupSurfaceLua creates an isolated test surface and applies the requested
// construction deferral.
func setupSurfaceLua(
	surface string,
	testCase blueprintCase,
	deferName string,
) string {
	return luaCommand(
		"setup.lua",
		luaString("surface_name", surface),
		luaString("encoded", testCase.encoded),
		luaString("defer_name", deferName),
		luaBool("defer_scalar_body", deferName == e2eScalarBodyAdder),
	)
}

// reviveGhostsLua completes a deliberately interrupted construction schedule.
func reviveGhostsLua(surface string) string {
	return luaCommand("revive.lua", luaString("surface_name", surface))
}

// setStartLua exercises the clock's player-operated START control.
func setStartLua(surface string, enabled bool) string {
	return luaCommand(
		"start.lua",
		luaString("surface_name", surface),
		luaBool("enabled", enabled),
	)
}
