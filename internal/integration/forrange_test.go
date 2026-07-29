// This file protects range-loop generation through the blueprint command.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestForRangeBlueprintJSON verifies forRange JSON through the blueprint
// command.
// forRange (`for range n`) is the loop test case: one clock, an index register
// and a result register (each a loop-carried phi), a stop gate, and the numeric
// readout. forRange compiles from a self-loop SSA shape but lowers to the same
// blueprint as the counted forI (see TestForIAndForRangeProduceSameBlueprint).
func TestForRangeBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "forrange")

	assertBlueprintHeader(t, doc, "forRange", 54, 61)

	assertClockStartControl(t, doc)
	assertForRangePanelDisplays(t, doc)
	assertForRangeControlBehaviour(t, doc)
	assertForRangeElectricity(t, doc)
	assertForRangeDigitDisplay(t, doc)
}

// assertForRangePanelDisplays checks the loop's teaching labels: the clock, the
// bound, both loop-carried phi nodes with their next-value adders, the stop
// gate, and the two increment constants.
func assertForRangePanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"loop clock (1 Hz)", clockStartControlLabel, "A = 1",
		"i1 = φ(0, i1 + c0)", "i2 = i1 + c0", "c0 = 1",
		"run while i1 < A", "i5 = φ(0, i5 + c1)",
		"i6 = i5 + c1", "c1 = 2",
	}, displayPanelTexts(doc))
}

// assertForRangeControlBehaviour checks the loop's inputs: the bound n and the
// two phi increments (c0 steps the index, c1 steps the result).
func assertForRangeControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	entities := entitiesByNumber(t, doc)
	assertConstant(t, entities[1], "signal-A", 1)
	assertConstant(t, entities[6], "iron-plate", 1)
	assertConstant(t, entities[13], "copper-cable", 2)
}

// assertForRangeElectricity checks the loop needs two relay poles for its taller
// layout and that the single substation powers every combinator.
func assertForRangeElectricity(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertRelayPoleCount(t, doc, 2)
	assertSingleSubstationPower(t, doc, 54, 29)
}

// assertForRangeDigitDisplay checks the eight-stage numeric readout reads the
// loop result on its item signal.
func assertForRangeDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 14, "electronic-circuit")
}
