// This file protects Factorio-compatible division-by-zero lowering.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDivZeroBlueprintJSON verifies divZero JSON through the blueprint command.
// divZero returns a / (a - a): Go would panic, but gofactos lowers SSA to a
// static circuit and Factorio defines division by zero as 0. The divisor is a
// runtime net (the a - a subtract), not a baked constant, so this test pins that
// the divide reads the subtract's output as its second operand.
func TestDivZeroBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "divzero")

	assertBlueprintHeader(t, doc, "divZero", 31, 34)

	assertDivZeroPanelDisplays(t, doc)
	assertDivZeroControlBehaviour(t, doc)
	assertNoRelayPoles(t, doc)
	assertSingleSubstationPower(t, doc, 31, 18)
	assertDivZeroDigitDisplay(t, doc)
}

// assertDivZeroPanelDisplays checks the teaching labels: the parameter, the
// subtract that makes the zero divisor, and the divide.
func assertDivZeroPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{"A = 1", "t0 = A - A", "t1 = A / t0"}, displayPanelTexts(doc))
}

// assertDivZeroControlBehaviour checks the parameter, the subtract that yields
// the zero divisor, and the divide that reads it as a runtime net, not a baked 0.
func assertDivZeroControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	entities := entitiesByNumber(t, doc)
	assertConstant(t, entities[1], "signal-A", 1)
	assertArithmeticSignals(t, entities[2], "signal-A", "-", "signal-A", "iron-plate")
	assertArithmeticSignals(t, entities[3], "signal-A", "/", "iron-plate", "copper-plate")
}

// assertDivZeroDigitDisplay checks the eight-stage numeric readout reads the
// quotient on its item signal.
func assertDivZeroDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 4, "copper-plate")
}
