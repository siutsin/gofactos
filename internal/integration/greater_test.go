// This file protects direct boolean comparison and display generation.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGreaterBlueprintJSON verifies greater JSON through the blueprint command.
// greater (return a > b) is the smallest test case: a bare comparison whose
// 1/-1 condition is the result, with no merge, read straight by boolDisplay.
func TestGreaterBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "greater")

	assertBlueprintHeader(t, doc, "greater", 12, 6)

	assertGreaterPanelDisplays(t, doc)
	assertGreaterControlBehaviour(t, doc)
	assertNoRelayPoles(t, doc)
	assertSingleSubstationPower(t, doc, 12, 3)
	assertBoolDisplay(t, entitiesByNumber(t, doc)[6], "iron-plate")
}

// assertGreaterPanelDisplays checks the teaching labels: the two parameters and
// the compare's true and false legs.
func assertGreaterPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 1", "B = 1", "t0 = A > B", "if A ≤ B", "t0 = false",
	}, displayPanelTexts(doc))
}

// assertGreaterControlBehaviour checks the parameters and the negating arithmetic
// that synthesises the present -1 for a false result.
func assertGreaterControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	e := entitiesByNumber(t, doc)
	assertConstant(t, e[1], "signal-A", 1)
	assertConstant(t, e[2], "signal-B", 1)
	assertArithmetic(t, e[5], "signal-info", "*", -1, "iron-plate")
}
