// This file protects ordinary-call expansion without recursive machinery.
package integration

import (
	"bytes"
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/siutsin/gofactos/internal/app"
)

// TestCallBlueprintJSON verifies all roots in the shared call cases pass
// through --func and that expanded calls retain their public JSON contracts.
func TestCallBlueprintJSON(t *testing.T) {
	for _, tc := range []struct {
		name, function string
	}{
		{name: "absolute", function: "absolute"},
		{name: "branchcall", function: "branchCall"},
		{name: "identity", function: "identity"},
		{name: "square", function: "square"},
		{name: "sumidentities", function: "sumIdentities"},
		{name: "sumsquares", function: "sumSquares"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc := runBlueprintJSON(t, tc.name)
			assertBlueprintIdentity(t, doc, tc.function)
			require.NotEmpty(t, doc.Blueprint.Entities)
			require.NotEmpty(t, doc.Blueprint.Wires)
			assertNoRecursiveStartControl(t, doc)

			switch tc.name {
			case "sumsquares":
				assertSumSquaresCallContract(t, doc)
			case "sumidentities":
				assertSumIdentitiesCallContract(t, doc)
			case "branchcall":
				assertBranchCallContract(t, doc)
			}
		})
	}
}

// assertNoRecursiveStartControl keeps ordinary calls independent of the
// manually armed recursive controller.
func assertNoRecursiveStartControl(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Empty(t, clockStartControls(doc))
	assert.NotContains(
		t,
		displayPanelTexts(doc),
		clockStartControlLabel,
	)
}

// TestCallCasesRequireRootSelection proves the CLI rejects each shared
// source file until --func identifies one root and writes no partial blueprint.
func TestCallCasesRequireRootSelection(t *testing.T) {
	root := projectRoot(t)
	for _, file := range []string{"branchcall.go", "calls.go"} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			command := app.NewCommand()
			command.Writer = &output
			command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
			err := command.Run(context.Background(), []string{
				"gofactos",
				"blueprint",
				filepath.Join(root, "internal", "testdata", file),
			})
			require.ErrorContains(t, err, "multiple functions found")
			require.ErrorContains(t, err, "use --func to select one")
			assert.Empty(t, output.Bytes())
		})
	}
}

// assertSumSquaresCallContract protects expansion of two calls with distinct
// arguments and results.
func assertSumSquaresCallContract(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 3",
		"B = -4",
		"square#1.t0 = A * A",
		"square#2.t0 = B * B",
		"t2 = t0 + t1",
	}, displayPanelTexts(doc))
	assertOrdinaryCallShape(t, doc, []string{"signal-A", "signal-B"})
}

// assertSumIdentitiesCallContract protects reuse of one input across two call
// sites.
func assertSumIdentitiesCallContract(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assert.Equal(t, []string{
		"A = 1",
		"t2 = t0 + t1",
	}, displayPanelTexts(doc))
	assertOrdinaryCallShape(t, doc, []string{"signal-A"})

	var add *blueprintArithmeticConditions
	for i := range doc.Blueprint.Entities {
		conditions := doc.Blueprint.Entities[i].ControlBehavior
		if conditions == nil || conditions.ArithmeticConditions == nil ||
			conditions.ArithmeticConditions.Operation != "+" {
			continue
		}
		require.Nil(t, add)
		add = conditions.ArithmeticConditions
	}
	require.NotNil(t, add)
	assert.Equal(t, "signal-A", signalName(add.FirstSignal))
	assert.Equal(t, "signal-A", signalName(add.SecondSignal))
}

// assertBranchCallContract protects independent expansion of a branching
// callee at two call sites.
func assertBranchCallContract(t *testing.T, doc blueprintJSON) {
	t.Helper()
	labels := displayPanelTexts(doc)
	assert.Contains(t, labels, "absolute#1.result = merge")
	assert.Contains(t, labels, "absolute#2.result = merge")
	assert.Contains(t, labels, "normalise t1")
	assert.Contains(t, labels, "normalise t2")
	assertOrdinaryCallShape(t, doc, []string{"signal-A"})
}

// assertOrdinaryCallShape keeps non-recursive calls compact and free of
// recursive-only controls.
func assertOrdinaryCallShape(
	t *testing.T,
	doc blueprintJSON,
	wantInputs []string,
) {
	t.Helper()
	assert.Equal(t, wantInputs, blueprintInputSignals(doc))
	assert.Equal(t, 8, numericDigitPanelCount(doc))
	for _, entity := range doc.Blueprint.Entities {
		assert.NotEqual(t, "small-lamp", entity.Name)
		if entity.ControlBehavior == nil ||
			entity.ControlBehavior.ArithmeticConditions == nil {
			continue
		}
		conditions := entity.ControlBehavior.ArithmeticConditions
		if conditions.Operation == "%" && conditions.SecondConstant != nil {
			assert.NotEqual(t, 60, *conditions.SecondConstant)
		}
	}
	for _, label := range displayPanelTexts(doc) {
		assert.NotContains(t, label, "clock")
		assert.False(t, strings.HasPrefix(label, "frame "))
		assert.NotEqual(t, "call stack", label)
		assert.NotEqual(t, "one frame = one live call", label)
		assert.NotEqual(t, "green = current step", label)
		assert.NotEqual(t, "green = current frame", label)
		assert.NotEqual(t, "activation records", label)
	}
}

// blueprintInputSignals provides a stable view of a call blueprint's public
// inputs.
func blueprintInputSignals(doc blueprintJSON) []string {
	var signals []string
	for _, entity := range doc.Blueprint.Entities {
		if entity.Name != "constant-combinator" ||
			entity.ControlBehavior == nil ||
			entity.ControlBehavior.Sections == nil {
			continue
		}
		for _, section := range entity.ControlBehavior.Sections.Sections {
			for _, filter := range section.Filters {
				if filter.Type == "virtual" &&
					strings.HasPrefix(filter.Name, "signal-") {
					signals = append(signals, filter.Name)
				}
			}
		}
	}
	sort.Strings(signals)
	return signals
}

// numericDigitPanelCount protects the fixed width of numeric result displays.
func numericDigitPanelCount(doc blueprintJSON) int {
	count := 0
	for _, entity := range doc.Blueprint.Entities {
		if entity.Name == "display-panel" &&
			entity.ControlBehavior != nil &&
			len(entity.ControlBehavior.Parameters) == 10 {
			count++
		}
	}
	return count
}
