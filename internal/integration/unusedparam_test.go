// This file protects pruning of unused parameter sources and labels.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestUnusedParamBlueprintJSON verifies the unusedParam blueprint JSON end to
// end. unusedParam(a, b) returns b + 1, so a is never read even though --set
// configures it. Its source is pruned while b keeps signature-indexed signal-B.
func TestUnusedParamBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "unusedparam")

	assertBlueprintHeader(t, doc, "unusedParam", 31, 33)

	assertUnusedParamPanelDisplays(t, doc)
	assertUnusedParamControlBehaviour(t, doc)
	assertNoRelayPoles(t, doc)
	assertSingleSubstationPower(t, doc, 31, 17)
	assertUnusedParamDigitDisplay(t, doc)
}

// assertUnusedParamPanelDisplays checks the live signature-indexed parameter,
// the add, and the literal, with no label for the configured dead parameter a.
func assertUnusedParamPanelDisplays(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(
		t,
		[]string{"B = 7", "t0 = B + c0", "c0 = 1"},
		displayPanelTexts(doc),
	)
}

// assertUnusedParamControlBehaviour checks the live parameter and literal, the
// add, and that configured but unused parameter a leaves no source behind.
func assertUnusedParamControlBehaviour(t *testing.T, doc blueprintJSON) {
	t.Helper()
	e := entitiesByNumber(t, doc)
	assertConstant(t, e[1], "signal-B", 7)
	assertConstant(t, e[3], "iron-plate", 1)
	assertArithmeticSignals(t, e[2], "signal-B", "+", "iron-plate", "copper-plate")

	for _, ent := range doc.Blueprint.Entities {
		if ent.Name != "constant-combinator" || ent.ControlBehavior == nil ||
			ent.ControlBehavior.Sections == nil {
			continue
		}
		for _, sec := range ent.ControlBehavior.Sections.Sections {
			for _, f := range sec.Filters {
				assert.NotEqual(t, "signal-A", f.Name)
			}
		}
	}
}

// assertUnusedParamDigitDisplay checks the eight-stage numeric readout reads the
// result on its item signal.
func assertUnusedParamDigitDisplay(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertNumericDigitDisplay(t, entitiesByNumber(t, doc), 4, "copper-plate")
}
