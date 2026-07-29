// This file protects multi-file loading and explicit root selection.
package integration

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLoaderBlueprintJSON verifies --func selection through the blueprint
// command.
// loader.go holds two functions, double (n * 2) and triple (n * 3), so the
// blueprint command needs --func to choose one. Each selection compiles only the
// named function to an otherwise identical layout that differs solely in the
// literal multiplier and its teaching label.
func TestLoaderBlueprintJSON(t *testing.T) {
	for _, tc := range []struct {
		fn   string
		mult int
	}{
		{"double", 2},
		{"triple", 3},
	} {
		t.Run(tc.fn, func(t *testing.T) {
			t.Parallel()
			doc := runBlueprintJSON(t, "loader-"+tc.fn)

			assertBlueprintHeader(t, doc, tc.fn, 31, 33)

			assertLoaderPanelDisplays(t, doc, tc.mult)
			assertLoaderControlBehaviour(t, doc, tc.mult)
			assertNoRelayPoles(t, doc)
			assertSingleSubstationPower(t, doc, 31, 17)
			assertLoaderDigitDisplay(t, doc)
		})
	}
}

// assertLoaderPanelDisplays checks the teaching labels: the parameter, the
// multiply, and the literal multiplier.
func assertLoaderPanelDisplays(t *testing.T, doc blueprintJSON, mult int) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 1", "t0 = A * c0", fmt.Sprintf("c0 = %d", mult),
	}, displayPanelTexts(doc))
}

// assertLoaderControlBehaviour checks the parameter source, the literal
// multiplier, and the single multiply combinator.
func assertLoaderControlBehaviour(t *testing.T, doc blueprintJSON, mult int) {
	t.Helper()
	e := entitiesByNumber(t, doc)
	assertConstant(t, e[1], "signal-A", 1)
	assertConstant(t, e[3], "iron-plate", mult)
	assertArithmeticSignals(t, e[2], "signal-A", "*", "iron-plate", "copper-plate")
}

// assertLoaderDigitDisplay checks the eight-stage numeric readout reads the
// result on its item signal.
func assertLoaderDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 4, "copper-plate")
}
