// This file adds the shared clock required by clocked netlist components.
package factorio

// clockSummaryRequester is an optional component capability for an owner of an
// undriven pulse input to name the shared blueprint clock.
type clockSummaryRequester interface {
	clockSummary(period int) string
}

// directClockRequester marks components whose pulse input may receive the
// shared clock without an intervening gate.
type directClockRequester interface {
	directClock()
}

// clockStartRequester marks state that the shared START control enables and
// resets. Every implementation exposes an undriven `start` input.
type clockStartRequester interface {
	clockStart()
}

// clockPhase inserts the one blueprint clock for undriven pulse owners.
// Select leaves the clock's destination unwired because the pulse source exists
// only once the netlist is built, so Clock adds a single clockDiv and drives
// every still-undriven pulse port from it. It also fans one default-off START
// net to every clocked state owner. Loop registers receive pulses from their
// stop gate, so the clock drives the gate's pulse port. Clock runs after Select
// and before Allocate, so the module it adds flows through the remaining
// phases.
func clockPhase(sel *selected) {
	pulses := undrivenPulseInputs(sel.insts)
	if len(pulses) == 0 {
		return
	}
	starts := undrivenStartInputs(sel.insts)
	if len(starts) == 0 {
		panic("factorio: clocked netlist has no START consumers")
	}
	period := effectiveClockPeriod(sel.clockPeriod)
	cd := newInstance(newClockDivWithSummary(
		period,
		requestedClockSummary(pulses, period),
	))
	sel.insts = append(sel.insts, cd)
	sel.nets = append(sel.nets, connect(cd.port("pulse"), pulses...))
	sel.nets = append(sel.nets, connect(cd.port("start"), starts...))
}

// undrivenPulseInputs finds components that require the shared clock source.
func undrivenPulseInputs(insts []*instance) []*port {
	var pulses []*port
	for _, in := range insts {
		if _, ok := in.comp.(directClockRequester); !ok {
			continue
		}
		for _, p := range in.ports {
			if p.spec.name == "pulse" && p.spec.kind == portIn && p.net == nil {
				pulses = append(pulses, p)
			}
		}
	}
	return pulses
}

// undrivenStartInputs finds every state owner controlled by the shared clock.
func undrivenStartInputs(insts []*instance) []*port {
	var starts []*port
	for _, in := range insts {
		if _, ok := in.comp.(clockStartRequester); !ok {
			continue
		}
		start := in.port("start")
		if start.spec.kind == portIn && start.net == nil {
			starts = append(starts, start)
		}
	}
	return starts
}

// requestedClockSummary lets a clock consumer provide the most useful label.
func requestedClockSummary(pulses []*port, period int) string {
	for _, pulse := range pulses {
		requester, ok := pulse.inst.comp.(clockSummaryRequester)
		if !ok {
			continue
		}
		if summary := requester.clockSummary(period); summary != "" {
			return summary
		}
	}
	return ""
}
