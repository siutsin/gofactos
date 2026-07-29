// This file protects operand order when a constant appears first.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConstFirstBlueprintJSON verifies constFirst JSON through the blueprint
// command.
// constFirst (a*a + 7*b) is the literal-and-chaining test case: it pins the
// literal source (`7` on an item signal), inlined literal labels (`t1 = c0 * B`),
// and chained intermediates (`t2 = t0 + t1`) from the CLI output.
func TestConstFirstBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "constfirst")

	assertBlueprintHeader(t, doc, "constFirst", 37, 36)

	assertConstFirstPanelDisplays(t, doc)
	assertConstFirstControlBehaviour(t, doc)
	assertNoRelayPoles(t, doc)
	assertSingleSubstationPower(t, doc, 37, 19)
	assertConstFirstDigitDisplay(t, doc)
}

// assertConstFirstPanelDisplays checks the teaching labels: the parameters on
// their letters, the literal source inlined as its value, and the computed
// values on their SSA names.
func assertConstFirstPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 1", "B = 1", "t0 = A * A", "t1 = c0 * B", "c0 = 7", "t2 = t0 + t1",
	}, displayPanelTexts(doc))

	entities := entitiesByNumber(t, doc)
	for _, want := range []struct {
		num  int
		text string
		icon string
	}{
		{31, "A = 1", "signal-A"},
		{32, "B = 1", "signal-B"},
		{33, "t0 = A * A", ""},
		{34, "t1 = c0 * B", ""},
		{35, "c0 = 7", ""},
		{36, "t2 = t0 + t1", ""},
	} {
		ent := entities[want.num]
		require.Equal(t, "display-panel", ent.Name)
		assert.True(t, ent.AlwaysShow)
		assert.Equal(t, want.text, ent.Text)
		assert.Equal(t, want.icon, signalName(ent.Icon))
		assert.Nil(t, ent.ControlBehavior)
	}
}

// assertConstFirstControlBehaviour checks the non-display combinators: the two
// parameter sources, the literal source, and the three arithmetic combinators,
// so a label-only change cannot hide a broken circuit.
func assertConstFirstControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	entities := entitiesByNumber(t, doc)
	assertConstant(t, entities[1], "signal-A", 1)
	assertConstant(t, entities[2], "signal-B", 1)
	assertConstant(t, entities[5], "iron-plate", 7)
	assertArithmeticSignals(t, entities[3], "signal-A", "*", "signal-A", "copper-plate")
	assertArithmeticSignals(t, entities[4], "iron-plate", "*", "signal-B", "steel-plate")
	assertArithmeticSignals(t, entities[6], "copper-plate", "+", "steel-plate", "iron-gear-wheel")
}

// assertConstFirstDigitDisplay checks the eight-stage numeric readout reads the
// result on its item signal.
func assertConstFirstDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 7, "iron-gear-wheel")
}
