// This file supplies shared blueprint contracts for integration tests.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siutsin/gofactos/internal/app"
)

type blueprintJSON struct {
	Blueprint struct {
		Item     string            `json:"item"`
		Label    string            `json:"label"`
		Version  int64             `json:"version"`
		Entities []blueprintEntity `json:"entities"`
		Wires    []blueprintWire   `json:"wires"`
	} `json:"blueprint"`
}

type blueprintWire [4]int

const blueprintVersion int64 = 562949954076672

type blueprintEntity struct {
	EntityNumber    int                       `json:"entity_number"`
	Name            string                    `json:"name"`
	Quality         string                    `json:"quality,omitempty"`
	Position        blueprintPosition         `json:"position"`
	Direction       int                       `json:"direction,omitempty"`
	Text            string                    `json:"text,omitempty"`
	Icon            *blueprintSignal          `json:"icon,omitempty"`
	Colour          *blueprintColour          `json:"color,omitempty"`
	AlwaysShow      bool                      `json:"always_show,omitempty"`
	ControlBehavior *blueprintControlBehavior `json:"control_behavior,omitempty"`
}

type blueprintPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type blueprintSignal struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type blueprintColour struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}

type blueprintControlBehavior struct {
	ArithmeticConditions *blueprintArithmeticConditions `json:"arithmetic_conditions,omitempty"`
	DeciderConditions    *blueprintDeciderConditions    `json:"decider_conditions,omitempty"`
	Sections             *blueprintSections             `json:"sections,omitempty"`
	Parameters           []blueprintDisplayMessage      `json:"parameters,omitempty"`
	IsOn                 *bool                          `json:"is_on,omitempty"`
	CircuitEnabled       bool                           `json:"circuit_enabled,omitempty"`
	CircuitCondition     *blueprintCircuitCondition     `json:"circuit_condition,omitempty"`
}

type blueprintCircuitCondition struct {
	FirstSignal *blueprintSignal `json:"first_signal,omitempty"`
	Comparator  string           `json:"comparator"`
	Constant    *int             `json:"constant,omitempty"`
}

type blueprintArithmeticConditions struct {
	FirstSignal    *blueprintSignal `json:"first_signal,omitempty"`
	Operation      string           `json:"operation"`
	SecondSignal   *blueprintSignal `json:"second_signal,omitempty"`
	OutputSignal   *blueprintSignal `json:"output_signal,omitempty"`
	FirstConstant  *int             `json:"first_constant,omitempty"`
	SecondConstant *int             `json:"second_constant,omitempty"`
}

type blueprintDeciderConditions struct {
	Conditions []blueprintDeciderCondition `json:"conditions"`
	Outputs    []blueprintDeciderOutput    `json:"outputs"`
}

type blueprintDeciderCondition struct {
	FirstSignal  *blueprintSignal `json:"first_signal,omitempty"`
	Comparator   string           `json:"comparator"`
	Constant     *int             `json:"constant,omitempty"`
	SecondSignal *blueprintSignal `json:"second_signal,omitempty"`
	CompareType  string           `json:"compare_type,omitempty"`
}

type blueprintDeciderOutput struct {
	Signal             *blueprintSignal `json:"signal,omitempty"`
	CopyCountFromInput bool             `json:"copy_count_from_input"`
}

type blueprintSections struct {
	Sections []blueprintLogisticSection `json:"sections"`
}

type blueprintLogisticSection struct {
	Index   int                       `json:"index"`
	Filters []blueprintConstantFilter `json:"filters"`
}

type blueprintConstantFilter struct {
	Index      int    `json:"index"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	Quality    string `json:"quality"`
	Comparator string `json:"comparator"`
	Count      int    `json:"count"`
}

type blueprintDisplayMessage struct {
	Text      string                     `json:"text"`
	Icon      *blueprintSignal           `json:"icon,omitempty"`
	Condition *blueprintDisplayCondition `json:"condition,omitempty"`
}

type blueprintDisplayCondition struct {
	FirstSignal *blueprintSignal `json:"first_signal,omitempty"`
	Comparator  string           `json:"comparator,omitempty"`
	Constant    *int             `json:"constant,omitempty"`
}

type gridCell struct {
	x int
	y int
}

// runBlueprintJSON decodes blueprint-command output for content checks. Exact
// byte checks live in the expected output test.
func runBlueprintJSON(t *testing.T, name string) blueprintJSON {
	t.Helper()
	root := projectRoot(t)
	c := mustBlueprintCase(t, name)
	got := generateBlueprintJSON(t, root, c)

	var doc blueprintJSON
	require.NoError(t, json.Unmarshal(got, &doc))
	return doc
}

// assertBlueprintHeader checks the shared envelope and test case dimensions.
func assertBlueprintHeader(
	t *testing.T,
	doc blueprintJSON,
	label string,
	wantEntities int,
	wantWires int,
) {
	t.Helper()
	assertBlueprintIdentity(t, doc, label)
	require.Len(t, doc.Blueprint.Entities, wantEntities)
	require.Len(t, doc.Blueprint.Wires, wantWires)
}

// assertBlueprintIdentity checks the shared envelope and function label.
func assertBlueprintIdentity(t *testing.T, doc blueprintJSON, label string) {
	t.Helper()
	assert.Equal(t, "blueprint", doc.Blueprint.Item)
	assert.Equal(t, label, doc.Blueprint.Label)
	assert.Equal(t, blueprintVersion, doc.Blueprint.Version)
}

// generateBlueprintJSON centralises invocation of the production blueprint
// command for integration tests and returns its readable JSON output.
func generateBlueprintJSON(
	t *testing.T,
	root string,
	c blueprintCase,
) []byte {
	t.Helper()
	source, err := c.sourcePath(root)
	require.NoError(t, err)

	var buf bytes.Buffer
	command := app.NewCommand()
	command.Writer = &buf
	require.NoError(
		t,
		command.Run(
			context.Background(),
			c.cliArgs(source),
		),
	)
	return bytes.Clone(buf.Bytes())
}

// mustBlueprintCase keeps integration tests tied to the shared test case
// manifest.
func mustBlueprintCase(t *testing.T, name string) blueprintCase {
	t.Helper()
	c, ok := findBlueprintCase(name)
	require.True(t, ok, "blueprint case %q is not in the manifest", name)
	return c
}

// projectRoot anchors test case paths independently of the caller's directory.
func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	return root
}

// displayPanelTexts exposes the blueprint's human-facing explanation order.
func displayPanelTexts(doc blueprintJSON) []string {
	labels := make([]string, 0, len(doc.Blueprint.Entities))
	for _, ent := range doc.Blueprint.Entities {
		if ent.Name == "display-panel" && ent.Text != "" {
			labels = append(labels, ent.Text)
		}
	}
	return labels
}

// entitiesByNumber gives layout assertions stable entity lookup while also
// rejecting duplicate identifiers.
func entitiesByNumber(t *testing.T, doc blueprintJSON) map[int]blueprintEntity {
	t.Helper()
	entities := make(map[int]blueprintEntity, len(doc.Blueprint.Entities))
	for _, ent := range doc.Blueprint.Entities {
		_, seen := entities[ent.EntityNumber]
		require.False(t, seen, "duplicate entity %d", ent.EntityNumber)
		entities[ent.EntityNumber] = ent
	}
	require.Len(t, entities, len(doc.Blueprint.Entities))
	return entities
}

// signalName lets assertions compare optional signals without nil handling.
func signalName(sig *blueprintSignal) string {
	if sig == nil {
		return ""
	}
	return sig.Name
}

// assertConstant checks a constant combinator's single editable filter.
func assertConstant(
	t *testing.T,
	ent blueprintEntity,
	signal string,
	count int,
) {
	t.Helper()

	// Entity kind.
	require.Equal(t, "constant-combinator", ent.Name)
	require.NotNil(t, ent.ControlBehavior)

	// Section shape.
	sections := ent.ControlBehavior.Sections
	require.NotNil(t, sections)
	require.Len(t, sections.Sections, 1)
	require.Len(t, sections.Sections[0].Filters, 1)
	filter := sections.Sections[0].Filters[0]
	assert.Equal(t, 1, sections.Sections[0].Index)

	// Filter payload. A parameter rides a virtual letter (signal-A); a literal
	// constant rides an item signal (iron-plate).
	expectedType := "item"
	if strings.HasPrefix(signal, "signal-") {
		expectedType = "virtual"
	}
	assert.Equal(t, 1, filter.Index)
	assert.Equal(t, expectedType, filter.Type)
	assert.Equal(t, signal, filter.Name)
	assert.Equal(t, "normal", filter.Quality)
	assert.Equal(t, "=", filter.Comparator)
	assert.Equal(t, count, filter.Count)
}

// assertArithmetic checks one arithmetic combinator's operation and output.
func assertArithmetic(
	t *testing.T,
	ent blueprintEntity,
	first string,
	op string,
	second int,
	output string,
) {
	t.Helper()

	// Entity kind.
	require.Equal(t, "arithmetic-combinator", ent.Name)
	require.NotNil(t, ent.ControlBehavior)

	// Operation payload.
	ac := ent.ControlBehavior.ArithmeticConditions
	require.NotNil(t, ac)
	assert.Equal(t, first, signalName(ac.FirstSignal))
	assert.Equal(t, op, ac.Operation)
	require.NotNil(t, ac.SecondConstant)
	assert.Equal(t, second, *ac.SecondConstant)
	assert.Equal(t, output, signalName(ac.OutputSignal))
	assert.Nil(t, ac.SecondSignal)
	assert.Nil(t, ac.FirstConstant)
}

// assertArithmeticSignals checks an arithmetic combinator whose second operand
// is a signal rather than a constant, such as a + b.
func assertArithmeticSignals(
	t *testing.T,
	ent blueprintEntity,
	first string,
	op string,
	second string,
	output string,
) {
	t.Helper()

	require.Equal(t, "arithmetic-combinator", ent.Name)
	require.NotNil(t, ent.ControlBehavior)

	ac := ent.ControlBehavior.ArithmeticConditions
	require.NotNil(t, ac)
	assert.Equal(t, first, signalName(ac.FirstSignal))
	assert.Equal(t, op, ac.Operation)
	assert.Equal(t, second, signalName(ac.SecondSignal))
	assert.Equal(t, output, signalName(ac.OutputSignal))
	assert.Nil(t, ac.FirstConstant)
	assert.Nil(t, ac.SecondConstant)
}

// assertDecider checks one decider's condition, output, and copy-count mode.
func assertDecider(
	t *testing.T,
	ent blueprintEntity,
	first string,
	comparator string,
	constant int,
	output string,
	copyCount bool,
) {
	t.Helper()

	// Entity kind.
	require.Equal(t, "decider-combinator", ent.Name)
	require.NotNil(t, ent.ControlBehavior)

	// Single condition and output.
	dc := ent.ControlBehavior.DeciderConditions
	require.NotNil(t, dc)
	require.Len(t, dc.Conditions, 1)
	require.Len(t, dc.Outputs, 1)
	cond := dc.Conditions[0]
	out := dc.Outputs[0]
	assert.Equal(t, first, signalName(cond.FirstSignal))
	assert.Equal(t, comparator, cond.Comparator)

	// Condition payload.
	require.NotNil(t, cond.Constant)
	assert.Equal(t, constant, *cond.Constant)
	assert.Nil(t, cond.SecondSignal)

	// Output payload.
	assert.Equal(t, output, signalName(out.Signal))
	assert.Equal(t, copyCount, out.CopyCountFromInput)
}

// assertDigitPanel checks one digit panel's 0-9 icon ladder.
func assertDigitPanel(
	t *testing.T,
	ent blueprintEntity,
	signal string,
) {
	t.Helper()

	// Entity kind.
	require.Equal(t, "display-panel", ent.Name)
	assert.True(t, ent.AlwaysShow)
	assert.Empty(t, ent.Text)
	require.NotNil(t, ent.ControlBehavior)

	// The ladder has one message per visible digit.
	params := ent.ControlBehavior.Parameters
	require.Len(t, params, 10)
	for v, msg := range params {
		assert.Empty(t, msg.Text)
		assert.Equal(t, fmt.Sprintf("signal-%d", v), signalName(msg.Icon))
		require.NotNil(t, msg.Condition)
		assert.Equal(t, signal, signalName(msg.Condition.FirstSignal))
		assert.Equal(t, "=", msg.Condition.Comparator)

		// Factorio omits a zero constant; absent means zero here.
		if v == 0 {
			assert.Nil(t, msg.Condition.Constant)
			continue
		}
		require.NotNil(t, msg.Condition.Constant)
		assert.Equal(t, v, *msg.Condition.Constant)
	}
}

// assertNumericDigitDisplay checks the standard eight-stage decimal readout.
func assertNumericDigitDisplay(
	t *testing.T,
	entities map[int]blueprintEntity,
	firstDiv int,
	firstInput string,
) {
	t.Helper()
	for k := range 8 {
		// Stage zero reads the public result; later stages read the private chain.
		value := "signal-dot"
		if k == 0 {
			value = firstInput
		}
		div := firstDiv + k*3
		mod := div + 1
		panel := div + 2
		assertArithmetic(t, entities[div], value, "/", 10, "signal-dot")
		assertArithmetic(t, entities[mod], value, "%", 10,
			fmt.Sprintf("signal-%d", k))
		assertDigitPanel(t, entities[panel],
			fmt.Sprintf("signal-%d", 7-k))
	}
}

// assertBoolDisplay checks the boolean readout panel: a check icon when the
// result is 1 (true) and a deny icon when it is -1 (false), both reading the
// result signal.
func assertBoolDisplay(t *testing.T, ent blueprintEntity, resultSignal string) {
	t.Helper()

	require.Equal(t, "display-panel", ent.Name)
	assert.True(t, ent.AlwaysShow)
	assert.Empty(t, ent.Text)
	require.NotNil(t, ent.ControlBehavior)

	params := ent.ControlBehavior.Parameters
	require.Len(t, params, 2)
	for i, want := range []struct {
		icon     string
		constant int
	}{
		{"signal-check", 1},
		{"signal-deny", -1},
	} {
		assert.Empty(t, params[i].Text)
		assert.Equal(t, want.icon, signalName(params[i].Icon))
		require.NotNil(t, params[i].Condition)
		assert.Equal(t, resultSignal, signalName(params[i].Condition.FirstSignal))
		assert.Equal(t, "=", params[i].Condition.Comparator)
		require.NotNil(t, params[i].Condition.Constant)
		assert.Equal(t, want.constant, *params[i].Condition.Constant)
	}
}

// assertPoweredBySubstation checks every powered entity is supplied.
func assertPoweredBySubstation(
	t *testing.T,
	doc blueprintJSON,
	substation blueprintEntity,
	wantPowered int,
) {
	t.Helper()
	powered := 0
	for _, ent := range doc.Blueprint.Entities {
		if !isPoweredEntity(ent) {
			continue
		}
		powered++
		for _, cell := range entityCells(ent) {
			assert.Truef(t,
				substationCovers(substation.Position, cell),
				"substation does not cover entity %d cell %v",
				ent.EntityNumber,
				cell,
			)
		}
	}
	assert.Equal(t, wantPowered, powered)
}

// assertNoRelayPoles checks that a compact test case needs no circuit relays.
func assertNoRelayPoles(t *testing.T, doc blueprintJSON) {
	t.Helper()
	assertRelayPoleCount(t, doc, 0)
}

// assertRelayPoleCount checks how many circuit relays routing inserted.
func assertRelayPoleCount(t *testing.T, doc blueprintJSON, want int) {
	t.Helper()
	got := 0
	for _, ent := range doc.Blueprint.Entities {
		if ent.Name == "medium-electric-pole" {
			got++
		}
	}
	assert.Equal(t, want, got)
}

// assertSingleSubstationPower checks the sole power source and its coverage.
func assertSingleSubstationPower(
	t *testing.T,
	doc blueprintJSON,
	entityNumber int,
	wantPowered int,
) {
	t.Helper()
	var substations int
	for _, ent := range doc.Blueprint.Entities {
		if ent.Name == "substation" {
			substations++
		}
	}
	require.Equal(t, 1, substations)
	substation := entitiesByNumber(t, doc)[entityNumber]
	require.Equal(t, "substation", substation.Name)
	assert.Equal(t, "legendary", substation.Quality)
	assertPoweredBySubstation(t, doc, substation, wantPowered)
}

// assertPoweredByAnySubstation checks every powered entity is within reach
// of at least one substation. Wide blueprints need more than one substation, so
// coverage is satisfied when any of them supplies the cell.
func assertPoweredByAnySubstation(
	t *testing.T,
	doc blueprintJSON,
	wantSubstations int,
	wantPowered int,
) {
	t.Helper()
	subs := substationPositions(doc.Blueprint.Entities)
	require.Len(t, subs, wantSubstations)

	powered := 0
	for _, ent := range doc.Blueprint.Entities {
		if !isPoweredEntity(ent) {
			continue
		}
		powered++
		assertPoweredByAny(t, ent, subs)
	}
	assert.Equal(t, wantPowered, powered)
}

// substationPositions provides the supply points used by coverage checks.
func substationPositions(entities []blueprintEntity) []blueprintPosition {
	var positions []blueprintPosition
	for _, ent := range entities {
		if ent.Name == "substation" {
			positions = append(positions, ent.Position)
		}
	}
	return positions
}

// isPoweredEntity limits supply checks to entities that consume electricity.
func isPoweredEntity(ent blueprintEntity) bool {
	return ent.Name == "arithmetic-combinator" ||
		ent.Name == "decider-combinator" ||
		ent.Name == "small-lamp"
}

// assertPoweredByAny ensures every occupied tile of an entity has a supplier.
func assertPoweredByAny(
	t *testing.T,
	ent blueprintEntity,
	substations []blueprintPosition,
) {
	t.Helper()
	for _, cell := range entityCells(ent) {
		assert.Truef(
			t,
			coveredByAnySubstation(substations, cell),
			"entity %d cell %v not covered by any substation",
			ent.EntityNumber,
			cell,
		)
	}
}

// coveredByAnySubstation supports layouts whose power coverage overlaps.
func coveredByAnySubstation(
	substations []blueprintPosition,
	cell gridCell,
) bool {
	for _, position := range substations {
		if substationCovers(position, cell) {
			return true
		}
	}
	return false
}

// entityCells mirrors the generator's tile occupancy calculation.
func entityCells(ent blueprintEntity) []gridCell {
	px, py := ent.Position.X, ent.Position.Y
	switch ent.Name {
	case "constant-combinator", "display-panel", "medium-electric-pole",
		"small-lamp":
		return []gridCell{{int(math.Floor(px)), int(math.Floor(py))}}
	case "substation":
		left, top := int(math.Round(px-1)), int(math.Round(py-1))
		return []gridCell{
			{left, top},
			{left + 1, top},
			{left, top + 1},
			{left + 1, top + 1},
		}
	}

	// Combinators are 2x1 facing east or west, and 1x2 facing north or south.
	w, h := 1, 2
	if ent.Direction == 4 || ent.Direction == 12 {
		w, h = 2, 1
	}
	left := int(math.Round(px - float64(w)/2))
	top := int(math.Round(py - float64(h)/2))
	cells := make([]gridCell, 0, w*h)
	for i := range w {
		for j := range h {
			cells = append(cells, gridCell{left + i, top + j})
		}
	}
	return cells
}

// substationCovers checks one occupied tile against the supply square.
func substationCovers(p blueprintPosition, cell gridCell) bool {
	cellCentre := blueprintPosition{
		X: float64(cell.x) + 0.5,
		Y: float64(cell.y) + 0.5,
	}
	return math.Abs(p.X-cellCentre.X) <= 14 &&
		math.Abs(p.Y-cellCentre.Y) <= 14
}
