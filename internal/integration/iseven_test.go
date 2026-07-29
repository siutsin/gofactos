// This file protects modulo-based boolean generation and power coverage.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsEvenBlueprintJSON verifies isEven JSON through the blueprint command.
// isEven returns a constant true or false through a branch, so it pins the
// boolean encoding (true is 1, false is -1) merging to the boolDisplay readout.
func TestIsEvenBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "iseven")

	assertBlueprintHeader(t, doc, "isEven", 30, 17)

	assertIsEvenPanelDisplays(t, doc)
	assertIsEvenControlBehaviour(t, doc)
	assertIsEvenElectricity(t, doc)
	assertBoolDisplay(t, entitiesByNumber(t, doc)[14], "electronic-circuit")
}

// assertIsEvenPanelDisplays checks the teaching labels: the modulo, the equals
// test, the true and false legs, the normalise steps, the gates, and the merge.
func assertIsEvenPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 1", "t0 = A % c0", "c0 = 2", "t1 = t0 = 0", "if t0 ≠ 0", "t1 = false",
		"normalise c1", "normalise c2", "if t1 { merge = c1 }",
		"if !t1 { merge = c2 }", "i5 = merge", "c1 = true", "c2 = false",
	}, displayPanelTexts(doc))
}

// assertIsEvenControlBehaviour checks the modulo, the zero test, the boolean
// constants (true is 1, false is -1), the gates, and the merge.
func assertIsEvenControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	e := entitiesByNumber(t, doc)
	assertConstant(t, e[1], "signal-A", 1)
	assertConstant(t, e[3], "iron-plate", 2)
	assertConstant(t, e[12], "steel-plate", 1)
	assertConstant(t, e[13], "iron-gear-wheel", -1)
	assertArithmeticSignals(t, e[2], "signal-A", "%", "iron-plate", "copper-plate")
	assertDecider(t, e[4], "copper-plate", "=", 0, "copper-cable", false)
	assertArithmetic(t, e[6], "signal-info", "*", -1, "copper-cable")
	assertDecider(t, e[9], "copper-cable", "=", 1, "signal-dot", true)
	assertDecider(t, e[10], "copper-cable", "=", -1, "signal-dot", true)
	assertArithmetic(t, e[11], "signal-dot", "*", 1, "electronic-circuit")
}

// assertIsEvenElectricity checks the two relay poles the taller boolean merge
// needs and that the single substation powers every combinator.
func assertIsEvenElectricity(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertRelayPoleCount(t, doc, 2)
	assertSingleSubstationPower(t, doc, 30, 9)
}
