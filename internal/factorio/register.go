// This file defines the clocked storage used for loop-carried SSA values.
package factorio

import "fmt"

// register is a clocked memory cell. It holds a value between update pulses and
// latches next on the pulse. An initialised register also exposes its constant
// entry value before the first pulse. It is a loop-carried phi node made
// physical.
//
// Its stored value sits on one private red net fed by two gated sources: the
// hold branch, live when pulse is 0, and the write branch, live on the pulse.
// At most one is non-zero each tick, so the net is their sum. The hold branch
// self-feeds the net it writes, and that self-feed turns the sum into memory.
//
// An initialised register adds one constant-combinator column. It emits the
// entry value and its inverse on two signals on the private net. The write cell
// therefore stores next-entry while the output cell adds entry back. Keeping
// the constant live is safe because the feedback cell copies only the stored
// delta.
//
// See docs/backend.md ("Composite Modules") for the design rationale.
//
// inc is the legacy scalar register's render hint. Its net alias names the
// per-iteration step. An uninitialised register with no inc is a bare cell and
// gets no label. inc never affects the circuit.
type register struct {
	inc     *netlistNet
	initial *int
}

// newRegisterWithInitial creates storage for a loop phi with a constant entry.
func newRegisterWithInitial(initial int) *register {
	return &register{initial: &initial}
}

// kind identifies registers for clock wiring and component-specific labels.
func (r *register) kind() string { return "register" }

// clockStart declares that START owns this register's retained state.
func (r *register) clockStart() {}

// ports declares the next value, pulse, START control, and held loop state.
func (r *register) ports() []portSpec {
	return []portSpec{
		{name: "next", kind: portIn, colour: green},
		{name: "pulse", kind: portIn, colour: green},
		{name: "start", kind: portIn, colour: green},
		{name: "value", kind: portOut, colour: green},
	}
}

// footprint reserves the state cells, optional entry source, and summary label.
func (r *register) footprint(_ int) footprint {
	if r.initial != nil {
		return footprint{width: 4, height: 3}
	}
	return footprint{width: 3, height: 3}
}

// build emits the hold and write paths that preserve state between pulses.
func (r *register) build(e *emitter, self *instance) {
	pnext, ppulse := self.port("next"), self.port("pulse")
	pstart, pvalue := self.port("start"), self.port("value")
	nextSig, pulseSig := portSignal(pnext), portSignal(ppulse)
	startSig, valueSig := portSignal(pstart), portSignal(pvalue)
	gv := privateData // the held value, on the private red net
	tmp := privateTmp
	zero, one := 0, 1

	offset := 0
	var initial handle
	if r.initial != nil {
		offset = 1
		entry := *r.initial
		//nolint:gosec // Factorio values intentionally wrap to signed 32 bits.
		inverse := int(int32(0) - int32(entry))
		initial = e.add(entity{
			Name: constCombinatorName,
			Position: position{
				X: self.pos.X - 0.5,
				Y: self.pos.Y,
			},
			ControlBehavior: &controlBehavior{
				Sections: &constantCombinatorSections{
					Sections: []logisticSection{{
						Index: 1,
						Filters: []constantFilter{
							{
								Index: 1, Type: tmp.Type,
								Name:    tmp.Name,
								Quality: "normal", Comparator: "=",
								Count: entry,
							},
							{
								Index: 2, Type: nextSig.Type,
								Name:    nextSig.Name,
								Quality: "normal", Comparator: "=",
								Count: inverse,
							},
						},
					}},
				},
			},
		})
	}

	// Each state combinator is a 1x2 unit centred on one tile column.
	at := func(dx int) position {
		return position{
			X: self.pos.X + float64(dx+offset) - 0.5,
			Y: self.pos.Y + 0.5,
		}
	}

	// hold: while started and pulse == 0, copy value -> value. Turning START off
	// removes the self-feed and clears retained state for a clean rerun.
	hold := e.add(entity{
		Name:      deciderCombinatorName,
		Position:  at(0),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{DeciderConditions: &deciderConditions{
			Conditions: []deciderCondition{
				{FirstSignal: &pulseSig, Comparator: "=", Constant: &zero},
				{
					FirstSignal: &startSig,
					Comparator:  "=",
					Constant:    &one,
					CompareType: "and",
				},
			},
			Outputs: []deciderOutput{{Signal: &gv, CopyCountFromInput: true}},
		}},
	})
	// wr: next * pulse -> value. Non-zero only on the pulse, when it loads next.
	wr := e.add(entity{
		Name:      arithCombinatorName,
		Position:  at(1),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{ArithmeticConditions: &arithmeticConditions{
			FirstSignal: &nextSig, Operation: "*", SecondSignal: &pulseSig, OutputSignal: &gv,
		}},
	})
	outputOperation := "*"
	outputConstant := &one
	var outputSecond *signalID
	if r.initial != nil {
		outputOperation = "+"
		outputConstant = nil
		outputSecond = &tmp
	}
	// obuf bridges the held value onto the public green output. An initialised
	// register adds its entry constant back to the stored delta.
	obuf := e.add(entity{
		Name:      arithCombinatorName,
		Position:  at(2),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{ArithmeticConditions: &arithmeticConditions{
			FirstSignal: &gv, Operation: outputOperation,
			SecondSignal: outputSecond, SecondConstant: outputConstant,
			OutputSignal: &valueSig,
		}},
	})

	// Private red net: value, self-fed by hold, written by wr, read by obuf.
	e.link(hold, connectorRedOut, hold, connectorRedIn)
	e.link(wr, connectorRedOut, hold, connectorRedIn)
	e.link(obuf, connectorRedIn, hold, connectorRedIn)
	if r.initial != nil {
		// The entry source shares the private input net. Its distinct signals
		// cannot enter the delta feedback.
		e.link(initial, connectorRedIn, hold, connectorRedIn)
		e.link(initial, connectorRedIn, wr, connectorRedIn)
	}
	// pulse, START, and next share the green input bus as distinct signals.
	e.bind(ppulse, hold, connectorGreenIn)
	e.bind(pstart, hold, connectorGreenIn)
	e.link(hold, connectorGreenIn, wr, connectorGreenIn)
	e.bind(pnext, wr, connectorGreenIn)
	// The held value leaves on green via the bridge.
	e.bind(pvalue, obuf, connectorGreenOut)
}

// combinatorLabel reduces a register to one source-level phi summary.
// Scalar registers retain their increment label; initialised registers name
// their actual entry constant and next value.
func (r *register) combinatorLabel(ent entity, self *instance, l *labeller) (labelPanel, bool) {
	if ent.ControlBehavior == nil ||
		ent.ControlBehavior.DeciderConditions == nil {
		return labelPanel{}, false
	}
	vp := self.port("value")
	if vp.net == nil {
		return labelPanel{}, false
	}
	name := l.portSignalLabel(self, "value", &vp.net.signal)
	if r.initial != nil {
		next := self.port("next")
		if next.net == nil {
			return labelPanel{}, false
		}
		return labelPanel{
			text: fmt.Sprintf(
				"%s = φ(%d, %s)",
				name,
				*r.initial,
				l.portSignalLabel(self, "next", &next.net.signal),
			),
			pos: position{
				X: self.pos.X + 0.5,
				Y: self.pos.Y - 1,
			},
		}, true
	}
	if r.inc == nil {
		return labelPanel{}, false
	}
	inc := l.signalLabel(&r.inc.signal)
	return labelPanel{
		text: fmt.Sprintf("%s = φ(0, %s + %s)", name, name, inc),
		pos:  position{X: self.pos.X + 0.5, Y: self.pos.Y - 1},
	}, true
}
