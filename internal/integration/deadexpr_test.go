// This file protects dead-expression pruning in generated blueprints.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDeadExprBlueprintJSON verifies deadExpr JSON through the blueprint
// command.
// deadExpr returns a+b but also computes a*b and r*r and discards them. The
// lowering pass's backward liveness pruning removes those provisional
// instances, so no multiply combinator is emitted. This test pins that the
// dead expressions leave no trace.
func TestDeadExprBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "deadexpr")

	assertBlueprintHeader(t, doc, "deadExpr", 31, 33)

	assertDeadExprPanelDisplays(t, doc)
	assertDeadExprControlBehaviour(t, doc)
	assertDeadExprPruned(t, doc)
	assertNoRelayPoles(t, doc)
	assertSingleSubstationPower(t, doc, 31, 17)
	assertDeadExprDigitDisplay(t, doc)
}

// assertDeadExprPanelDisplays checks the teaching labels: two parameters and the
// single live result, with no label for the discarded products.
func assertDeadExprPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{"A = 1", "B = 1", "t0 = A + B"}, displayPanelTexts(doc))
}

// assertDeadExprControlBehaviour checks the parameters and the one live adder.
func assertDeadExprControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	entities := entitiesByNumber(t, doc)
	assertConstant(t, entities[1], "signal-A", 1)
	assertConstant(t, entities[2], "signal-B", 1)
	assertArithmeticSignals(t, entities[3], "signal-A", "+", "signal-B", "iron-plate")
}

// assertDeadExprPruned proves backward liveness pruning: a*b and r*r are
// discarded, so no multiply combinator is emitted and the only non-readout op
// is the live a+b. The readout chain uses only divide and modulo.
func assertDeadExprPruned(t *testing.T, doc blueprintJSON) {
	t.Helper()
	adds, muls := 0, 0
	for _, ent := range doc.Blueprint.Entities {
		if ent.Name != "arithmetic-combinator" || ent.ControlBehavior == nil ||
			ent.ControlBehavior.ArithmeticConditions == nil {
			continue
		}
		switch ent.ControlBehavior.ArithmeticConditions.Operation {
		case "+":
			adds++
		case "*":
			muls++
		}
	}
	assert.Equal(t, 1, adds, "only the live a+b remains")
	assert.Equal(t, 0, muls, "the dead a*b and r*r leave no multiply combinator")
}

// assertDeadExprDigitDisplay checks the eight-stage numeric readout reads the
// result on its item signal.
func assertDeadExprDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 4, "iron-plate")
}
