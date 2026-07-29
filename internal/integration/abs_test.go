// This file protects the absolute-value blueprint contract through the CLI.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAbsBlueprintJSON verifies abs's labels, control behaviour, relay, power,
// and digit display through blueprint-command JSON.
func TestAbsBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "abs")

	assertBlueprintHeader(t, doc, "abs", 46, 45)

	assertAbsPanelDisplays(t, doc)
	assertAbsControlBehaviour(t, doc)
	assertAbsElectricity(t, doc)
	assertAbsDigitDisplay(t, doc)
}

// TestAbsBlueprintJSONReflectsSourceChanges verifies the CLI regenerates the
// expected JSON when the source changes, so the test covers generation rather
// than only pinning the checked-in test case.
func TestAbsBlueprintJSONReflectsSourceChanges(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "abs-bound-2")

	// The overall blueprint shape should stay stable.
	assert.Equal(t, "abs", doc.Blueprint.Label)
	require.Len(t, doc.Blueprint.Entities, 46)
	require.Len(t, doc.Blueprint.Wires, 45)

	// The changed comparison should flow through labels and deciders.
	labels := displayPanelTexts(doc)
	assert.Contains(t, labels, "t0 = A < 2")
	assert.Contains(t, labels, "if A ≥ 2")
	assert.NotContains(t, labels, "t0 = A < 0")
	assert.NotContains(t, labels, "if A ≥ 0")

	entities := entitiesByNumber(t, doc)
	assertDecider(t, entities[2], "signal-A", "<", 2, "copper-plate", false)
	assertDecider(t, entities[3], "signal-A", "≥", 2, "signal-info", false)
	assertAbsElectricity(t, doc)
	assertAbsDigitDisplay(t, doc)
}

// assertAbsPanelDisplays checks teaching labels separately because the talk
// depends on those words being short, stable, and easy to scan in game.
func assertAbsPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	labels := []string{
		"A = 1",
		"t0 = A < 0",
		"if A ≥ 0",
		"t0 = false",
		"t1 = -A",
		"normalise t1",
		"normalise A",
		"if t0 { merge = t1 }",
		"if !t0 { merge = A }",
		"i2 = merge",
	}
	assert.Equal(t, labels, displayPanelTexts(doc))
	assert.NotContains(t, labels, "phi")
	assert.NotContains(t, labels, "merge = t1")

	entities := entitiesByNumber(t, doc)
	for _, want := range []struct {
		num  int
		text string
		icon string
	}{
		{35, "A = 1", "signal-A"},
		{36, "t0 = A < 0", ""},
		{37, "if A ≥ 0", ""},
		{38, "t0 = false", ""},
		{39, "t1 = -A", ""},
		{40, "normalise t1", ""},
		{41, "normalise A", ""},
		{42, "if t0 { merge = t1 }", ""},
		{43, "if !t0 { merge = A }", ""},
		{44, "i2 = merge", ""},
	} {
		ent := entities[want.num]
		require.Equal(t, "display-panel", ent.Name)
		assert.True(t, ent.AlwaysShow)
		assert.Equal(t, want.text, ent.Text)
		assert.Equal(t, want.icon, signalName(ent.Icon))
		assert.Nil(t, ent.ControlBehavior)
	}
}

// assertAbsControlBehaviour checks the non-display combinators that implement
// abs, so label-only changes cannot hide a broken circuit.
func assertAbsControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	entities := entitiesByNumber(t, doc)
	assertConstant(t, entities[1], "signal-A", 1)
	assertDecider(t, entities[2], "signal-A", "<", 0, "copper-plate", false)
	assertDecider(t, entities[3], "signal-A", "≥", 0, "signal-info", false)
	assertArithmetic(t, entities[4], "signal-info", "*", -1, "copper-plate")
	assertArithmetic(t, entities[5], "signal-A", "*", -1, "iron-plate")
	assertArithmetic(t, entities[6], "iron-plate", "*", 1, "signal-dot")
	assertArithmetic(t, entities[7], "signal-A", "*", 1, "signal-dot")
	assertDecider(t, entities[8], "copper-plate", "=", 1, "signal-dot", true)
	assertDecider(t, entities[9], "copper-plate", "=", -1, "signal-dot", true)
	assertArithmetic(t, entities[10], "signal-dot", "*", 1, "steel-plate")
}

// assertAbsElectricity checks every powered combinator is covered by the
// generated substation, so the imported blueprint runs in game.
func assertAbsElectricity(t *testing.T, doc blueprintJSON) {
	t.Helper()
	entities := entitiesByNumber(t, doc)

	// The public relay uses a medium pole; power uses a legendary substation.
	assertRelayPoleCount(t, doc, 1)
	assert.Equal(t, "medium-electric-pole", entities[45].Name)
	assert.Empty(t, entities[45].Quality)
	assertSingleSubstationPower(t, doc, 46, 25)
}

// assertAbsDigitDisplay checks the eight-stage numeric readout because it is a
// compact composite with intentionally unlabeled internal combinators.
func assertAbsDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 11, "steel-plate")
}
