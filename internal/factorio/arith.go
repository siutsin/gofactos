// This file defines the leaf module used to lower binary arithmetic.
package factorio

// arith is a leaf module: an arithmetic combinator computing a op b, reading
// both operands from its green input and writing the result to green output.
type arith struct{ op string }

// newArith creates the module selected for a supported binary operation.
func newArith(op string) *arith { return &arith{op: op} }

// kind identifies arithmetic modules in diagnostics and placement metadata.
func (a *arith) kind() string { return "arith" }

// ports declares the operands and result that connect arithmetic to the
// netlist.
func (a *arith) ports() []portSpec {
	return []portSpec{
		{name: "a", kind: portIn, colour: green},
		{name: "b", kind: portIn, colour: green},
		{name: "out", kind: portOut, colour: green},
	}
}

// footprint reserves a 2x2 unit for the combinator and its teaching label.
func (a *arith) footprint(dir int) footprint {
	return expandFootprint(dir, footprintPart{0, 1, false})
}

// build emits the combinator that makes a selected arithmetic operation real.
func (a *arith) build(e *emitter, self *instance) {
	pa, pb, pout := self.port("a"), self.port("b"), self.port("out")
	aSig, bSig, outSig := portSignal(pa), portSignal(pb), portSignal(pout)
	h := e.add(entity{
		Name:      arithCombinatorName,
		Position:  self.pos,
		Direction: self.dir,
		ControlBehavior: &controlBehavior{ArithmeticConditions: &arithmeticConditions{
			FirstSignal:  &aSig,
			Operation:    a.op,
			SecondSignal: &bSig,
			OutputSignal: &outSig,
		}},
	})
	e.bind(pa, h, connectorFor(false, pa.spec.kind, pa.spec.colour))
	e.bind(pb, h, connectorFor(false, pb.spec.kind, pb.spec.colour))
	e.bind(pout, h, connectorFor(false, pout.spec.kind, pout.spec.colour))
}
