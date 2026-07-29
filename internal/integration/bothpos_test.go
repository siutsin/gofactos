// This file protects boolean short-circuit lowering and its visible result.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBothPosBlueprintJSON verifies bothPos JSON through the blueprint command.
// bothPos (a > 0 && b > 0) is the boolean-merge test case: it pins the
// short-circuit && lowering (two compares plus a phi merging a constant false)
// and the boolDisplay readout (check for true, deny for false) from the CLI
// output. It also exercises relay insertion for the constant-false arm.
func TestBothPosBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "bothpos")

	assertBlueprintHeader(t, doc, "bothPos", 31, 18)

	assertBothPosPanelDisplays(t, doc)
	assertBothPosControlBehaviour(t, doc)
	assertBothPosElectricity(t, doc)
	assertBothPosBoolDisplay(t, doc)
}

// assertBothPosPanelDisplays checks the teaching labels: the parameters, the two
// comparisons, the false arm, the normalise and gate steps, and the merge.
func assertBothPosPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 1", "B = 1", "t0 = A > 0", "if A ≤ 0", "t0 = false",
		"t1 = B > 0", "if B ≤ 0", "t1 = false", "normalise t1", "normalise c0",
		"if t0 { merge = t1 }", "if !t0 { merge = c0 }", "t2 = merge",
		"c0 = false",
	}, displayPanelTexts(doc))
}

// assertBothPosControlBehaviour checks every non-display combinator: the two
// parameters, the constant false arm, the two compares (each a true decider, a
// negated decider, and a negating arithmetic), the two normalise steps, the two
// gates, and the merge, so a label-only change cannot hide a broken circuit.
func assertBothPosControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	entities := entitiesByNumber(t, doc)

	assertConstant(t, entities[1], "signal-A", 1)
	assertConstant(t, entities[2], "signal-B", 1)
	assertConstant(t, entities[14], "copper-plate", -1)

	// t0 = (a > 0) on steel-plate, present 1 or -1.
	assertDecider(t, entities[3], "signal-A", ">", 0, "steel-plate", false)
	assertDecider(t, entities[4], "signal-A", "≤", 0, "signal-info", false)
	assertArithmetic(t, entities[5], "signal-info", "*", -1, "steel-plate")

	// t1 = (b > 0) on iron-plate.
	assertDecider(t, entities[6], "signal-B", ">", 0, "iron-plate", false)
	assertDecider(t, entities[7], "signal-B", "≤", 0, "signal-info", false)
	assertArithmetic(t, entities[8], "signal-info", "*", -1, "iron-plate")

	// The phi normalises both arms to signal-dot, gates on t0, and sums.
	assertArithmetic(t, entities[9], "iron-plate", "*", 1, "signal-dot")
	assertArithmetic(t, entities[10], "copper-plate", "*", 1, "signal-dot")
	assertDecider(t, entities[11], "steel-plate", "=", 1, "signal-dot", true)
	assertDecider(t, entities[12], "steel-plate", "=", -1, "signal-dot", true)
	assertArithmetic(t, entities[13], "signal-dot", "*", 1, "iron-gear-wheel")
}

// assertBothPosElectricity checks the single substation powers every combinator
// and that bothPos inserts exactly one relay pole, since the constant false arm
// reaches past a combinator's wire span.
func assertBothPosElectricity(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertRelayPoleCount(t, doc, 1)
	assertSingleSubstationPower(t, doc, 31, 11)
}

// assertBothPosBoolDisplay checks the boolean readout shows a check for a true
// result and a deny for a false one, reading the merged result signal.
func assertBothPosBoolDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertBoolDisplay(t, entitiesByNumber(t, doc)[15], "iron-gear-wheel")
}
