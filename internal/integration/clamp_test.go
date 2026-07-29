// This file protects sequential branch merges in the clamp blueprint.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestClampBlueprintJSON protects sequential phi merges, literal bounds,
// multi-substation power, and relay routing through the blueprint command.
func TestClampBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "clamp")

	assertBlueprintHeader(t, doc, "clamp", 70, 62)

	assertClampPanelDisplays(t, doc)
	assertClampControlBehaviour(t, doc)
	assertClampElectricity(t, doc)
	assertClampDigitDisplay(t, doc)
}

// assertClampPanelDisplays checks the teaching labels for both clamp stages: the
// parameter, the two compares, the false arms, normalise and gate steps, and the
// two merges.
func assertClampPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 1", "t0 = A < 0", "if A ≥ 0", "t0 = false", "normalise c0",
		"normalise A", "if t0 { merge = c0 }", "if !t0 { merge = A }",
		"t1 = merge", "c0 = 0", "t2 = t1 > 100", "if t1 ≤ 100", "t2 = false",
		"normalise c1", "normalise t1", "if t2 { merge = c1 }",
		"if !t2 { merge = t1 }", "t3 = merge", "c1 = 100",
	}, displayPanelTexts(doc))
}

// assertClampControlBehaviour checks every non-display combinator across both
// clamp stages: the parameter, the two literal bounds, the two compares, the
// normalise steps, the gates, and the two merges, so a label-only change cannot
// hide a broken circuit.
func assertClampControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	e := entitiesByNumber(t, doc)

	assertConstant(t, e[1], "signal-A", 1)
	assertConstant(t, e[10], "iron-plate", 0)
	assertConstant(t, e[19], "iron-gear-wheel", 100)

	// Stage one: t1 = max(x, 0). compare x < 0, gate, merge.
	assertDecider(t, e[2], "signal-A", "<", 0, "copper-plate", false)
	assertDecider(t, e[3], "signal-A", "≥", 0, "signal-info", false)
	assertArithmetic(t, e[4], "signal-info", "*", -1, "copper-plate")
	assertArithmetic(t, e[5], "iron-plate", "*", 1, "signal-dot")
	assertArithmetic(t, e[6], "signal-A", "*", 1, "signal-dot")
	assertDecider(t, e[7], "copper-plate", "=", 1, "signal-dot", true)
	assertDecider(t, e[8], "copper-plate", "=", -1, "signal-dot", true)
	assertArithmetic(t, e[9], "signal-dot", "*", 1, "steel-plate")

	// Stage two: t3 = min(t1, 100). compare t1 > 100, gate, merge.
	assertDecider(t, e[11], "steel-plate", ">", 100, "copper-cable", false)
	assertDecider(t, e[12], "steel-plate", "≤", 100, "signal-info", false)
	assertArithmetic(t, e[13], "signal-info", "*", -1, "copper-cable")
	assertArithmetic(t, e[14], "iron-gear-wheel", "*", 1, "signal-dot")
	assertArithmetic(t, e[15], "steel-plate", "*", 1, "signal-dot")
	assertDecider(t, e[16], "copper-cable", "=", 1, "signal-dot", true)
	assertDecider(t, e[17], "copper-cable", "=", -1, "signal-dot", true)
	assertArithmetic(t, e[18], "signal-dot", "*", 1, "electronic-circuit")
}

// assertClampElectricity checks the clamp blueprint needs two substations and
// six relay poles, and that every combinator is powered by one of the
// substations.
func assertClampElectricity(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertRelayPoleCount(t, doc, 6)
	assertPoweredByAnySubstation(t, doc, 2, 32)
}

// assertClampDigitDisplay checks the eight-stage numeric readout reads the
// clamped result on its item signal.
func assertClampDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 20, "electronic-circuit")
}
