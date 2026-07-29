// This file protects concise labels that make generated circuits readable.
package factorio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vsig builds a virtual signal for label tests.
func vsig(name string) *signalID { return &signalID{Type: "virtual", Name: name} }

// testLabeller renders labels with no SSA-name aliases: the unit tests use
// virtual letter signals, not item signals, so signalLabel falls back to the
// plain name.
var testLabeller = &labeller{}

// labelText exposes standalone label rendering to focused tests.
func (l *labeller) labelText(
	ent entity,
) (text string, icon *signalID, ok bool) {
	return l.labelTextForOwner(ent, nil)
}

// labelTextFor exposes contextual panel selection to focused tests.
func (l *labeller) labelTextFor(
	ent entity,
	owner *instance,
) (string, bool) {
	panel, ok := l.labelFor(ent, owner)
	return panel.text, ok
}

// TestLabelText proves the fallback panel text is read straight from a
// combinator's control behaviour for every combinator shape the modules emit.
func TestLabelText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ent  entity
		text string
		icon string
	}{
		{
			name: "arith two signals",
			ent: entity{Name: "arithmetic-combinator", ControlBehavior: &controlBehavior{
				ArithmeticConditions: &arithmeticConditions{
					FirstSignal: vsig("signal-A"), Operation: "+",
					SecondSignal: vsig("signal-B"), OutputSignal: vsig("signal-C"),
				}}},
			text: "C = A + B", icon: "signal-C",
		},
		{
			name: "neg signal times constant",
			ent: entity{Name: "arithmetic-combinator", ControlBehavior: &controlBehavior{
				ArithmeticConditions: &arithmeticConditions{
					FirstSignal: vsig("signal-A"), Operation: "*",
					SecondConstant: new(-1), OutputSignal: vsig("signal-D"),
				}}},
			text: "D = -A", icon: "signal-D",
		},
		{
			name: "decider fixed one",
			ent: entity{Name: "decider-combinator", ControlBehavior: &controlBehavior{
				DeciderConditions: &deciderConditions{
					Conditions: []deciderCondition{{FirstSignal: vsig("signal-A"), Comparator: "<", Constant: new(0)}},
					Outputs:    []deciderOutput{{Signal: vsig("signal-E"), CopyCountFromInput: false}},
				}}},
			text: "E = A < 0", icon: "signal-E",
		},
		{
			name: "decider copy count",
			ent: entity{Name: "decider-combinator", ControlBehavior: &controlBehavior{
				DeciderConditions: &deciderConditions{
					Conditions: []deciderCondition{{FirstSignal: vsig("signal-F"), Comparator: "=", Constant: new(1)}},
					Outputs:    []deciderOutput{{Signal: vsig("signal-G"), CopyCountFromInput: true}},
				}}},
			text: "G = G if F = true", icon: "signal-G",
		},
		{
			name: "constant value",
			ent: entity{Name: "constant-combinator", ControlBehavior: &controlBehavior{
				Sections: &constantCombinatorSections{Sections: []logisticSection{{Index: 1, Filters: []constantFilter{
					{Index: 1, Type: "virtual", Name: "signal-A", Quality: "normal", Comparator: "=", Count: 7},
				}}}},
			}},
			text: "A = 7", icon: "signal-A",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, icon, ok := testLabeller.labelText(tc.ent)
			require.True(t, ok)
			assert.Equal(t, tc.text, text)
			require.NotNil(t, icon)
			assert.Equal(t, tc.icon, icon.Name)
		})
	}
}

// TestFriendlyLabelTextHidesPrivateSignals proves composite-module labels
// explain the circuit step instead of exposing scratch signals like info.
func TestFriendlyLabelTextHidesPrivateSignals(t *testing.T) {
	t.Parallel()
	in := newInstance(newCompare("<", 0))
	in.dir = dirEast
	in.pos = anchorPos(in, 0, 0, dirEast)
	in.port("a").net = &netlistNet{signal: signalID{Type: "virtual", Name: "signal-X"}}
	in.port("cond").net = &netlistNet{signal: signalID{Type: "virtual", Name: "signal-Z"}}

	e := newEmitter()
	in.comp.build(e, in)

	got := make([]string, 0, len(e.entities))
	for _, ent := range e.entities {
		text, ok := testLabeller.labelTextFor(ent, in)
		require.True(t, ok)
		assert.NotContains(t, text, "info")
		assert.NotContains(t, text, "scratch")
		got = append(got, text)
	}
	assert.Equal(t, []string{
		"Z = X < 0",
		"if X ≥ 0",
		"Z = false",
	}, got)
}

// TestCompareLabelPreservesOperandType proves the compare module keeps an
// integer equality's literal value and spells the 1/-1 sentinels as true/false
// only for a Boolean operand, so `n == 1` never teaches the false label
// `n = true`.
func TestCompareLabelPreservesOperandType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		op       string
		constant int
		boolean  bool
		want     []string
	}{
		{
			"integer equals one", "=", 1, false,
			[]string{"Z = X = 1", "if X ≠ 1", "Z = false"},
		},
		{
			"integer equals minus one", "=", -1, false,
			[]string{"Z = X = -1", "if X ≠ -1", "Z = false"},
		},
		{
			"boolean equals true", "=", 1, true,
			[]string{"Z = X = true", "if X ≠ true", "Z = false"},
		},
		{
			"boolean not equal false", "≠", -1, true,
			[]string{"Z = X ≠ false", "if X = false", "Z = false"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmp := newCompare(tc.op, tc.constant)
			cmp.boolean = tc.boolean
			in := newInstance(cmp)
			in.dir = dirEast
			in.pos = anchorPos(in, 0, 0, dirEast)
			in.port("a").net = &netlistNet{signal: signalID{Type: "virtual", Name: "signal-X"}}
			in.port("cond").net = &netlistNet{signal: signalID{Type: "virtual", Name: "signal-Z"}}

			e := newEmitter()
			in.comp.build(e, in)

			got := make([]string, 0, len(e.entities))
			for _, ent := range e.entities {
				text, ok := testLabeller.labelTextFor(ent, in)
				require.True(t, ok)
				got = append(got, text)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestPhiLabelTextNamesSelectedValues proves the phi labels name the branch
// values while distinguishing normalisation from the branch choice.
func TestPhiLabelTextNamesSelectedValues(t *testing.T) {
	t.Parallel()
	in := newInstance(&phi{})
	in.dir = dirEast
	in.pos = anchorPos(in, 0, 0, dirEast)
	in.port("cond").net = &netlistNet{signal: signalID{Type: "virtual", Name: "signal-K"}}
	in.port("a").net = &netlistNet{signal: signalID{Type: "virtual", Name: "signal-X"}}
	in.port("b").net = &netlistNet{signal: signalID{Type: "virtual", Name: "signal-Y"}}
	in.port("out").net = &netlistNet{signal: signalID{Type: "virtual", Name: "signal-R"}}

	e := newEmitter()
	in.comp.build(e, in)

	got := make([]string, 0, len(e.entities))
	for _, ent := range e.entities {
		text, ok := testLabeller.labelTextFor(ent, in)
		if !ok {
			continue
		}
		got = append(got, text)
	}
	assert.Equal(t, []string{
		"normalise X",
		"normalise Y",
		"if K { merge = X }",
		"if !K { merge = Y }",
		"R = merge",
	}, got)
}

// TestPhiNormalisersUsePortContext proves two caller call values retain their
// distinct names when both phi inputs share one physical signal.
func TestPhiNormalisersUsePortContext(t *testing.T) {
	t.Parallel()
	in := newInstance(&phi{})
	in.dir = dirEast
	in.pos = anchorPos(in, 0, 0, dirEast)
	setPortSignal(in, "cond", "signal-K")
	shared := &netlistNet{
		signal: signalID{Type: "virtual", Name: "signal-X"},
	}
	in.port("a").net = shared
	in.port("b").net = shared
	setPortSignal(in, "out", "signal-R")

	e := newEmitter()
	in.comp.build(e, in)
	l := &labeller{
		alias: map[string]string{
			"signal-K": "cond",
			"signal-X": "producer",
			"signal-R": "result",
		},
		portAlias: map[*port]string{
			in.port("a"): "t1",
			in.port("b"): "t2",
		},
	}

	var got []string
	for _, ent := range e.entities[:2] {
		text, ok := l.labelTextFor(ent, in)
		require.True(t, ok)
		got = append(got, text)
	}
	require.Equal(t, []string{
		"normalise t1",
		"normalise t2",
	}, got)
}

// TestRegisterShowsPhiNodeLabel proves a register gets exactly one summary panel
// naming the SSA phi node by its value wire alias and its increment by the
// constant's alias, with no scratch wire names and no per-combinator clutter.
// The label is emitted by the panel pass (after Allocate), so the test sets the
// value and increment signals and runs that pass.
func TestRegisterShowsPhiNodeLabel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		value    string
		incSig   string
		incAlias string
		want     string
	}{
		{"index", "signal-I", "signal-C", "c0", "I = φ(0, I + c0)"},
		{"result", "signal-R", "signal-D", "c1", "R = φ(0, R + c1)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			incNet := &netlistNet{signal: signalID{Type: "virtual", Name: tc.incSig}}
			in := newInstance(&register{inc: incNet})
			in.dir = dirEast
			in.pos = anchorPos(in, 0, 0, dirEast)
			setPortSignal(in, "next", "signal-N")
			setPortSignal(in, "pulse", "signal-P")
			setPortSignal(in, "start", "signal-S")
			setPortSignal(in, "value", tc.value)

			e := newEmitter()
			in.comp.build(e, in)
			for i := range e.entities {
				e.owner[e.entities[i].EntityNumber] = in
			}
			e.constAlias = map[string]string{tc.incSig: tc.incAlias}
			addLabelPanels(e)

			var labels []string
			for _, ent := range e.entities {
				if ent.Name == displayPanelName {
					labels = append(labels, ent.Text)
				}
			}
			assert.Equal(t, []string{tc.want}, labels)
			assertNoScratchLabels(t, labels)
		})
	}
}

// TestInitialRegisterShowsTruthfulPhiLabel proves an initialised register names
// both the real entry constant and the computed next value.
func TestInitialRegisterShowsTruthfulPhiLabel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		initial int
		want    string
	}{
		{name: "one", initial: 1, want: "R = φ(1, N)"},
		{name: "zero", initial: 0, want: "R = φ(0, N)"},
		{name: "negative", initial: -3, want: "R = φ(-3, N)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := newInstance(newRegisterWithInitial(tc.initial))
			in.dir = dirEast
			in.pos = anchorPos(in, 0, 0, dirEast)
			setPortSignal(in, "next", "signal-N")
			setPortSignal(in, "pulse", "signal-P")
			setPortSignal(in, "start", "signal-S")
			setPortSignal(in, "value", "signal-R")

			e := newEmitter()
			in.comp.build(e, in)
			for i := range e.entities {
				e.owner[e.entities[i].EntityNumber] = in
			}
			addLabelPanels(e)

			var labels []string
			for _, ent := range e.entities {
				if ent.Name == displayPanelName {
					labels = append(labels, ent.Text)
				}
			}
			assert.Equal(t, []string{tc.want}, labels)
			assertNoScratchLabels(t, labels)
		})
	}
}

// TestStopGateShowsExitConditionLabel proves the stop gate gets exactly one
// summary panel naming the loop exit condition it gates the clock on, and the
// gate arith carries no label. The panel pass emits it after Allocate.
func TestStopGateShowsExitConditionLabel(t *testing.T) {
	t.Parallel()
	in := newInstance(&stopGate{})
	in.dir = dirEast
	in.pos = anchorPos(in, 0, 0, dirEast)
	setPortSignal(in, "pulse", "signal-P")
	setPortSignal(in, "start", "signal-S")
	setPortSignal(in, "index", "signal-I")
	setPortSignal(in, "bound", "signal-A")
	setPortSignal(in, "gated", "signal-G")

	e := newEmitter()
	in.comp.build(e, in)
	for i := range e.entities {
		e.owner[e.entities[i].EntityNumber] = in
	}
	addLabelPanels(e)

	var labels []string
	for _, ent := range e.entities {
		if ent.Name == displayPanelName {
			labels = append(labels, ent.Text)
		}
	}
	assert.Equal(t, []string{"run while I < A"}, labels)
	assertNoScratchLabels(t, labels)
}

// TestLabelTextSkipsDisplayPanel confirms a non-combinator carries no label, so
// the panel pass never tries to label a panel.
func TestLabelTextSkipsDisplayPanel(t *testing.T) {
	t.Parallel()
	_, _, ok := testLabeller.labelText(entity{Name: "display-panel", Text: "x"})
	assert.False(t, ok)
}

// setPortSignal gives label tests readable signals without running allocation.
func setPortSignal(in *instance, portName, signalName string) {
	in.port(portName).net = &netlistNet{
		signal: signalID{Type: "virtual", Name: signalName},
	}
}

// assertNoScratchLabels keeps private implementation signals out of the
// player-facing explanation.
func assertNoScratchLabels(t *testing.T, labels []string) {
	t.Helper()
	for _, text := range labels {
		assert.NotContains(t, text, "dot")
		assert.NotContains(t, text, "info")
		assert.NotContains(t, text, "check")
		assert.NotContains(t, text, "value")
		assert.NotContains(t, text, "scratch")
		assert.NotContains(t, text, "grey")
		assert.NotContains(t, text, "white")
		assert.NotContains(t, text, "black")
	}
}
