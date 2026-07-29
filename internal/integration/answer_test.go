// This file protects constant-only blueprint generation through the CLI.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnswerBlueprintJSON verifies answer JSON through the blueprint command.
// answer (return 42) is the constant-only test case: no parameters and a single
// literal source on an item signal feeding the numeric readout, so it pins the
// no-parameter path from the CLI output.
func TestAnswerBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "answer")

	assertBlueprintHeader(t, doc, "answer", 27, 31)

	assertAnswerPanelDisplays(t, doc)
	assertAnswerControlBehaviour(t, doc)
	assertNoRelayPoles(t, doc)
	assertSingleSubstationPower(t, doc, 27, 16)
	assertAnswerDigitDisplay(t, doc)
}

// assertAnswerPanelDisplays checks the single teaching label: the literal source
// inlined as its cN token, with no icon.
func assertAnswerPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{"c0 = 42"}, displayPanelTexts(doc))

	ent := entitiesByNumber(t, doc)[26]
	require.Equal(t, "display-panel", ent.Name)
	assert.True(t, ent.AlwaysShow)
	assert.Equal(t, "c0 = 42", ent.Text)
	assert.Empty(t, signalName(ent.Icon))
	assert.Nil(t, ent.ControlBehavior)
}

// assertAnswerControlBehaviour checks the literal source emits 42 on its item
// signal, so a label-only change cannot hide a broken constant.
func assertAnswerControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertConstant(t, entitiesByNumber(t, doc)[1], "iron-plate", 42)
}

// assertAnswerDigitDisplay checks the eight-stage numeric readout reads the
// literal on its item signal.
func assertAnswerDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 2, "iron-plate")
}
