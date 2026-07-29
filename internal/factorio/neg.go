// This file defines the leaf module used to lower unary integer negation.
package factorio

// neg is a leaf module: an arithmetic combinator computing in * -1, the unary
// negation a branch like abs's -n needs. It reads its operand on green and
// writes the result on green.
type neg struct{}

// kind identifies negation modules in diagnostics and placement metadata.
func (n *neg) kind() string { return "neg" }

// ports declares the operand and result that connect negation to the netlist.
func (n *neg) ports() []portSpec {
	return []portSpec{
		{name: "in", kind: portIn, colour: green},
		{name: "out", kind: portOut, colour: green},
	}
}

// footprint reserves a 2x2 unit for the combinator and its teaching label.
func (n *neg) footprint(dir int) footprint {
	return expandFootprint(dir, footprintPart{0, 1, false})
}

// build emits the arithmetic combinator that makes unary negation physical.
func (n *neg) build(e *emitter, self *instance) {
	pin, pout := self.port("in"), self.port("out")
	inSig, outSig := portSignal(pin), portSignal(pout)
	minusOne := -1
	h := e.add(entity{
		Name:      arithCombinatorName,
		Position:  self.pos,
		Direction: self.dir,
		ControlBehavior: &controlBehavior{ArithmeticConditions: &arithmeticConditions{
			FirstSignal:    &inSig,
			Operation:      "*",
			SecondConstant: &minusOne,
			OutputSignal:   &outSig,
		}},
	})
	e.bind(pin, h, connectorFor(false, pin.spec.kind, pin.spec.colour))
	e.bind(pout, h, connectorFor(false, pout.spec.kind, pout.spec.colour))
}
