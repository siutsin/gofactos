// This file defines comparison modules that preserve false as a wire value.
package factorio

import "fmt"

// compare is a composite module that emits the 1/-1 condition the merge reads:
// present 1 when a cmp const holds, present -1 when it does not, never absent.
// A single decider cannot emit a fixed -1 with the fields gofactos uses, so
// compare spends three combinators:
//
//	D1: a cmp const  -> cond = 1   (decider, output value 1)
//	D2: a !cmp const -> tmp  = 1   (decider, output value 1, internal red)
//	A1: tmp * -1     -> cond = -1  (arithmetic, internal red in)
//
// D1 and A1 are mutually exclusive physical emitters behind the module's single
// cond output port. They share its green network, while compare remains the
// public net's sole logical driver at the IR boundary.
// The 1/-1 form means the active merge gate tests cond == 1 and the inactive
// cond == -1, so neither fires while cond is absent before compare settles and
// a boolean false (-1) is never confused with absent.
type compare struct {
	op       string // Factorio comparator, e.g. "<"
	constant int    // right-hand side when comparing against a constant
	variable bool   // when true, compare against the b signal port, not constant
	boolean  bool   // when true, the constant is a 1/-1 boolean sentinel, not an int
}

// newCompare creates the constant comparison used by conditional SSA values.
func newCompare(op string, constant int) *compare {
	return &compare{op: op, constant: constant}
}

// newCompareVar creates a signal-to-signal comparison for variable operands.
// It reads the right operand from a green port instead of a baked constant,
// supporting comparisons such as `x < lo`.
func newCompareVar(op string) *compare {
	return &compare{op: op, variable: true}
}

// kind identifies comparison modules in diagnostics and placement metadata.
func (c *compare) kind() string { return "compare" }

// ports declares the operands and encoded condition exposed to the netlist.
func (c *compare) ports() []portSpec {
	ports := []portSpec{
		{name: "a", kind: portIn, colour: green},
		{name: "cond", kind: portOut, colour: green},
	}
	if c.variable {
		ports = append(ports, portSpec{name: "b", kind: portIn, colour: green})
	}
	return ports
}

// footprint reserves both branches needed to represent true and false.
func (c *compare) footprint(dir int) footprint {
	return expandFootprint(dir,
		footprintPart{0, 1, false},
		footprintPart{0, 4, false},
		footprintPart{3, 4, false},
	)
}

// build emits both comparison branches so false remains present as -1.
func (c *compare) build(e *emitter, self *instance) {
	pa, pcond := self.port("a"), self.port("cond")
	aSig, condSig := portSignal(pa), portSignal(pcond)
	tmpSig := privateTmp
	minusOne := -1

	// The right-hand side is either a baked constant or the b signal port.
	cond := func(op string) deciderCondition {
		dc := deciderCondition{FirstSignal: &aSig, Comparator: op}
		if c.variable {
			b := portSignal(self.port("b"))
			dc.SecondSignal = &b
		} else {
			k := c.constant
			dc.Constant = &k
		}
		return dc
	}

	// D1: a cmp rhs -> cond = 1, on the green cond net.
	d1 := e.add(entity{
		Name:      deciderCombinatorName,
		Position:  self.pos,
		Direction: self.dir,
		ControlBehavior: &controlBehavior{DeciderConditions: &deciderConditions{
			Conditions: []deciderCondition{cond(c.op)},
			Outputs:    []deciderOutput{{Signal: &condSig, CopyCountFromInput: false}},
		}},
	})

	// D2: a !cmp rhs -> tmp = 1, on an internal red net into A1.
	d2 := e.add(entity{
		Name:      deciderCombinatorName,
		Position:  position{X: self.pos.X, Y: self.pos.Y + 3},
		Direction: self.dir,
		ControlBehavior: &controlBehavior{DeciderConditions: &deciderConditions{
			Conditions: []deciderCondition{cond(negateComparator(c.op))},
			Outputs:    []deciderOutput{{Signal: &tmpSig, CopyCountFromInput: false}},
		}},
	})

	// A1: tmp * -1 -> cond = -1, joining the green cond net.
	a1 := e.add(entity{
		Name:      arithCombinatorName,
		Position:  position{X: self.pos.X + 3, Y: self.pos.Y + 3},
		Direction: self.dir,
		ControlBehavior: &controlBehavior{ArithmeticConditions: &arithmeticConditions{
			FirstSignal:    &tmpSig,
			Operation:      "*",
			SecondConstant: &minusOne,
			OutputSignal:   &condSig,
		}},
	})

	// Internal wiring: both deciders read a on green; tmp flows D2 -> A1 on red;
	// D1 and A1 share the green cond net.
	e.link(d1, connectorGreenIn, d2, connectorGreenIn)
	e.link(d2, connectorRedOut, a1, connectorRedIn)
	e.link(a1, connectorGreenOut, d1, connectorGreenOut)

	// Boundary ports: a (and b in variable mode) enter at D1's green input,
	// cond leaves at D1's green output.
	e.bind(pa, d1, connectorGreenIn)
	if c.variable {
		e.bind(self.port("b"), d1, connectorGreenIn)
	}
	e.bind(pcond, d1, connectorGreenOut)
}

// combinatorLabel presents the comparison as a yes/no test without scratch
// signals.
func (c *compare) combinatorLabel(
	ent entity,
	self *instance,
	l *labeller,
) (labelPanel, bool) {
	cb := ent.ControlBehavior
	if cb == nil {
		return labelPanel{}, false
	}
	if dc := cb.DeciderConditions; dc != nil && len(dc.Conditions) > 0 && len(dc.Outputs) > 0 {
		cond, out := dc.Conditions[0], dc.Outputs[0].Signal
		condition := l.conditionLabelForPorts(cond, self, "a", "b", c.boolean)
		if out != nil && out.Name == privateTmp.Name {
			return labelPanel{
				text: "if " + condition,
				pos:  labelPanelPosition(ent),
			}, true
		}
		return labelPanel{
			text: fmt.Sprintf(
				"%s = %s",
				l.portSignalLabel(self, "cond", out),
				condition,
			),
			pos: labelPanelPosition(ent),
		}, true
	}
	if ac := cb.ArithmeticConditions; ac != nil && ac.OutputSignal != nil &&
		ac.FirstSignal != nil && ac.FirstSignal.Name == privateTmp.Name {
		return labelPanel{
			text: fmt.Sprintf(
				"%s = false",
				l.portSignalLabel(self, "cond", ac.OutputSignal),
			),
			pos: labelPanelPosition(ent),
		}, true
	}
	return labelPanel{}, false
}
