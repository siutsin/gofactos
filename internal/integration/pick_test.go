// This file protects boolean parameters used directly as branch conditions.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPickBlueprintJSON verifies pick JSON through the blueprint command.
// pick(b bool) returns 5 or 3 from a boolean parameter, pinning the 1/-1
// encoding (false is -1) that drives a merge between two literal constants.
func TestPickBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "pick")

	assertBlueprintHeader(t, doc, "pick", 41, 39)

	assertPickPanelDisplays(t, doc)
	assertPickControlBehaviour(t, doc)
	assertNoRelayPoles(t, doc)
	assertSingleSubstationPower(t, doc, 41, 21)
	assertPickDigitDisplay(t, doc)
}

// assertPickPanelDisplays checks the teaching labels: the bool parameter, the two
// normalise steps, the two gates, the merge, and the two literal choices.
func assertPickPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 1", "normalise c0", "normalise c1", "if A { merge = c0 }",
		"if !A { merge = c1 }", "i2 = merge", "c0 = 5", "c1 = 3",
	}, displayPanelTexts(doc))
}

// assertPickControlBehaviour checks the bool parameter and the two constants, and
// that the gates read the parameter as 1 (true) and -1 (false), proving the
// boolean encoding selects the merge branch.
func assertPickControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	e := entitiesByNumber(t, doc)
	assertConstant(t, e[1], "signal-A", 1)
	assertConstant(t, e[7], "iron-plate", 5)
	assertConstant(t, e[8], "copper-plate", 3)
	assertArithmetic(t, e[2], "iron-plate", "*", 1, "signal-dot")
	assertArithmetic(t, e[3], "copper-plate", "*", 1, "signal-dot")
	assertDecider(t, e[4], "signal-A", "=", 1, "signal-dot", true)
	assertDecider(t, e[5], "signal-A", "=", -1, "signal-dot", true)
	assertArithmetic(t, e[6], "signal-dot", "*", 1, "steel-plate")
}

// assertPickDigitDisplay checks the eight-stage numeric readout reads the merged
// choice on its item signal.
func assertPickDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 9, "steel-plate")
}
