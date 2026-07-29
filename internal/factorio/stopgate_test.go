// This file proves loop gates stop exactly without corrupting held state.
package factorio

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStopGate proves the gate primitive: with the index below the bound it
// passes the clock pulse through, and once the index reaches or exceeds the
// bound it closes, emitting nothing, so the registers it feeds freeze. The pulse
// is injected by hand each tick, mirroring the synchronous sim model.
func TestStopGate(t *testing.T) {
	t.Parallel()
	for _, tc := range []stopGateCase{
		{name: "below bound passes the pulse", index: 2, wantGated: 1},
		{name: "at bound stops", index: 4, wantGated: 0},
		{name: "above bound stops", index: 5, wantGated: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testStopGateCase(t, tc)
		})
	}
}

// TestStopGateWarmupReadinessSurvivesCounterWrap proves readiness latches once
// established instead of clearing when the private tick count overflows.
func TestStopGateWarmupReadinessSurvivesCounterWrap(t *testing.T) {
	t.Parallel()
	gate := newInstance(newStopGateWithWarmup(2))
	pulse := newInstance(newConstSrc(0))
	start := newInstance(newConstSrc(1))
	index := newInstance(newConstSrc(0))
	bound := newInstance(newConstSrc(10))
	insts := []*instance{pulse, start, index, bound, gate}
	nets := []*netlistNet{
		connect(pulse.port("out"), gate.port("pulse")),
		connect(start.port("out"), gate.port("start")),
		connect(index.port("out"), gate.port("index")),
		connect(bound.port("out"), gate.port("bound")),
		connect(gate.port("gated")),
	}
	require.NoError(t, allocateSignals(nets))
	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))

	var counter, ready int
	for _, ent := range e.entities {
		if ent.ControlBehavior == nil {
			continue
		}
		if conditions := ent.ControlBehavior.ArithmeticConditions; conditions != nil && conditions.OutputSignal != nil &&
			conditions.OutputSignal.Name == privateTmp.Name {
			counter = ent.EntityNumber
		}
		if conditions := ent.ControlBehavior.DeciderConditions; conditions != nil && len(conditions.Outputs) == 1 &&
			conditions.Outputs[0].Signal != nil &&
			conditions.Outputs[0].Signal.Name == privateInc.Name {
			ready = ent.EntityNumber
		}
	}
	require.Positive(t, counter)
	require.Positive(t, ready)

	s := newSim(e.wires)
	for range 6 {
		s.advance(e.entities)
	}
	require.Equal(
		t,
		1,
		s.value(ready, connectorGreenIn, privateInc.Name),
	)
	countNet := s.network(counter, connectorRedIn)
	s.state[countNet][privateTmp.Name] = math.MaxInt32
	for range 3 {
		s.advance(e.entities)
		require.Equal(
			t,
			1,
			s.value(ready, connectorGreenIn, privateInc.Name),
		)
	}
}

// TestStopGateWarmupResetsWhenStopped proves OFF clears both the private count
// and ready latch before a second run begins warming from zero.
func TestStopGateWarmupResetsWhenStopped(t *testing.T) {
	t.Parallel()
	gate := newInstance(newStopGateWithWarmup(2))
	pulse := newInstance(newConstSrc(0))
	start := newInstance(newConstSrc(1))
	index := newInstance(newConstSrc(0))
	bound := newInstance(newConstSrc(10))
	insts := []*instance{pulse, start, index, bound, gate}
	nets := []*netlistNet{
		connect(pulse.port("out"), gate.port("pulse")),
		connect(start.port("out"), gate.port("start")),
		connect(index.port("out"), gate.port("index")),
		connect(bound.port("out"), gate.port("bound")),
		connect(gate.port("gated")),
	}
	require.NoError(t, allocateSignals(nets))
	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))
	startBinding := e.bound[start.port("out")]
	on := true
	e.entities[int(startBinding.h)-1].ControlBehavior.IsOn = &on

	ready := 0
	for _, ent := range e.entities {
		if ent.ControlBehavior == nil {
			continue
		}
		conditions := ent.ControlBehavior.DeciderConditions
		if conditions != nil && len(conditions.Outputs) == 1 &&
			conditions.Outputs[0].Signal != nil &&
			conditions.Outputs[0].Signal.Name == privateInc.Name {
			ready = ent.EntityNumber
		}
	}
	require.Positive(t, ready)
	s := newSim(e.wires)
	for range 8 {
		s.advance(e.entities)
	}
	require.Equal(t, 1, s.value(ready, connectorGreenIn, privateInc.Name))

	*e.entities[int(startBinding.h)-1].ControlBehavior.IsOn = false
	for range 4 {
		s.advance(e.entities)
	}
	require.Zero(t, s.value(ready, connectorRedIn, privateTmp.Name))
	require.Zero(t, s.value(ready, connectorGreenIn, privateInc.Name))

	*e.entities[int(startBinding.h)-1].ControlBehavior.IsOn = true
	for range 3 {
		s.advance(e.entities)
		require.Zero(t, s.value(ready, connectorGreenIn, privateInc.Name))
	}
	for range 8 {
		s.advance(e.entities)
	}
	require.Equal(t, 1, s.value(ready, connectorGreenIn, privateInc.Name))
}

type stopGateCase struct {
	name      string
	index     int
	wantGated int
}

// testStopGateCase builds the real gate once per boundary condition so the
// table tests behaviour rather than a duplicate formula.
func testStopGateCase(t *testing.T, tc stopGateCase) {
	t.Helper()
	gate := newInstance(&stopGate{})
	idxSrc := newInstance(newConstSrc(tc.index))
	boundSrc := newInstance(newConstSrc(4))
	startSrc := newInstance(newConstSrc(1))
	insts := []*instance{idxSrc, boundSrc, startSrc, gate}

	// The pulse net has no driver; the test injects the pulse directly.
	pulseNet := &netlistNet{}
	gate.port("pulse").net = pulseNet
	nets := []*netlistNet{
		connect(idxSrc.port("out"), gate.port("index")),
		connect(boundSrc.port("out"), gate.port("bound")),
		connect(startSrc.port("out"), gate.port("start")),
		pulseNet,
		connect(gate.port("gated")),
	}
	require.NoError(t, allocateSignals(nets))
	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))
	passNum := stopGatePassEntity(t, e, gate)
	out := e.bound[gate.port("gated")]
	s := &stopGateTestSim{
		entities:    e.entities,
		sim:         newSim(e.wires),
		out:         out,
		gatedSignal: gate.port("gated").net.signal.Name,
		pulseSignal: pulseNet.signal.Name,
	}
	s.pulseNet = s.sim.network(passNum, connectorGreenIn)

	// Let the inputs settle, then pulse the clock high and read the gate.
	for range 5 {
		s.tick(0)
	}
	assert.Equal(t, 0, s.gated(), "no clock pulse means no gated pulse")
	s.tick(1)
	assert.Equal(t, tc.wantGated, s.gated())
}

// stopGatePassEntity locates the pulse path for direct simulator injection.
func stopGatePassEntity(
	t *testing.T,
	e *emitter,
	gate *instance,
) int {
	t.Helper()
	for num, owner := range e.owner {
		if owner == gate &&
			e.entities[num-1].Name == arithCombinatorName {
			return num
		}
	}
	t.Fatal("stop gate pass entity not found")
	return 0
}

type stopGateTestSim struct {
	entities    []entity
	sim         *sim
	out         connBinding
	gatedSignal string
	pulseNet    simNetwork
	pulseSignal string
}

// tick advances the emitted gate with a controlled raw pulse.
func (s *stopGateTestSim) tick(pulse int) {
	if pulse != 0 {
		if s.sim.state[s.pulseNet] == nil {
			s.sim.state[s.pulseNet] = map[string]int{}
		}
		s.sim.state[s.pulseNet][s.pulseSignal] = pulse
	}
	s.sim.advance(s.entities)
}

// gated reads the public pulse that controls downstream register writes.
func (s *stopGateTestSim) gated() int {
	return s.sim.value(
		int(s.out.h),
		s.out.conn,
		s.gatedSignal,
	)
}
