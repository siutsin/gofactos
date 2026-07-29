// This file defines the pulse gate that lets finite loops stop in hardware.
package factorio

// stopGate passes the clock pulse while a condition holds, so a loop
// terminates: once the index reaches the bound the gate closes, the registers
// stop latching, and they hold their final value via the self-feed that already
// holds them between pulses. It is the pulse-gating analogue of the clocked
// register, and the loop exit condition (`i < n`) made physical, which an
// ungated circuit would otherwise ignore. Proven in the tick simulator: the
// gated index counts 0..n and freezes, with no overshoot or oscillation. The
// Clock period absorbs the gate's feedback latency.
//
// Two combinators, a compact 1x2 unit like the clock and the register:
//
//	go:   if start and index < bound -> go = 1 (private red net)
//	gate: pulse * go       -> gated    (arith; go is 0 or 1, so gated = pulse
//	                                    while running and 0 once stopped)
//
// A recurrence-local warm-up adds private resettable count, ready, and run
// cells. They suppress the first raw pulse without adding a public net.
//
// A 0/1 go flag, not the 1/-1 branch-merge condition: gated = pulse * go must
// zero the pulse when stopped, and a -1 would invert the pulse rather than kill
// it (the register's hold checks pulse == 0 and its write is next * pulse). So
// the gate's own decider emits a fixed 1, not the merge's 1/-1. The summary
// label names the exit condition; the panel pass fills it after signals are
// allocated.
type stopGate struct {
	warmupTicks int
}

const stopGateWarmGoColumn = 3

// newStopGateWithWarmup creates a recurrence gate that delays its first pulse.
// A private counter suppresses raw pulses during the requested warm-up.
func newStopGateWithWarmup(warmupTicks int) *stopGate {
	return &stopGate{warmupTicks: warmupTicks}
}

// kind identifies stop gates for clock wiring and component-specific labels.
func (g *stopGate) kind() string { return "stopGate" }

// directClock declares that this gate receives the shared clock directly.
func (g *stopGate) directClock() {}

// clockStart declares that START controls this gate and its warm-up state.
func (g *stopGate) clockStart() {}

// ports declares the loop condition and gated pulse exposed to the netlist.
func (g *stopGate) ports() []portSpec {
	return []portSpec{
		{name: "pulse", kind: portIn, colour: green},
		{name: "start", kind: portIn, colour: green},
		{name: "index", kind: portIn, colour: green},
		{name: "bound", kind: portIn, colour: green},
		{name: "gated", kind: portOut, colour: green},
	}
}

// footprint reserves the gate, optional warm-up cells, and summary label.
func (g *stopGate) footprint(_ int) footprint {
	width := 2
	if g.warmupTicks > 0 {
		width = 6
	}
	return footprint{width: width, height: 3}
}

// build emits the run condition that prevents updates after loop termination.
func (g *stopGate) build(e *emitter, self *instance) {
	ppulse, pstart := self.port("pulse"), self.port("start")
	pindex := self.port("index")
	pbound, pgated := self.port("bound"), self.port("gated")
	pulseSig, startSig := portSignal(ppulse), portSignal(pstart)
	indexSig := portSignal(pindex)
	boundSig, gatedSig := portSignal(pbound), portSignal(pgated)
	goSig := privateData // the 0/1 run flag, on the private red net
	readySig := privateInc
	countSig := privateTmp
	one := 1

	// Each combinator is a 1x2 unit centred on a one-tile column.
	at := func(dx int) position {
		return position{X: self.pos.X + float64(dx) - 0.5, Y: self.pos.Y + 0.5}
	}

	goColumn := 0
	passColumn := 1
	var warmOne, warmHold, ready handle
	if g.warmupTicks > 0 {
		goColumn = stopGateWarmGoColumn
		passColumn = 5
		warmOne = e.add(entity{
			Name:      arithCombinatorName,
			Position:  at(0),
			Direction: clockUnitDir,
			ControlBehavior: &controlBehavior{
				ArithmeticConditions: &arithmeticConditions{
					FirstSignal:    &goSig,
					Operation:      "*",
					SecondConstant: &one,
					OutputSignal:   &countSig,
				},
			},
		})
		warmHold = e.add(entity{
			Name:      arithCombinatorName,
			Position:  at(1),
			Direction: clockUnitDir,
			ControlBehavior: &controlBehavior{
				ArithmeticConditions: &arithmeticConditions{
					FirstSignal:  &countSig,
					Operation:    "*",
					SecondSignal: &goSig,
					OutputSignal: &countSig,
				},
			},
		})
		ready = e.add(entity{
			Name:      deciderCombinatorName,
			Position:  at(2),
			Direction: clockUnitDir,
			ControlBehavior: &controlBehavior{
				DeciderConditions: &deciderConditions{
					Conditions: []deciderCondition{
						{
							FirstSignal: &countSig,
							Comparator:  "≥",
							Constant:    &g.warmupTicks,
						},
						{
							FirstSignal: &goSig,
							Comparator:  "=",
							Constant:    &one,
							CompareType: "and",
						},
						{
							FirstSignal: &readySig,
							Comparator:  "=",
							Constant:    &one,
						},
						{
							FirstSignal: &goSig,
							Comparator:  "=",
							Constant:    &one,
							CompareType: "and",
						},
					},
					Outputs: []deciderOutput{{
						Signal:             &readySig,
						CopyCountFromInput: false,
					}},
				},
			},
		})
	}

	// go: emit a fixed 1 while started and index < bound, on the private net.
	goCell := e.add(entity{
		Name:      deciderCombinatorName,
		Position:  at(goColumn),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{DeciderConditions: &deciderConditions{
			Conditions: []deciderCondition{
				{
					FirstSignal:  &indexSig,
					Comparator:   "<",
					SecondSignal: &boundSig,
				},
				{
					FirstSignal: &startSig,
					Comparator:  "=",
					Constant:    &one,
					CompareType: "and",
				},
			},
			Outputs: []deciderOutput{{Signal: &goSig, CopyCountFromInput: false}},
		}},
	})
	runCell := goCell
	if g.warmupTicks > 0 {
		runCell = e.add(entity{
			Name:      arithCombinatorName,
			Position:  at(4),
			Direction: clockUnitDir,
			ControlBehavior: &controlBehavior{
				ArithmeticConditions: &arithmeticConditions{
					FirstSignal:  &goSig,
					Operation:    "*",
					SecondSignal: &readySig,
					OutputSignal: &goSig,
				},
			},
		})
	}
	// gate: pulse * go -> gated. Non-zero only while go is 1, so the gated pulse
	// stops the instant the index reaches the bound.
	pass := e.add(entity{
		Name:      arithCombinatorName,
		Position:  at(passColumn),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{ArithmeticConditions: &arithmeticConditions{
			FirstSignal: &pulseSig, Operation: "*", SecondSignal: &goSig, OutputSignal: &gatedSig,
		}},
	})

	if g.warmupTicks > 0 {
		// The count net sums one while go is live with the prior enabled count.
		// When START turns off, go drops and the feedback clears in one tick.
		e.link(warmOne, connectorRedOut, warmHold, connectorRedIn)
		e.link(warmHold, connectorRedOut, warmHold, connectorRedIn)
		e.link(ready, connectorRedIn, warmHold, connectorRedIn)
		// A separate private green net carries go and the ready latch. Both count
		// and readiness therefore reset when go drops.
		e.link(goCell, connectorGreenOut, warmOne, connectorGreenIn)
		e.link(goCell, connectorGreenOut, warmHold, connectorGreenIn)
		e.link(goCell, connectorGreenOut, ready, connectorGreenIn)
		e.link(ready, connectorGreenOut, ready, connectorGreenIn)
		e.link(goCell, connectorRedOut, runCell, connectorRedIn)
		e.link(ready, connectorRedOut, runCell, connectorRedIn)
	}
	// Private red net: go, or warmed run, into the gate arith.
	e.link(runCell, connectorRedOut, pass, connectorRedIn)
	// index and bound (green) feed the condition; pulse (green) feeds the gate;
	// the gated pulse leaves on green.
	e.bind(pindex, goCell, connectorGreenIn)
	e.bind(pbound, goCell, connectorGreenIn)
	e.bind(pstart, goCell, connectorGreenIn)
	e.bind(ppulse, pass, connectorGreenIn)
	e.bind(pgated, pass, connectorGreenOut)
}

// combinatorLabel presents the stop gate as one source-level run condition.
// It names the condition that gates the clock, for example "run while i1 < A".
func (g *stopGate) combinatorLabel(ent entity, self *instance, l *labeller) (labelPanel, bool) {
	cb := ent.ControlBehavior
	if cb == nil || cb.DeciderConditions == nil || len(cb.DeciderConditions.Conditions) == 0 {
		return labelPanel{}, false
	}
	cond := cb.DeciderConditions.Conditions[0]
	index := self.port("index").net
	bound := self.port("bound").net
	if cond.FirstSignal == nil || cond.SecondSignal == nil ||
		index == nil || bound == nil ||
		cond.FirstSignal.Name != index.signal.Name ||
		cond.SecondSignal.Name != bound.signal.Name {
		return labelPanel{}, false
	}
	labelColumn := 0
	if g.warmupTicks > 0 {
		labelColumn = stopGateWarmGoColumn
	}
	return labelPanel{
		text: "run while " + l.conditionLabelForPorts(
			cond,
			self,
			"index",
			"bound",
			false,
		),
		pos: position{
			X: self.pos.X + float64(labelColumn),
			Y: self.pos.Y - 1,
		},
	}, true
}
