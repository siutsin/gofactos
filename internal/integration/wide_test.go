// This file protects relay insertion for blueprints with wide inputs.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWideBlueprintJSON verifies wide JSON through the blueprint command. wide
// (a + b + c + d) chains four parameters, stretching a wire past a combinator's
// 9-tile reach, so this test case forces Route to insert a relay pole.
func TestWideBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "wide")

	assertBlueprintHeader(t, doc, "wide", 40, 38)

	assertWidePanelDisplays(t, doc)
	assertWideControlBehaviour(t, doc)
	assertWideElectricity(t, doc)
	assertWideDigitDisplay(t, doc)
}

// assertWidePanelDisplays checks the teaching labels: the four parameters and the
// chained additions.
func assertWidePanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 1", "B = 1", "C = 1", "D = 1", "t0 = A + B", "t1 = t0 + C",
		"t2 = t1 + D",
	}, displayPanelTexts(doc))
}

// assertWideControlBehaviour checks the four parameters and the left-associated
// chain of adders.
func assertWideControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	e := entitiesByNumber(t, doc)
	assertConstant(t, e[1], "signal-A", 1)
	assertConstant(t, e[2], "signal-B", 1)
	assertConstant(t, e[3], "signal-C", 1)
	assertConstant(t, e[4], "signal-D", 1)
	assertArithmeticSignals(t, e[5], "signal-A", "+", "signal-B", "iron-plate")
	assertArithmeticSignals(t, e[6], "iron-plate", "+", "signal-C", "copper-plate")
	assertArithmeticSignals(t, e[7], "copper-plate", "+", "signal-D", "steel-plate")
}

// assertWideElectricity checks wide inserts exactly one relay pole for its
// over-reach wire and that the single substation powers every combinator.
func assertWideElectricity(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertRelayPoleCount(t, doc, 1)
	assertSingleSubstationPower(t, doc, 40, 19)
}

// assertWideDigitDisplay checks the eight-stage numeric readout reads the summed
// result on its item signal.
func assertWideDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 8, "steel-plate")
}
