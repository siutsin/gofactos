// This file protects the smallest arithmetic blueprint contract.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddBlueprintJSON verifies add's labels, control behaviour, power, and
// digit display through blueprint-command JSON. The simplest function pins
// parameters to letter signals and the sum to an item signal.
func TestAddBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "add")

	assertBlueprintHeader(t, doc, "add", 31, 33)

	assertAddPanelDisplays(t, doc)
	assertAddControlBehaviour(t, doc)
	assertNoRelayPoles(t, doc)
	assertSingleSubstationPower(t, doc, 31, 17)
	assertAddDigitDisplay(t, doc)
}

// assertAddPanelDisplays checks the three teaching labels: the two parameters on
// their letters and the sum on its SSA name.
func assertAddPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{"A = 1", "B = 1", "t0 = A + B"}, displayPanelTexts(doc))

	entities := entitiesByNumber(t, doc)
	for _, want := range []struct {
		num  int
		text string
		icon string
	}{
		{28, "A = 1", "signal-A"},
		{29, "B = 1", "signal-B"},
		{30, "t0 = A + B", ""},
	} {
		ent := entities[want.num]
		require.Equal(t, "display-panel", ent.Name)
		assert.True(t, ent.AlwaysShow)
		assert.Equal(t, want.text, ent.Text)
		assert.Equal(t, want.icon, signalName(ent.Icon))
		assert.Nil(t, ent.ControlBehavior)
	}
}

// assertAddControlBehaviour checks the non-display combinators: each parameter
// source and the adder, so a label-only change cannot hide a broken circuit.
func assertAddControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	entities := entitiesByNumber(t, doc)
	assertConstant(t, entities[1], "signal-A", 1)
	assertConstant(t, entities[2], "signal-B", 1)
	assertArithmeticSignals(t, entities[3], "signal-A", "+", "signal-B", "iron-plate")
}

// assertAddDigitDisplay checks the eight-stage numeric readout reads the result
// on its item signal.
func assertAddDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 4, "iron-plate")
}
