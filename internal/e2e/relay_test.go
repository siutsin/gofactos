// This file verifies a long circuit network through its relay pole.
package e2e

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// runWideRelayCase verifies the wide fixture's complete routed sum in the real
// circuit network.
func runWideRelayCase(
	t *testing.T,
	server *factorioServer,
	testCase blueprintCase,
	want int,
) {
	t.Helper()
	const surface = "gofactos-e2e-wide-relay"
	deferred, remaining := setupCaseSurface(
		t,
		server,
		surface,
		testCase,
		"",
	)
	require.Zero(t, deferred)
	require.Zero(t, remaining)
	waitForState(
		t,
		server,
		surface,
		10*time.Second,
		func(state circuitState) bool { return state.display == want },
	)
	assertDisplayStableFor(t, server, surface, want)
}
