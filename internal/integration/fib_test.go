// This file protects iterative Fibonacci layout and display contracts.
package integration

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFibonacciBlueprintJSON verifies recurrence JSON through the blueprint
// command.
func TestFibonacciBlueprintJSON(t *testing.T) {
	t.Parallel()
	doc := runBlueprintJSON(t, "fib-n10")
	assert.Equal(t, "fib", doc.Blueprint.Label)

	labels := displayPanelTexts(doc)
	assertFibonacciLabels(t, labels)
	entities := entitiesByNumber(t, doc)
	assertFibonacciLabelPositions(t, entities)
	assertFibonacciRelayCount(t, entities)
	assertClockStartControl(t, doc)
	assert.Equal(
		t,
		[]float64{19.5, 20.5, 21.5, 22.5, 23.5, 24.5, 25.5, 26.5},
		fibonacciDigitPositions(t, entities),
		"digit panels must span the documented x coordinates",
	)
	firstDiv, firstInput := fibonacciDisplayStart(t, entities)
	assertNumericDigitDisplay(t, entities, firstDiv, firstInput)

	require.NotEmpty(t, doc.Blueprint.Wires)
}

// assertFibonacciLabels keeps the recurrence stages understandable in the
// generated blueprint.
func assertFibonacciLabels(t *testing.T, labels []string) {
	t.Helper()
	assert.Contains(t, labels, "A = 10")
	assert.Contains(t, labels, "t0 = φ(0, t1)")
	assert.Contains(t, labels, "t1 = φ(1, t4)")
	assert.Contains(t, labels, "t2 = φ(0, t5)")

	clocks, starts, gates, registers := 0, 0, 0, 0
	for _, label := range labels {
		switch {
		case label == "loop clock (1 Hz)":
			clocks++
		case label == clockStartControlLabel:
			starts++
		case strings.HasPrefix(label, "run while "):
			gates++
		case strings.Contains(label, " = φ("):
			registers++
		}
	}
	assert.Equal(t, 1, clocks)
	assert.Equal(t, 1, starts)
	assert.Equal(t, 1, gates)
	assert.Equal(t, 3, registers)
}

// assertFibonacciLabelPositions protects the documented recurrence layout.
func assertFibonacciLabelPositions(
	t *testing.T,
	entities map[int]blueprintEntity,
) {
	t.Helper()
	labelPositions := make(map[string]blueprintPosition, len(entities))
	for _, ent := range entities {
		if ent.Name == "display-panel" && ent.Text != "" {
			_, duplicate := labelPositions[ent.Text]
			require.False(
				t,
				duplicate,
				"duplicate test case layout label %q",
				ent.Text,
			)
			labelPositions[ent.Text] = ent.Position
		}
	}
	assert.Equal(t, map[string]blueprintPosition{
		"loop clock (1 Hz)":    {X: 1.5, Y: 0.5},
		clockStartControlLabel: {X: 3.5, Y: 0.5},
		"A = 10":               {X: 5.5, Y: 0.5},
		"run while t2 < A":     {X: 11, Y: 0.5},
		"t0 = φ(0, t1)":        {X: 15.5, Y: 0.5},
		"t4 = t0 + t1":         {X: 19.5, Y: 0.5},
		"t1 = φ(1, t4)":        {X: 15.5, Y: 4.5},
		"c0 = 1":               {X: 5.5, Y: 3.5},
		"t5 = t2 + c0":         {X: 19.5, Y: 3.5},
		"t2 = φ(0, t5)":        {X: 15.5, Y: 8.5},
	}, labelPositions, "test case layout diagram must match generated labels")
}

// assertFibonacciRelayCount checks that only genuinely long recurrence wires
// need poles; parallel phi registers must not create an artificial corridor.
func assertFibonacciRelayCount(
	t *testing.T,
	entities map[int]blueprintEntity,
) {
	t.Helper()
	relays := 0
	for _, ent := range entities {
		if ent.Name == "medium-electric-pole" {
			relays++
		}
	}
	assert.Equal(t, 8, relays)
}

// fibonacciDigitPositions exposes the result row for compact layout checks.
func fibonacciDigitPositions(
	t *testing.T,
	entities map[int]blueprintEntity,
) []float64 {
	t.Helper()
	var digitX []float64
	for _, ent := range entities {
		if ent.Name != "display-panel" || ent.Text != "" ||
			ent.ControlBehavior == nil ||
			len(ent.ControlBehavior.Parameters) != 10 {
			continue
		}
		assert.InDelta(
			t,
			10.5,
			ent.Position.Y,
			0,
			"digit panels must remain on the documented row",
		)
		digitX = append(digitX, ent.Position.X)
	}
	sort.Float64s(digitX)
	return digitX
}

// fibonacciDisplayStart identifies the public entry to the decimal display.
func fibonacciDisplayStart(
	t *testing.T,
	entities map[int]blueprintEntity,
) (int, string) {
	t.Helper()
	firstDiv, firstInput, starts := 0, "", 0
	for number, ent := range entities {
		if ent.ControlBehavior == nil {
			continue
		}
		ac := ent.ControlBehavior.ArithmeticConditions
		if ac == nil || ac.Operation != "/" ||
			ac.SecondConstant == nil || *ac.SecondConstant != 10 ||
			ac.FirstSignal == nil ||
			ac.FirstSignal.Name == "signal-dot" {
			continue
		}
		firstDiv = number
		firstInput = ac.FirstSignal.Name
		starts++
	}
	require.Equal(t, 1, starts)
	return firstDiv, firstInput
}
