// This file protects register initialisation, latching, and hold behaviour.
package factorio

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterLatchesAndHolds proves the register primitive end to end: with no
// pulse the value stays at zero, a single pulse latches `next`, and afterwards
// the value holds across many quiet ticks via the self-feed. The pulse is
// injected by hand each tick so the test can toggle it, mirroring the
// synchronous sim model (no combinator drives the pulse net).
func TestRegisterLatchesAndHolds(t *testing.T) {
	t.Parallel()
	reg := newInstance(&register{})
	nextSrc := newInstance(newConstSrc(7))
	startSrc := newInstance(newConstSrc(1))
	insts := []*instance{nextSrc, startSrc, reg}

	// The pulse net has no driver or reader edge; it exists only to carry a
	// signal, and the test injects the pulse value directly into the sim.
	pulseNet := &netlistNet{}
	reg.port("pulse").net = pulseNet
	nets := []*netlistNet{
		connect(nextSrc.port("out"), reg.port("next")),
		connect(startSrc.port("out"), reg.port("start")),
		pulseNet,
		connect(reg.port("value")),
	}
	require.NoError(t, allocateSignals(nets))
	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))

	var holdNum int
	for n, owner := range e.owner {
		if owner == reg && e.entities[n-1].Name == deciderCombinatorName {
			holdNum = n
		}
	}
	require.NotZero(t, holdNum)

	out := e.bound[reg.port("value")]
	valueSig := reg.port("value").net.signal.Name
	pulseSig := pulseNet.signal.Name

	s := newSim(e.wires)
	pulseNetID := s.network(holdNum, connectorGreenIn)
	read := func() int { return s.value(int(out.h), out.conn, valueSig) }
	tick := func(pulse int) {
		if pulse != 0 {
			if s.state[pulseNetID] == nil {
				s.state[pulseNetID] = map[string]int{}
			}
			s.state[pulseNetID][pulseSig] = pulse
		}
		s.advance(e.entities)
	}

	// No pulse: the cell never leaves zero.
	for range 5 {
		tick(0)
		assert.Equal(t, 0, read())
	}
	// One pulse latches next (7); the pipeline settles within a couple of ticks.
	tick(1)
	tick(0)
	tick(0)
	assert.Equal(t, 7, read())
	// Many quiet ticks: the value holds via the self-feed rather than decaying.
	for range 20 {
		tick(0)
		assert.Equal(t, 7, read())
	}
}

type registerTestRig struct {
	entities    []entity
	sim         *sim
	start       connBinding
	pulseNet    simNetwork
	pulseSignal string
	value       connBinding
	valueSignal string
}

// newRegisterTestRig builds the production cell around controllable sources so
// timing tests observe real emitted behaviour.
func newRegisterTestRig(
	t *testing.T,
	initial, next int,
) *registerTestRig {
	t.Helper()
	pulseSrc := newInstance(newConstSrc(0))
	startSrc := newInstance(newConstSrc(1))
	nextSrc := newInstance(newConstSrc(next))
	reg := newInstance(newRegisterWithInitial(initial))
	insts := []*instance{pulseSrc, startSrc, nextSrc, reg}
	nets := []*netlistNet{
		connect(pulseSrc.port("out"), reg.port("pulse")),
		connect(startSrc.port("out"), reg.port("start")),
		connect(nextSrc.port("out"), reg.port("next")),
		connect(reg.port("value")),
	}
	require.NoError(t, allocateSignals(nets))
	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))

	pulse := e.bound[pulseSrc.port("out")]
	start := e.bound[startSrc.port("out")]
	on := true
	e.entities[int(start.h)-1].ControlBehavior.IsOn = &on
	value := e.bound[reg.port("value")]
	s := newSim(e.wires)
	return &registerTestRig{
		entities:    e.entities,
		sim:         s,
		start:       start,
		pulseNet:    s.network(int(pulse.h), pulse.conn),
		pulseSignal: pulseSrc.port("out").net.signal.Name,
		value:       value,
		valueSignal: reg.port("value").net.signal.Name,
	}
}

// setStarted changes the test START source without replacing the circuit.
func (r *registerTestRig) setStarted(started bool) {
	control := &r.entities[int(r.start.h)-1]
	*control.ControlBehavior.IsOn = started
}

// tick advances one circuit update with an optional write pulse.
func (r *registerTestRig) tick(pulse int) {
	if pulse != 0 {
		if r.sim.state[r.pulseNet] == nil {
			r.sim.state[r.pulseNet] = map[string]int{}
		}
		r.sim.state[r.pulseNet][r.pulseSignal] = pulse
	}
	r.sim.advance(r.entities)
}

// settle lets initial feedback stabilise before an assertion reads the cell.
func (r *registerTestRig) settle() {
	for range 5 {
		r.tick(0)
	}
}

// read exposes the register's public value rather than its private feedback.
func (r *registerTestRig) read() int {
	return r.sim.value(
		int(r.value.h),
		r.value.conn,
		r.valueSignal,
	)
}

// TestRegisterStartsAtConstant proves the cell exposes its configured constant
// before any update pulse, including zero and negative values.
func TestRegisterStartsAtConstant(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		initial int
	}{
		{name: "one", initial: 1},
		{name: "zero", initial: 0},
		{name: "negative", initial: -3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRegisterTestRig(t, tc.initial, 99)
			rig.settle()
			assert.Equal(t, tc.initial, rig.read())
		})
	}
}

// TestRegisterMinInt32BiasWraps proves the stored-delta bias remains a valid
// Factorio int32 constant at the one value whose mathematical inverse is +2^31.
func TestRegisterMinInt32BiasWraps(t *testing.T) {
	t.Parallel()
	reg := newInstance(newRegisterWithInitial(math.MinInt32))
	next := newInstance(newConstSrc(0))
	pulse := newInstance(newConstSrc(0))
	start := newInstance(newConstSrc(1))
	insts := []*instance{next, pulse, start, reg}
	nets := []*netlistNet{
		connect(next.port("out"), reg.port("next")),
		connect(pulse.port("out"), reg.port("pulse")),
		connect(start.port("out"), reg.port("start")),
		connect(reg.port("value")),
	}
	require.NoError(t, allocateSignals(nets))
	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))
	counts := initialBiasCounts(e.entities)
	require.ElementsMatch(t, []int{math.MinInt32, math.MinInt32}, counts)
}

// initialBiasCounts finds both bias constants so int32 wrapping is directly
// testable.
func initialBiasCounts(entities []entity) []int {
	var counts []int
	for _, ent := range entities {
		for _, section := range initialBiasSections(ent) {
			for _, filter := range section.Filters {
				counts = append(counts, filter.Count)
			}
		}
	}
	return counts
}

// initialBiasSections excludes unrelated constants from bias assertions.
func initialBiasSections(ent entity) []logisticSection {
	if ent.Name != constCombinatorName ||
		ent.ControlBehavior == nil ||
		ent.ControlBehavior.Sections == nil {
		return nil
	}
	var sections []logisticSection
	for _, section := range ent.ControlBehavior.Sections.Sections {
		if hasFilterSignal(section.Filters, privateTmp.Name) {
			sections = append(sections, section)
		}
	}
	return sections
}

// hasFilterSignal recognises the private bias signal without assuming order.
func hasFilterSignal(filters []constantFilter, name string) bool {
	for _, filter := range filters {
		if filter.Name == name {
			return true
		}
	}
	return false
}

// TestRegisterLatchesPulseValue proves one update pulse replaces the initial
// value with next, including Factorio's signal-absent representation of zero.
func TestRegisterLatchesPulseValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		initial int
		next    int
	}{
		{name: "seven", initial: 1, next: 7},
		{name: "zero", initial: -3, next: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRegisterTestRig(t, tc.initial, tc.next)
			rig.settle()
			rig.tick(1)
			rig.settle()
			assert.Equal(t, tc.next, rig.read())
		})
	}
}

// TestRegisterHoldsQuietly proves a latched value neither decays nor changes
// while the update pulse stays silent.
func TestRegisterHoldsQuietly(t *testing.T) {
	t.Parallel()
	rig := newRegisterTestRig(t, -3, 7)
	rig.settle()
	rig.tick(1)
	rig.settle()
	for range 20 {
		rig.tick(0)
		assert.Equal(t, 7, rig.read())
	}
}

// TestRegisterStopResetsState proves OFF clears a latched value back to its
// entry state and permits the same cell to run again.
func TestRegisterStopResetsState(t *testing.T) {
	t.Parallel()
	rig := newRegisterTestRig(t, -3, 7)
	rig.settle()
	rig.tick(1)
	rig.settle()
	require.Equal(t, 7, rig.read())

	rig.setStarted(false)
	rig.settle()
	require.Equal(t, -3, rig.read())

	rig.setStarted(true)
	rig.tick(1)
	rig.settle()
	require.Equal(t, 7, rig.read())
}

// TestRegistersAdvanceFibonacciState proves two cells latch in parallel from
// the same old state and keep their private memory nets isolated.
func TestRegistersAdvanceFibonacciState(t *testing.T) {
	t.Parallel()
	pulseSrc := newInstance(newConstSrc(0))
	startSrc := newInstance(newConstSrc(1))
	a := newInstance(newRegisterWithInitial(0))
	b := newInstance(newRegisterWithInitial(1))
	sum := newInstance(newArith("+"))
	insts := []*instance{pulseSrc, startSrc, a, b, sum}
	nets := []*netlistNet{
		connect(pulseSrc.port("out"), a.port("pulse"), b.port("pulse")),
		connect(startSrc.port("out"), a.port("start"), b.port("start")),
		connect(a.port("value"), sum.port("a")),
		connect(b.port("value"), a.port("next"), sum.port("b")),
		connect(sum.port("out"), b.port("next")),
	}
	require.NoError(t, allocateSignals(nets))
	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))
	require.NoError(t, verifyColours(e))

	pulse := e.bound[pulseSrc.port("out")]
	av := e.bound[a.port("value")]
	bv := e.bound[b.port("value")]
	s := newSim(e.wires)
	pulseNet := s.network(int(pulse.h), pulse.conn)
	pulseSignal := pulseSrc.port("out").net.signal.Name
	tick := func(p int) {
		if p != 0 {
			if s.state[pulseNet] == nil {
				s.state[pulseNet] = map[string]int{}
			}
			s.state[pulseNet][pulseSignal] = p
		}
		s.advance(e.entities)
	}
	quiet := func() {
		for range 5 {
			tick(0)
		}
	}
	read := func(binding connBinding, port *port) int {
		return s.value(
			int(binding.h),
			binding.conn,
			port.net.signal.Name,
		)
	}

	quiet()
	for i, want := range [][2]int{
		{0, 1},
		{1, 1},
		{1, 2},
		{2, 3},
		{3, 5},
	} {
		if i > 0 {
			tick(1)
			quiet()
		}
		assert.Equal(t, want[0], read(av, a.port("value")))
		assert.Equal(t, want[1], read(bv, b.port("value")))
	}
}
