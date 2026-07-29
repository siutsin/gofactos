// This file protects generic recursion, status, and stack layout contracts.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siutsin/gofactos/internal/app"
)

const clockStartControlLabel = "START / RESET"

// TestRecursiveCaseBlueprintJSON drives every generic recursive test case
// through the blueprint command and validates its JSON contract directly.
func TestRecursiveCaseBlueprintJSON(t *testing.T) {
	cases := []struct {
		name, file, function string
		params               map[string]int
		inputCounts          []int
		booleanResult        bool
	}{
		{
			name: "Fibonacci", file: "fibonacci.go",
			function: "fibonacci", params: map[string]int{"n": 10},
			inputCounts: []int{10},
		},
		{
			name: "factorial", file: "recursive/factorial.go",
			function: "factorial", params: map[string]int{"n": 5},
			inputCounts: []int{5},
		},
		{
			name: "greatest common divisor", file: "recursive/gcd.go",
			function: "gcd", params: map[string]int{"a": 48, "b": 18},
			inputCounts: []int{48, 18},
		},
		{
			name: "boolean recursion", file: "recursive/reacheszero.go",
			function: "reachesZero", params: map[string]int{"n": 5},
			inputCounts: []int{5}, booleanResult: true,
		},
		{
			name:     "phi merge and unused argument",
			file:     "recursive/weighted.go",
			function: "weighted",
			params: map[string]int{
				"n": 4, "double": 1, "ignored": 9,
			},
			inputCounts: []int{4, 1, 9},
		},
	}
	positions := make([]float64, len(cases))
	t.Run("cases", func(t *testing.T) {
		for i, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				doc := runRecursiveCaseJSON(
					t,
					tc.file,
					tc.function,
					tc.params,
				)
				assertBlueprintIdentity(t, doc, tc.function)
				require.NotEmpty(t, doc.Blueprint.Entities)
				require.NotEmpty(t, doc.Blueprint.Wires)
				assertClockStartControl(t, doc)
				assertRecursiveInputs(t, doc, tc.inputCounts)
				assertRecursiveResultDisplay(t, doc, tc.booleanResult)
				assertRecursiveActivityLamps(t, doc)
				assertRecursiveLabels(t, doc, tc.function)
				positions[i] = assertRecursiveLayout(t, doc)
			})
		}
	})
	framePositions := make(map[float64]bool)
	for _, position := range positions {
		framePositions[position] = true
	}
	require.Greater(t, len(framePositions), 1)
}

// assertClockStartControl ensures clocked blueprints wait for construction and
// expose exactly one default-off control.
func assertClockStartControl(t *testing.T, doc blueprintJSON) {
	t.Helper()
	controls := clockStartControls(doc)
	require.Len(t, controls, 1)
	control := controls[0]
	require.NotNil(t, control.ControlBehavior)
	require.NotNil(t, control.ControlBehavior.IsOn)
	assert.False(t, *control.ControlBehavior.IsOn)
	require.NotNil(t, control.ControlBehavior.Sections)
	require.Len(t, control.ControlBehavior.Sections.Sections, 1)
	section := control.ControlBehavior.Sections.Sections[0]
	assert.Equal(t, 1, section.Index)
	require.Len(t, section.Filters, 1)
	filter := section.Filters[0]
	assert.Equal(t, 1, filter.Index)
	assert.NotEmpty(t, filter.Type)
	assert.NotEmpty(t, filter.Name)
	assert.Equal(t, "normal", filter.Quality)
	assert.Equal(t, "=", filter.Comparator)
	assert.Equal(t, 1, filter.Count)

	labelCount := 0
	for _, label := range displayPanelTexts(doc) {
		if label == clockStartControlLabel {
			labelCount++
		}
	}
	assert.Equal(t, 1, labelCount)
}

// clockStartControls isolates manually operated controls from ordinary
// constants.
func clockStartControls(doc blueprintJSON) []blueprintEntity {
	var controls []blueprintEntity
	for _, entity := range doc.Blueprint.Entities {
		if entity.Name == "constant-combinator" &&
			entity.ControlBehavior != nil &&
			entity.ControlBehavior.IsOn != nil {
			controls = append(controls, entity)
		}
	}
	return controls
}

// assertRecursiveActivityLamps protects visible progress for controller steps
// and active frames.
func assertRecursiveActivityLamps(t *testing.T, doc blueprintJSON) {
	t.Helper()
	pureGreen := blueprintColour{G: 1, A: 1}
	substations := substationPositions(doc.Blueprint.Entities)
	require.NotEmpty(t, substations)
	controllerLamps := 0
	frameLamps := 0
	for _, entity := range doc.Blueprint.Entities {
		if entity.Name != "small-lamp" {
			continue
		}
		require.NotNil(t, entity.Colour)
		assert.Equal(t, pureGreen, *entity.Colour)
		assertPoweredByAny(t, entity, substations)
		require.NotNil(t, entity.ControlBehavior)
		assert.True(t, entity.ControlBehavior.CircuitEnabled)
		condition := entity.ControlBehavior.CircuitCondition
		require.NotNil(t, condition)
		assert.Equal(t, "signal-check", signalName(condition.FirstSignal))
		assert.Equal(t, "=", condition.Comparator)
		require.NotNil(t, condition.Constant)
		switch *condition.Constant {
		case 1:
			controllerLamps++
		case 2:
			frameLamps++
		default:
			assert.Failf(
				t,
				"unexpected activity lamp condition",
				"entity %d uses signal-check = %d",
				entity.EntityNumber,
				*condition.Constant,
			)
		}
	}
	require.Positive(t, controllerLamps)
	assert.Equal(t, 13, frameLamps)
}

// assertRecursiveLabels keeps recursive-machine terminology concise and
// useful to players.
func assertRecursiveLabels(
	t *testing.T,
	doc blueprintJSON,
	function string,
) {
	t.Helper()
	labels := displayPanelTexts(doc)
	for _, want := range []string{
		"output / status",
		recursiveFrameLabel(0),
	} {
		assert.Contains(t, labels, want)
	}
	pcLabels := 0
	for _, label := range labels {
		if strings.HasPrefix(label, "PC ") {
			pcLabels++
		}
	}
	require.Positive(t, pcLabels)
	for depth := 1; depth <= 12; depth++ {
		assert.Contains(t, labels, recursiveFrameLabel(depth))
	}
	for _, label := range labels {
		if !strings.HasPrefix(label, "frame ") {
			continue
		}
		assert.NotContains(t, label, "call depth")
		assert.NotContains(t, label, "root call")
	}
	for _, removed := range []string{
		"recursive " + function + " machine",
		"controller",
		"green = current step",
		"call stack",
		"one frame = one live call",
		"green = current frame",
		"depth",
		"state",
		"child result",
		"final result",
	} {
		assert.NotContains(t, labels, removed)
	}
}

// recursiveFrameLabel centralises the public naming contract for stack rows.
func recursiveFrameLabel(depth int) string {
	if depth == 0 {
		return "frame 00: root"
	}
	return fmt.Sprintf("frame %02d", depth)
}

// assertRecursiveLayout protects the readable status-above-stack arrangement.
func assertRecursiveLayout(t *testing.T, doc blueprintJSON) float64 {
	t.Helper()
	wanted := map[string]struct{}{
		"output / status": {},
		"RUNNING":         {},
		"DONE":            {},
		"STACK OVERFLOW":  {},
	}
	for depth := range 13 {
		wanted[recursiveFrameLabel(depth)] = struct{}{}
	}

	positions := recursivePanelPositions(t, doc, wanted)
	frameLamps := recursiveFrameLampPositions(doc)
	maxPCX, foundPC := recursiveMaxPCX(doc)
	require.True(t, foundPC)
	require.Len(t, positions, len(wanted))

	statusNames := []string{
		"output / status",
		"RUNNING",
		"DONE",
		"STACK OVERFLOW",
	}
	statusY := positions[statusNames[0]].Y
	for _, name := range statusNames {
		assert.InDelta(t, statusY, positions[name].Y, 0)
	}
	for _, pair := range [][2]string{
		{"output / status", "RUNNING"},
		{"RUNNING", "DONE"},
		{"DONE", "STACK OVERFLOW"},
	} {
		assert.Less(t, positions[pair[0]].X, positions[pair[1]].X)
	}
	assert.InDelta(
		t,
		6,
		positions["RUNNING"].X-positions["output / status"].X,
		0,
	)
	assert.InDelta(t, 4, positions["DONE"].X-positions["RUNNING"].X, 0)
	assert.InDelta(
		t,
		4,
		positions["STACK OVERFLOW"].X-positions["DONE"].X,
		0,
	)

	root := positions[recursiveFrameLabel(0)]
	assert.InDelta(t, root.Y-4, statusY, 0)
	assert.Greater(t, root.X, maxPCX)
	controllerMaxX, foundController := recursiveControllerMaxX(doc, root.X)
	require.True(t, foundController)
	controllerGap := root.X - controllerMaxX
	assert.GreaterOrEqual(t, controllerGap, 4.0)
	assert.LessOrEqual(t, controllerGap, 5.0)
	require.Len(t, frameLamps, 13)
	sort.Slice(frameLamps, func(i, j int) bool {
		return frameLamps[i].Y < frameLamps[j].Y
	})
	for _, name := range statusNames {
		assert.GreaterOrEqual(t, positions[name].X, root.X)
	}
	for depth := range 13 {
		position := positions[recursiveFrameLabel(depth)]
		assert.InDelta(t, root.X, position.X, 0)
		assert.InDelta(t, root.Y+float64(depth*4), position.Y, 0)
		assert.InDelta(t, frameLamps[0].X, frameLamps[depth].X, 0)
		assert.InDelta(t, position.Y, frameLamps[depth].Y, 0)
	}
	assert.Greater(t, frameLamps[0].X, root.X)
	return root.X
}

// recursiveControllerMaxX exposes the controller edge used to bound layout
// gaps.
func recursiveControllerMaxX(
	doc blueprintJSON,
	frameX float64,
) (float64, bool) {
	maxX := 0.0
	found := false
	for _, entity := range doc.Blueprint.Entities {
		if entity.Position.X >= frameX ||
			entity.Name == "medium-electric-pole" ||
			entity.Name == "substation" {
			continue
		}
		maxX = max(maxX, entity.Position.X)
		found = true
	}
	return maxX, found
}

// recursivePanelPositions provides stable lookup for user-visible layout
// assertions.
func recursivePanelPositions(
	t *testing.T,
	doc blueprintJSON,
	wanted map[string]struct{},
) map[string]blueprintPosition {
	t.Helper()
	positions := make(map[string]blueprintPosition, len(wanted))
	for _, entity := range doc.Blueprint.Entities {
		if entity.Name != "display-panel" {
			continue
		}
		for _, text := range recursivePanelTexts(entity) {
			if _, ok := wanted[text]; !ok {
				continue
			}
			require.NotContains(t, positions, text)
			positions[text] = entity.Position
		}
	}
	return positions
}

// recursiveFrameLampPositions exposes the activation indicators for alignment
// checks.
func recursiveFrameLampPositions(doc blueprintJSON) []blueprintPosition {
	var positions []blueprintPosition
	for _, entity := range doc.Blueprint.Entities {
		if !isRecursiveFrameLamp(entity) {
			continue
		}
		positions = append(positions, entity.Position)
	}
	return positions
}

// isRecursiveFrameLamp distinguishes frame indicators from controller lamps.
func isRecursiveFrameLamp(entity blueprintEntity) bool {
	if entity.Name != "small-lamp" || entity.ControlBehavior == nil {
		return false
	}
	condition := entity.ControlBehavior.CircuitCondition
	return condition != nil && condition.Constant != nil &&
		*condition.Constant == 2
}

// recursiveMaxPCX locates the instruction-label edge that frames must clear.
func recursiveMaxPCX(doc blueprintJSON) (float64, bool) {
	maxX := 0.0
	found := false
	for _, entity := range doc.Blueprint.Entities {
		if entity.Name != "display-panel" ||
			!strings.HasPrefix(entity.Text, "PC ") {
			continue
		}
		maxX = max(maxX, entity.Position.X)
		found = true
	}
	return maxX, found
}

// recursivePanelTexts exposes both fixed and conditional display messages.
func recursivePanelTexts(entity blueprintEntity) []string {
	texts := []string{entity.Text}
	if entity.ControlBehavior == nil {
		return texts
	}
	for _, parameter := range entity.ControlBehavior.Parameters {
		texts = append(texts, parameter.Text)
	}
	return texts
}

// assertRecursiveResultDisplay protects the result readout appropriate to the
// function's return type.
func assertRecursiveResultDisplay(
	t *testing.T,
	doc blueprintJSON,
	booleanResult bool,
) {
	t.Helper()
	var booleanPanels []blueprintEntity
	digitPanels := make(map[string]blueprintEntity)
	for _, entity := range doc.Blueprint.Entities {
		if entity.Name != "display-panel" || entity.ControlBehavior == nil {
			continue
		}
		params := entity.ControlBehavior.Parameters
		switch len(params) {
		case 2:
			booleanPanels = append(booleanPanels, entity)
		case 10:
			require.NotNil(t, params[0].Condition)
			require.NotNil(t, params[0].Condition.FirstSignal)
			signal := signalName(params[0].Condition.FirstSignal)
			require.NotContains(t, digitPanels, signal)
			digitPanels[signal] = entity
		}
	}

	if booleanResult {
		require.Len(t, booleanPanels, 1)
		assert.Empty(t, digitPanels)
		assertBoolDisplay(t, booleanPanels[0], "iron-plate")
		assertRecursiveBoolDisplayPosition(t, doc, booleanPanels[0])
		return
	}

	assert.Empty(t, booleanPanels)
	require.Len(t, digitPanels, 8)
	for digit := range 8 {
		signal := fmt.Sprintf("signal-%d", digit)
		panel, ok := digitPanels[signal]
		require.True(t, ok)
		assertDigitPanel(t, panel, signal)
	}
	assertRecursiveResultBoardPosition(t, doc, digitPanels)
}

// assertRecursiveResultBoardPosition keeps numeric output beside status.
func assertRecursiveResultBoardPosition(
	t *testing.T,
	doc blueprintJSON,
	digitPanels map[string]blueprintEntity,
) {
	t.Helper()
	leftmostX := 0.0
	foundDigit := false
	for _, panel := range digitPanels {
		if !foundDigit || panel.Position.X < leftmostX {
			leftmostX = panel.Position.X
			foundDigit = true
		}
	}
	require.True(t, foundDigit)

	overflow := recursiveOverflowPosition(t, doc)
	assert.InDelta(t, overflow.X+2, leftmostX, 0)
}

// assertRecursiveBoolDisplayPosition keeps boolean output beside status.
func assertRecursiveBoolDisplayPosition(
	t *testing.T,
	doc blueprintJSON,
	panel blueprintEntity,
) {
	t.Helper()
	overflow := recursiveOverflowPosition(t, doc)
	assert.InDelta(t, overflow.X+2, panel.Position.X, 0)
	assert.InDelta(t, overflow.Y, panel.Position.Y, 0)
}

// recursiveOverflowPosition anchors result-display proximity checks.
func recursiveOverflowPosition(
	t *testing.T,
	doc blueprintJSON,
) blueprintPosition {
	t.Helper()
	var position blueprintPosition
	found := false
	for _, entity := range doc.Blueprint.Entities {
		if entity.Name != "display-panel" || entity.ControlBehavior == nil {
			continue
		}
		for _, message := range entity.ControlBehavior.Parameters {
			if message.Text != "STACK OVERFLOW" {
				continue
			}
			require.False(t, found)
			position = entity.Position
			found = true
		}
	}
	require.True(t, found)
	return position
}

// assertRecursiveInputs protects both machine values and their player-facing
// labels.
func assertRecursiveInputs(
	t *testing.T,
	doc blueprintJSON,
	counts []int,
) {
	t.Helper()
	wantCounts := make(map[string]int, len(counts))
	wantPanels := make(map[string]string, len(counts))
	for index, count := range counts {
		letter := rune('A' + index)
		signal := fmt.Sprintf("signal-%c", letter)
		wantCounts[signal] = count
		wantPanels[signal] = fmt.Sprintf("%c = %d", letter, count)
	}
	assert.Equal(t, wantCounts, recursiveInputCounts(t, doc))
	assert.Equal(t, wantPanels, recursiveParameterPanels(t, doc))
}

// recursiveInputCounts provides a stable view of recursive call parameters.
func recursiveInputCounts(
	t *testing.T,
	doc blueprintJSON,
) map[string]int {
	t.Helper()
	counts := make(map[string]int)
	for _, entity := range doc.Blueprint.Entities {
		if entity.Name != "constant-combinator" ||
			entity.ControlBehavior == nil ||
			entity.ControlBehavior.IsOn != nil ||
			entity.ControlBehavior.Sections == nil {
			continue
		}
		for _, section := range entity.ControlBehavior.Sections.Sections {
			for _, filter := range section.Filters {
				if filter.Type != "virtual" ||
					!strings.HasPrefix(filter.Name, "signal-") {
					continue
				}
				require.NotContains(t, counts, filter.Name)
				counts[filter.Name] = filter.Count
			}
		}
	}
	return counts
}

// recursiveParameterPanels ties editable parameters to their visible labels.
func recursiveParameterPanels(
	t *testing.T,
	doc blueprintJSON,
) map[string]string {
	t.Helper()
	panels := make(map[string]string)
	for _, entity := range doc.Blueprint.Entities {
		if entity.Name != "display-panel" || entity.Icon == nil {
			continue
		}
		require.NotContains(t, panels, entity.Icon.Name)
		panels[entity.Icon.Name] = entity.Text
	}
	return panels
}

// runRecursiveCaseJSON exercises generic recursion through the blueprint
// command.
func runRecursiveCaseJSON(
	t *testing.T,
	file, function string,
	params map[string]int,
) blueprintJSON {
	t.Helper()
	root := projectRoot(t)
	args := []string{
		"gofactos",
		"blueprint",
		"--json",
		"--func",
		function,
	}
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		args = append(
			args,
			"--set",
			name+"="+strconv.Itoa(params[name]),
		)
	}
	args = append(args, filepath.Join(root, "internal", "testdata", file))

	var output bytes.Buffer
	command := app.NewCommand()
	command.Writer = &output
	require.NoError(t, command.Run(context.Background(), args))

	var doc blueprintJSON
	require.NoError(t, json.Unmarshal(output.Bytes(), &doc))
	return doc
}
