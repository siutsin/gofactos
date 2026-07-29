// This file proves stateful owners receive one correctly configured clock.
package factorio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClockPhaseWiresLoop proves Clock drives both loop pulse and START inputs.
func TestClockPhaseWiresLoop(t *testing.T) {
	t.Parallel()
	gate := newInstance(&stopGate{})
	sel := &selected{insts: []*instance{gate}}
	clockPhase(sel)

	require.Len(t, sel.insts, 2, "a clockDiv should be added")
	require.Equal(t, "clockDiv", sel.insts[1].comp.kind())
	clock, ok := sel.insts[1].comp.(*clockDiv)
	require.True(t, ok)
	require.Equal(t, clockPeriod, clock.period)
	require.Equal(t, "loop clock (1 Hz)", clock.summary)
	require.NotNil(t, gate.port("pulse").net, "the gate pulse must be wired")
	require.Equal(t, sel.insts[1].port("pulse"), gate.port("pulse").net.driver)
	require.NotNil(t, gate.port("start").net, "the gate START must be wired")
	require.Equal(t, sel.insts[1].port("start"), gate.port("start").net.driver)
}

// TestClockPhaseUsesFastPeriod proves the selected period reaches the emitted
// clock and its player-visible rate label.
func TestClockPhaseUsesFastPeriod(t *testing.T) {
	t.Parallel()
	gate := newInstance(&stopGate{})
	sel := &selected{
		insts:       []*instance{gate},
		clockPeriod: fastClockPeriod,
	}
	clockPhase(sel)

	require.Len(t, sel.insts, 2)
	clock, ok := sel.insts[1].comp.(*clockDiv)
	require.True(t, ok)
	require.Equal(t, fastClockPeriod, clock.period)
	require.Equal(t, "loop clock (4 Hz)", clock.summary)
}

// TestClockPhaseDoesNotBypassStopGate proves an accidentally unwired register
// stays invalid rather than receiving the shared clock directly.
func TestClockPhaseDoesNotBypassStopGate(t *testing.T) {
	t.Parallel()
	gate := newInstance(&stopGate{})
	register := newInstance(&register{})
	sel := &selected{insts: []*instance{gate, register}}

	clockPhase(sel)

	require.Len(t, sel.insts, 3)
	require.NotNil(t, gate.port("pulse").net)
	assert.Nil(t, register.port("pulse").net)
	require.NotNil(t, register.port("start").net)
	require.Same(t, gate.port("start").net, register.port("start").net)
}

// TestClockPhaseNoLoopNoClock proves Clock adds nothing when there is no loop.
func TestClockPhaseNoLoopNoClock(t *testing.T) {
	t.Parallel()
	a := newInstance(newArith("+"))
	sel := &selected{insts: []*instance{a}}
	clockPhase(sel)
	require.Len(t, sel.insts, 1, "no loop means no clockDiv")
}
