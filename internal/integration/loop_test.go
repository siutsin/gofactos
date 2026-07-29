// This file protects counted-loop source equivalence through the blueprint
// command.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestForIAndForRangeProduceSameBlueprint proves the two loop shapes converge.
// forI is a counted `for i := 0; i < n; i++` (a header block plus a body that
// jumps back); forRange is `for range n` (a self-loop, the body jumps to
// itself). The SSA differs, but gofactos detects the back edge in both and
// lowers each to the same clocked registers and stop gate, so the blueprints are
// identical down to the entities and wires. Only the label differs.
func TestForIAndForRangeProduceSameBlueprint(t *testing.T) {
	var fori, forrange blueprintJSON
	t.Run("blueprints", func(t *testing.T) {
		t.Run("forI", func(t *testing.T) {
			t.Parallel()
			fori = runBlueprintJSON(t, "fori")
		})
		t.Run("forRange", func(t *testing.T) {
			t.Parallel()
			forrange = runBlueprintJSON(t, "forrange")
		})
	})

	assert.Equal(t, "forI", fori.Blueprint.Label)
	assert.Equal(t, "forRange", forrange.Blueprint.Label)
	assert.Equal(t, fori.Blueprint.Entities, forrange.Blueprint.Entities,
		"the two loop shapes must place identical entities")
	assert.Equal(t, fori.Blueprint.Wires, forrange.Blueprint.Wires,
		"the two loop shapes must wire identically")
}

// TestForCounterBlueprintJSON keeps the counter-result test case on the
// blueprint-command path without adding another complete expected output.
func TestForCounterBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "forcounter")

	assertBlueprintHeader(t, doc, "forCounter", 54, 61)
	assertClockStartControl(t, doc)
	assert.Contains(t, displayPanelTexts(doc), "c1 = 1")
}
