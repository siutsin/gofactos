// This file protects the pulse timing contract of the shared clock divider.
package factorio

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClockDivPeriod proves the clock pulses once every `period` ticks: a
// started counter mod period fires a clean one-tick pulse, the per-pulse
// increment every loop steps on. The first pulse must be a single tick too: a
// two-tick startup pulse made a terminating loop over-count (forI(1) settled
// on 4 instead of 2), which is why the clock fires on bounded state 2, not 0.
func TestClockDivPeriod(t *testing.T) {
	t.Parallel()
	const period = 4
	cd := newInstance(newClockDivWithSummary(period, ""))
	insts := []*instance{cd}
	nets := []*netlistNet{
		connect(cd.port("pulse")), // pulse readout
		connect(cd.port("start")), // START readout
	}
	out := nets[0]
	require.NoError(t, allocateSignals(nets))
	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))

	outBind := e.bound[cd.port("pulse")]
	off := newSim(e.wires)
	for range 2 * period {
		off.advance(e.entities)
		require.Zero(t, off.value(
			int(outBind.h),
			outBind.conn,
			out.signal.Name,
		))
	}
	setClockStarted(t, e.entities, true)
	s := newSim(e.wires)
	var pulses []int
	for tick := range 6 * period {
		s.advance(e.entities)
		if s.value(int(outBind.h), outBind.conn, out.signal.Name) == 1 {
			pulses = append(pulses, tick)
		}
	}

	require.GreaterOrEqual(
		t,
		len(pulses),
		3,
		"clock must pulse repeatedly: %v",
		pulses,
	)
	require.Equal(t, 2, pulses[0])
	// Every pulse, including the first, is a single tick exactly `period` apart.
	// A consecutive (one tick apart) pulse would be the startup double-pulse.
	for i := 1; i < len(pulses); i++ {
		require.Equal(
			t,
			period,
			pulses[i]-pulses[i-1],
			"pulses must be single ticks, period apart: %v",
			pulses,
		)
	}
}

// TestClockDivBoundsSeededState proves a near-overflow feedback value is
// reduced into the periodic state before it can disrupt later pulses.
func TestClockDivBoundsSeededState(t *testing.T) {
	t.Parallel()
	const period = 4
	cd := newInstance(newClockDivWithSummary(period, ""))
	nets := []*netlistNet{
		connect(cd.port("pulse")),
		connect(cd.port("start")),
	}
	require.NoError(t, allocateSignals(nets))
	place([]*instance{cd}, nets)
	e := emitNetlist([]*instance{cd}, netEdges(nets))
	setClockStarted(t, e.entities, true)

	modulo := 0
	for _, ent := range e.entities {
		if ent.Name != arithCombinatorName || ent.ControlBehavior == nil ||
			ent.ControlBehavior.ArithmeticConditions == nil {
			continue
		}
		if ent.ControlBehavior.ArithmeticConditions.Operation == "%" {
			modulo = ent.EntityNumber
			break
		}
	}
	require.Positive(t, modulo)
	out := e.bound[cd.port("pulse")]
	s := newSim(e.wires)
	s.seeded = true
	s.state = s.constantState(e.entities)
	stateNet := s.network(modulo, connectorRedIn)
	seed := math.MaxInt32 - ((math.MaxInt32 - 1) % period)
	if s.state[stateNet] == nil {
		s.state[stateNet] = map[string]int{}
	}
	s.state[stateNet][privateData.Name] = seed

	s.advance(e.entities)
	require.Equal(t, 2, s.state[stateNet][privateData.Name])
	var pulses []int
	for tick := 1; tick <= 3*period; tick++ {
		s.advance(e.entities)
		if s.value(int(out.h), out.conn, nets[0].signal.Name) == 1 {
			pulses = append(pulses, tick)
		}
	}
	require.Equal(t, []int{1, 1 + period, 1 + 2*period}, pulses)
}

// TestClockDivUsesRequestedSummary verifies that a custom clock name replaces
// only the summary panel text.
func TestClockDivUsesRequestedSummary(t *testing.T) {
	t.Parallel()
	const summary = "recursion clock (1 Hz)"
	cd := newInstance(newClockDivWithSummary(clockPeriod, summary))
	nets := []*netlistNet{
		connect(cd.port("pulse")),
		connect(cd.port("start")),
	}
	require.NoError(t, allocateSignals(nets))
	place([]*instance{cd}, nets)
	e := emitNetlist([]*instance{cd}, netEdges(nets))

	var summaries []string
	for _, ent := range e.entities {
		if ent.Name == displayPanelName {
			summaries = append(summaries, ent.Text)
		}
	}
	require.Equal(t, []string{summary, clockStartLabel}, summaries)
}

// setClockStarted toggles the sole default-off control in emitted test state.
func setClockStarted(t *testing.T, entities []entity, started bool) {
	t.Helper()
	control := clockStartControl(t, entities)
	require.NotNil(t, control.ControlBehavior)
	require.NotNil(t, control.ControlBehavior.IsOn)
	*control.ControlBehavior.IsOn = started
}

// clockStartControl finds the control owned by the shared clock component.
func clockStartControl(t *testing.T, entities []entity) *entity {
	t.Helper()
	var control *entity
	for index := range entities {
		candidate := &entities[index]
		if candidate.Name != constCombinatorName ||
			candidate.ControlBehavior == nil ||
			candidate.ControlBehavior.IsOn == nil {
			continue
		}
		require.Nil(t, control)
		control = candidate
	}
	require.NotNil(t, control)
	return control
}
