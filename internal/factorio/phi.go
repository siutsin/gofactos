// This file defines the physical merge used for SSA phi nodes.
package factorio

import "fmt"

const (
	phiGateColumn    = 3
	phiOutputColumn  = 6
	phiBranchRowStep = 3
)

// phi is the merge: it selects one of two green branch inputs by the 1/-1
// condition on cond and drives the green result. It is slide 24 made physical,
// the wire literally being the phi node, because both gates copy onto one
// shared red sum net and exactly one is ever live.
//
//	cond == 1  -> a   cond == -1 -> b
//
// Topology (per branch X in {a, b}):
//
//	remapX: X * 1 -> S        (arithmetic, X on green, S on its own red net)
//	gateX:  if cond == k copy S -> S on the shared red sum net (decider)
//	output: S * 1 -> result   (arithmetic, sum on red, result on green)
//
// The gates read cond on green, the inter-module signal kept at its boundary,
// so each gate's red input carries only its own branch S. Recolouring cond to
// red and fanning it to both gates would union the two branch nets through that
// shared red cond, summing both branches; reading cond on green keeps the
// branch nets disjoint. Each gate's input net and the shared sum net are
// distinct, so copy-count copies the remapped branch value, not the gate's own
// output. With one gate live, the sum settles to the winner and never doubles.
type phi struct{}

// kind identifies phi modules in diagnostics and placement metadata.
func (p *phi) kind() string { return "phi" }

// ports declares the condition, branch values, and merged result.
func (p *phi) ports() []portSpec {
	return []portSpec{
		{name: "cond", kind: portIn, colour: green},
		{name: "a", kind: portIn, colour: green},
		{name: "b", kind: portIn, colour: green},
		{name: "out", kind: portOut, colour: green},
	}
}

// footprint reserves the two branch gates and their shared merge output.
func (p *phi) footprint(dir int) footprint {
	return expandFootprint(dir,
		footprintPart{0, 1, false},                    // remapA
		footprintPart{0, 1 + phiBranchRowStep, false}, // remapB
		footprintPart{phiGateColumn, 1, false},        // gateA
		footprintPart{
			phiGateColumn,
			1 + phiBranchRowStep,
			false,
		}, // gateB
		footprintPart{phiOutputColumn, 1, false}, // output
	)
}

// build emits mutually exclusive gates so the selected branch remains intact.
func (p *phi) build(e *emitter, self *instance) {
	pcond, pa, pb, pout := self.port("cond"), self.port("a"), self.port("b"), self.port("out")
	condSig := portSignal(pcond)
	aSig, bSig := portSignal(pa), portSignal(pb)
	outSig := portSignal(pout)
	dataSig := privateData
	one, minusOne := 1, -1

	remap := func(in signalID, dy float64) handle {
		return e.add(entity{
			Name:      arithCombinatorName,
			Position:  position{X: self.pos.X, Y: self.pos.Y + dy},
			Direction: self.dir,
			ControlBehavior: &controlBehavior{ArithmeticConditions: &arithmeticConditions{
				FirstSignal: &in, Operation: "*", SecondConstant: &one, OutputSignal: &dataSig,
			}},
		})
	}
	gate := func(want *int, dy float64) handle {
		return e.add(entity{
			Name: deciderCombinatorName,
			Position: position{
				X: self.pos.X + phiGateColumn,
				Y: self.pos.Y + dy,
			},
			Direction: self.dir,
			ControlBehavior: &controlBehavior{DeciderConditions: &deciderConditions{
				Conditions: []deciderCondition{{FirstSignal: &condSig, Comparator: "=", Constant: want}},
				Outputs:    []deciderOutput{{Signal: &dataSig, CopyCountFromInput: true}},
			}},
		})
	}

	remapA, remapB := remap(aSig, 0), remap(bSig, phiBranchRowStep)
	gateA, gateB := gate(&one, 0), gate(&minusOne, phiBranchRowStep)
	output := e.add(entity{
		Name: arithCombinatorName,
		Position: position{
			X: self.pos.X + phiOutputColumn,
			Y: self.pos.Y,
		},
		Direction: self.dir,
		ControlBehavior: &controlBehavior{ArithmeticConditions: &arithmeticConditions{
			FirstSignal: &dataSig, Operation: "*", SecondConstant: &one, OutputSignal: &outSig,
		}},
	})

	// Each branch's remapped S reaches only its own gate.
	e.link(remapA, connectorRedOut, gateA, connectorRedIn)
	e.link(remapB, connectorRedOut, gateB, connectorRedIn)
	// cond fans to both gates on green, keeping the branch nets disjoint.
	e.link(gateA, connectorGreenIn, gateB, connectorGreenIn)
	// Both gates copy onto the one shared red sum net the output reads.
	e.link(gateA, connectorRedOut, output, connectorRedIn)
	e.link(gateB, connectorRedOut, output, connectorRedIn)

	e.bind(pcond, gateA, connectorGreenIn)
	e.bind(pa, remapA, connectorGreenIn)
	e.bind(pb, remapB, connectorGreenIn)
	e.bind(pout, output, connectorGreenOut)
}

// combinatorLabel explains the merge while hiding its private scratch signal.
// Branch normalisation and gating retain their source-level value names;
// combinators with no teaching step get no panel.
func (p *phi) combinatorLabel(ent entity, self *instance, l *labeller) (labelPanel, bool) {
	cb := ent.ControlBehavior
	if cb == nil {
		return labelPanel{}, false
	}
	ac := cb.ArithmeticConditions
	if ac != nil && ac.OutputSignal != nil &&
		ac.OutputSignal.Name == privateData.Name && ac.FirstSignal == nil {
		return labelPanel{}, false
	}
	if panel, ok := phiArithmeticLabel(
		ent,
		ac,
		self,
		l,
	); ok {
		return panel, true
	}
	return phiDeciderLabel(ent, cb.DeciderConditions, self, l)
}

// phiArithmeticLabel labels the branch normalisers and final merge output.
func phiArithmeticLabel(
	ent entity,
	ac *arithmeticConditions,
	self *instance,
	l *labeller,
) (labelPanel, bool) {
	if ac == nil || ac.OutputSignal == nil {
		return labelPanel{}, false
	}
	if ac.OutputSignal.Name == privateData.Name {
		if ac.FirstSignal == nil {
			return labelPanel{}, false
		}
		portName := "a"
		if ent.Position.Y == self.pos.Y+phiBranchRowStep {
			portName = "b"
		}
		return phiLabelPanel(
			ent,
			"normalise "+l.portSignalLabel(
				self,
				portName,
				ac.FirstSignal,
			),
		), true
	}
	if ac.FirstSignal != nil && ac.FirstSignal.Name == privateData.Name {
		return phiLabelPanel(
			ent,
			fmt.Sprintf(
				"%s = merge",
				l.portSignalLabel(self, "out", ac.OutputSignal),
			),
		), true
	}
	return labelPanel{}, false
}

// phiDeciderLabel names which source branch each conditional gate admits.
func phiDeciderLabel(
	ent entity,
	dc *deciderConditions,
	self *instance,
	l *labeller,
) (labelPanel, bool) {
	if dc == nil || len(dc.Conditions) == 0 {
		return labelPanel{}, false
	}
	c := dc.Conditions[0]
	if c.Constant == nil {
		return labelPanel{}, false
	}
	cond := l.phiPortSignalLabel(self, "cond")
	switch *c.Constant {
	case 1:
		return phiLabelPanel(
			ent,
			fmt.Sprintf(
				"if %s { merge = %s }",
				cond,
				l.phiPortSignalLabel(self, "a"),
			),
		), true
	case -1:
		return phiLabelPanel(
			ent,
			fmt.Sprintf(
				"if !%s { merge = %s }",
				cond,
				l.phiPortSignalLabel(self, "b"),
			),
		), true
	}
	return labelPanel{}, false
}

// phiLabelPanel keeps all merge teaching panels aligned consistently.
func phiLabelPanel(ent entity, text string) labelPanel {
	return labelPanel{text: text, pos: labelPanelPosition(ent)}
}
