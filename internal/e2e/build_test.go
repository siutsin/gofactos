// This file protects circuits during partial construction.
package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runConstructionSchedule protects recursion against partial construction and
// verifies manual reset after completion.
func runConstructionSchedule(
	t *testing.T,
	server *factorioServer,
	testCase blueprintCase,
	deferName string,
	period int,
	cycles int,
) {
	t.Helper()
	require.Positive(t, cycles)
	surface := "gofactos-e2e-" + strings.ReplaceAll(deferName, "-", "_")
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		deferName,
	)
	require.Positive(t, deferred)
	require.Equal(t, deferred, remaining)

	assertMachineStoppedFor(t, server, surface, true)
	output := server.run(t, reviveGhostsLua(surface))
	var revived int
	_, err := fmt.Sscanf(output, "revived=%d remaining=%d", &revived, &remaining)
	require.NoError(t, err, output)
	require.Positive(t, revived)
	assert.Zero(t, remaining)
	assertMachineStoppedFor(t, server, surface, false)

	for cycle := 1; cycle <= cycles; cycle++ {
		passed := t.Run(fmt.Sprintf("cycle %d", cycle), func(t *testing.T) {
			runRecursiveCycle(
				t,
				server,
				surface,
				e2eFactorialResult,
				period,
			)
			output = server.run(t, setStartLua(surface, false))
			require.Equal(t, "start=off", output)
			assertResetAfterOff(t, server, surface)
		})
		if !passed {
			return
		}
	}
}

// runScalarConstructionSchedule verifies START prevents a partially built
// scalar loop from consuming pulses and that OFF permits a clean rerun.
func runScalarConstructionSchedule(
	t *testing.T,
	server *factorioServer,
	testCase blueprintCase,
) {
	t.Helper()
	const surface = "gofactos-e2e-scalar-construction"
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		e2eScalarBodyAdder,
	)
	require.Equal(t, 1, deferred)
	require.Equal(t, 1, remaining)

	assertClockStoppedFor(t, server, surface, true)
	output := server.run(t, reviveGhostsLua(surface))
	var revived int
	_, err := fmt.Sscanf(
		output,
		"revived=%d remaining=%d",
		&revived,
		&remaining,
	)
	require.NoError(t, err, output)
	require.Equal(t, deferred, revived)
	require.Zero(t, remaining)
	assertClockStoppedFor(t, server, surface, false)

	for cycle := 1; cycle <= 2; cycle++ {
		passed := t.Run(fmt.Sprintf("cycle %d", cycle), func(t *testing.T) {
			runClockedResultCycle(t, server, surface, e2eScalarResult)
			output = server.run(t, setStartLua(surface, false))
			require.Equal(t, "start=off", output)
			assertClockStoppedFor(t, server, surface, false)
		})
		if !passed {
			return
		}
	}
}
