// This file protects comparison and phi lowering in the max blueprint.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMaxBlueprintJSON verifies max JSON through the blueprint command. max
// merges two parameters by a `>=` comparison. It is the only test case whose
// merge picks distinct inputs rather than a value and a constant or negation.
func TestMaxBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "max")

	assertBlueprintHeader(t, doc, "max", 45, 44)

	assertMaxPanelDisplays(t, doc)
	assertMaxControlBehaviour(t, doc)
	assertNoRelayPoles(t, doc)
	assertSingleSubstationPower(t, doc, 45, 24)
	assertMaxDigitDisplay(t, doc)
}

// assertMaxPanelDisplays checks the teaching labels: the parameters, the compare,
// the two normalise steps, the gates, and the merge.
func assertMaxPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 1", "B = 1", "t0 = A ≥ B", "if A < B", "t0 = false", "normalise A",
		"normalise B", "if t0 { merge = A }", "if !t0 { merge = B }", "i1 = merge",
	}, displayPanelTexts(doc))
}

// assertMaxControlBehaviour checks the parameters, the gates that read the
// condition as 1/-1, and the merge that picks the larger parameter.
func assertMaxControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	e := entitiesByNumber(t, doc)
	assertConstant(t, e[1], "signal-A", 1)
	assertConstant(t, e[2], "signal-B", 1)
	assertArithmetic(t, e[5], "signal-info", "*", -1, "iron-plate")
	assertArithmetic(t, e[6], "signal-A", "*", 1, "signal-dot")
	assertArithmetic(t, e[7], "signal-B", "*", 1, "signal-dot")
	assertDecider(t, e[8], "iron-plate", "=", 1, "signal-dot", true)
	assertDecider(t, e[9], "iron-plate", "=", -1, "signal-dot", true)
	assertArithmetic(t, e[10], "signal-dot", "*", 1, "copper-plate")
}

// assertMaxDigitDisplay checks the eight-stage numeric readout reads the merged
// result on its item signal.
func assertMaxDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 11, "copper-plate")
}
