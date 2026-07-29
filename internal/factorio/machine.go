// This file orchestrates bounded recursive programs as Factorio machines.
package factorio

import (
	"fmt"
)

const (
	recursiveMachineDepth           = 12
	recursiveMachineRows            = recursiveMachineDepth + 1
	recursiveMachineRowHeight       = 4
	recursiveMachineFrameY          = 4
	recursiveMachineFrameFieldWidth = 5
	recursiveMachineFrameGap        = 3
	recursiveMachineStartWidth      = 20
	// Controller rows contain a two-tile combinator and a routing lane.
	recursiveMachineControllerRowHeight = 3
	recursiveMachineControllerWrapWidth = 58
	recursiveMachineActionLabelWidth    = 10
	recursiveMachineFrameLampX          = 14
)

const (
	recursiveModeInit = iota
	recursiveModeRun
	recursiveModeEnter
	recursiveModePop
	recursiveModeResume
	recursiveModePresent
	recursiveModeDone
	recursiveModeOverflow
)

// recursiveMachine executes a planned self-recursive function over bounded,
// addressed stack frames. The plan, rather than the source function's
// name or shape, determines every operation emitted by the component.
type recursiveMachine struct {
	program *recursiveProgram
	frameX  int
}

// newRecursiveMachine binds a compiled program to its measured circuit layout.
func newRecursiveMachine(program *recursiveProgram) *recursiveMachine {
	return &recursiveMachine{
		program: program,
		frameX:  measureRecursiveMachineFrameX(program),
	}
}

// kind identifies this component to shared instance and diagnostic machinery.
func (m *recursiveMachine) kind() string { return "recursiveMachine" }

// directClock declares that the machine gates the shared pulse internally.
func (m *recursiveMachine) directClock() {}

// clockStart declares that the shared START control resets machine state.
func (m *recursiveMachine) clockStart() {}

// ports exposes arguments, shared clock control, pulse, and the result.
func (m *recursiveMachine) ports() []portSpec {
	ports := make([]portSpec, 0, len(m.program.params)+3)
	for i := range m.program.params {
		ports = append(ports, portSpec{
			name: fmt.Sprintf("arg%d", i), kind: portIn, colour: green,
		})
	}
	return append(ports,
		portSpec{name: "pulse", kind: portIn, colour: green},
		portSpec{name: "start", kind: portIn, colour: green},
		portSpec{name: "result", kind: portOut, colour: green},
	)
}

// clockSummary gives recursive execution its user-facing timing description.
func (m *recursiveMachine) clockSummary(period int) string {
	return clockSummary("recursion", period)
}

// footprint reserves the machine's whole routing area. Route may place green
// bus relays in the otherwise empty cells after the component has been built.
func (m *recursiveMachine) footprint(_ int) footprint {
	return footprint{
		width:  recursiveMachineFootprintWidth(m.program, m.frameX),
		height: recursiveMachineFootprintHeight(m.program),
	}
}

// recursiveMachineFootprintWidth reserves both controller and frame wiring.
func recursiveMachineFootprintWidth(
	program *recursiveProgram,
	frameX int,
) int {
	fields := 1 + program.slotCount
	frameWidth := frameX +
		fields*recursiveMachineFrameFieldWidth + 1
	lampWidth := frameX + recursiveMachineFrameLampX + 1
	return max(frameWidth, lampWidth)
}

// recursiveMachineFootprintHeight leaves room for variable instruction rows.
func recursiveMachineFootprintHeight(program *recursiveProgram) int {
	// Each instruction can expand to several mutually exclusive action rows.
	// This deliberately leaves routing space between the controller and records.
	return 160 + len(program.instructions)*10 +
		recursiveMachineRows*recursiveMachineRowHeight
}

// build emits the machine only after its public signals have stable identities.
func (m *recursiveMachine) build(e *emitter, self *instance) {
	public := recursiveMachinePublic{
		pulse:  portSignal(self.port("pulse")),
		start:  portSignal(self.port("start")),
		result: portSignal(self.port("result")),
		args:   make([]signalID, len(m.program.params)),
	}
	for i := range public.args {
		public.args[i] = portSignal(self.port(fmt.Sprintf("arg%d", i)))
	}
	b := newRecursiveMachineBuilder(e, self, m.program, public, m.frameX)
	b.build()
	b.wireAll()
}

// combinatorLabel suppresses generic labels because the machine labels itself.
func (m *recursiveMachine) combinatorLabel(
	_ entity,
	_ *instance,
	_ *labeller,
) (labelPanel, bool) {
	return labelPanel{}, false
}

type recursiveMachinePublic struct {
	args         []signalID
	pulse, start signalID
	result       signalID
}

type recursiveMachineSignals struct {
	pc, sp, mode, ret, result signalID
	slots                     []signalID
	arm, step, action         signalID
	data, write               signalID
	target                    signalID
}

// newRecursiveMachineSignals allocates private identities for machine state.
func newRecursiveMachineSignals(slotCount int) recursiveMachineSignals {
	signals := recursiveMachineSignals{
		pc:     rmVirtual("signal-P"),
		sp:     rmVirtual("signal-S"),
		mode:   rmVirtual("signal-M"),
		ret:    rmVirtual("signal-R"),
		result: rmVirtual("signal-O"),
		arm:    rmVirtual("signal-green"),
		step:   privateTmp,
		action: privateInc,
		data:   rmVirtual("signal-V"),
		write:  privateInc,
		target: rmVirtual("signal-T"),
		slots:  make([]signalID, slotCount),
	}
	for i := range signals.slots {
		signals.slots[i] = rmVirtual(fmt.Sprintf("signal-%d", i))
	}
	return signals
}

// rmVirtual keeps recursive-machine signal construction concise and uniform.
func rmVirtual(name string) signalID {
	return signalID{Type: "virtual", Name: name}
}

type recursiveMachineBuilder struct {
	e                             *emitter
	self                          *instance
	program                       *recursiveProgram
	public                        recursiveMachinePublic
	s                             recursiveMachineSignals
	layout                        recursiveMachineLayout
	nets                          []*recursiveMachineNet
	stateBus, commandBus, stepBus *recursiveMachineNet
	armRed                        *recursiveMachineNet
	pcData, spData                *recursiveMachineNet
	modeData, retData             *recursiveMachineNet
	resultData                    *recursiveMachineNet
	slotData                      []*recursiveMachineNet
	runningNet                    *recursiveMachineNet
	frameX                        int
}

// newRecursiveMachineBuilder collects emission, layout, and wiring state.
func newRecursiveMachineBuilder(
	e *emitter,
	self *instance,
	program *recursiveProgram,
	public recursiveMachinePublic,
	frameX int,
) *recursiveMachineBuilder {
	return &recursiveMachineBuilder{
		e: e, self: self, program: program, public: public,
		s:      newRecursiveMachineSignals(program.slotCount),
		layout: newRecursiveMachineLayout(),
		frameX: frameX,
	}
}

// build emits controller logic before the addressed stack frame bank.
func (b *recursiveMachineBuilder) build() {
	b.buildController()
	b.buildRows()
}

// buildController assembles state storage and every program action pipeline.
func (b *recursiveMachineBuilder) buildController() {
	b.stateBus = b.net(green)
	b.commandBus = b.net(green)
	b.stepBus = b.net(green)
	b.armRed = b.net(red)
	b.pcData = b.net(green)
	b.spData = b.net(green)
	b.modeData = b.net(green)
	b.retData = b.net(green)
	b.resultData = b.net(green)
	b.slotData = make([]*recursiveMachineNet, b.program.slotCount)
	for i := range b.slotData {
		b.slotData[i] = b.net(green)
	}

	b.buildStartInput()
	b.buildGlobals()
	b.buildPulse()
	b.buildInitialise()
	b.buildPop()
	b.buildInstructions()
	b.buildResult()
	b.buildPresent()
}

// measureRecursiveMachineFrameX keeps frames clear of the actual controller.
func measureRecursiveMachineFrameX(program *recursiveProgram) int {
	machine := &recursiveMachine{program: program}
	self := newInstance(machine)
	self.dir = dirEast
	self.pos = anchorPos(self, 0, 0, self.dir)
	public := recursiveMachinePublic{
		args:   make([]signalID, len(program.params)),
		pulse:  privateData,
		result: privateData,
	}
	for i := range public.args {
		public.args[i] = inputSignals[i]
	}
	b := newRecursiveMachineBuilder(
		newEmitter(), self, program, public, 0,
	)
	b.buildController()
	controllerWidth := max(recursiveMachineStartWidth, b.layout.width())
	return controllerWidth + recursiveMachineFrameGap
}

// buildStartInput isolates the shared public control as the machine-private arm
// signal used by action and state-cell reset guards.
func (b *recursiveMachineBuilder) buildStartInput() {
	one := 1
	control := b.arithConstant(
		recursiveMachinePoint{18, 2},
		b.public.start,
		"*",
		one,
		b.s.arm,
	)
	b.e.bind(b.self.port("start"), control, connectorGreenIn)
	b.armRed.add(control, connectorRedOut)
	b.stateBus.add(control, connectorGreenOut)
}

// buildGlobals creates state shared by every addressed stack frame.
func (b *recursiveMachineBuilder) buildGlobals() {
	b.layout.group(20)
	sp := b.globalCell(b.spData, b.s.sp)
	mode := b.globalCell(b.modeData, b.s.mode)
	ret := b.globalCell(b.retData, b.s.ret)
	result := b.globalCell(b.resultData, b.s.result)
	for _, cell := range []recursiveMachineCell{
		sp,
		mode,
		ret,
		result,
	} {
		b.stateBus.add(cell.output, connectorGreenOut)
	}
}

// globalCell stores one shared field behind the machine's command protocol.
func (b *recursiveMachineBuilder) globalCell(
	data *recursiveMachineNet,
	field signalID,
) recursiveMachineCell {
	p := b.layout.take()
	b.layout.take()
	b.layout.take()
	cell := b.cellAt(p, data, field)
	selector := b.decider(b.layout.take(), []deciderCondition{
		rmCondition(field, "!=", 0, ""),
	}, []deciderOutput{{
		Signal: &b.s.write, CopyCountFromInput: false,
	}})
	b.commandBus.add(selector, connectorGreenIn)
	b.connectWrite(cell, selector)
	return cell
}

// buildPulse gates public clock pulses until the completed machine is armed.
func (b *recursiveMachineBuilder) buildPulse() {
	b.layout.group(1)
	h := b.multiplySignals(
		b.layout.take(), b.public.pulse, b.s.arm, b.s.step,
	)
	b.e.bind(b.self.port("pulse"), h, connectorGreenIn)
	b.armRed.add(h, connectorRedIn)
	b.stepBus.add(h, connectorGreenOut)
}

// buildInitialise maps public arguments into the root frame and starts
// execution.
func (b *recursiveMachineBuilder) buildInitialise() {
	action := b.newAction([]deciderCondition{
		rmCondition(b.s.mode, "=", recursiveModeInit, ""),
	}, "start root call")
	action.targetRow(1)
	commands := []signalID{b.s.pc, b.s.mode}
	action.writeConstant(b.pcData, b.program.entry)
	action.writeConstant(b.modeData, recursiveModeRun)
	for i, slot := range b.program.params {
		action.writePublicArgument(i, b.slotData[slot])
		commands = append(commands, b.s.slots[slot])
	}
	action.command(commands...)
}

// buildPop moves execution from a completed child back to its caller frame.
func (b *recursiveMachineBuilder) buildPop() {
	action := b.newAction([]deciderCondition{
		rmCondition(b.s.mode, "=", recursiveModePop, ""),
	}, "return: pop frame")
	action.writeStateDelta(b.spData, b.s.sp, -1)
	action.writeConstant(b.modeData, recursiveModeResume)
	action.command(b.s.sp, b.s.mode)
}

// buildPresent delays DONE while the public result and its display pipeline
// settle, keeping RUNNING visible for one final action.
func (b *recursiveMachineBuilder) buildPresent() {
	action := b.newAction([]deciderCondition{
		rmCondition(b.s.mode, "=", recursiveModePresent, ""),
	}, "publish result")
	action.writeConstant(b.modeData, recursiveModeDone)
	action.command(b.s.mode)
}

// buildResult gates the public value and presents observable execution status.
func (b *recursiveMachineBuilder) buildResult() {
	b.layout.group(5)
	done := b.decider(b.layout.take(), []deciderCondition{
		rmCondition(b.s.mode, "=", recursiveModeDone, ""),
	}, []deciderOutput{{
		Signal: &b.s.action, CopyCountFromInput: false,
	}})
	b.stateBus.add(done, connectorGreenIn)
	donePanelNet := b.net(green)
	donePanelNet.add(done, connectorGreenOut)

	ready := b.decider(b.layout.take(), []deciderCondition{
		rmCondition(b.s.mode, ">=", recursiveModePresent, ""),
		rmCondition(b.s.mode, "<", recursiveModeOverflow, "and"),
	}, []deciderOutput{{
		Signal: &b.s.action, CopyCountFromInput: false,
	}})
	b.stateBus.add(ready, connectorGreenIn)
	readyNet := b.net(red)
	readyNet.add(ready, connectorRedOut)

	result := b.multiplySignals(
		b.layout.take(), b.s.result, b.s.action, b.public.result,
	)
	b.stateBus.add(result, connectorGreenIn)
	readyNet.add(result, connectorRedIn)
	b.e.bind(b.self.port("result"), result, connectorGreenOut)

	b.runningNet = b.runningStatus(b.layout.take())
	overflowNet := b.statusMode(
		b.layout.take(), "=", recursiveModeOverflow,
	)
	width := recursiveMachineFootprintWidth(b.program, b.frameX)
	b.staticPanel(
		recursiveMachinePoint{width - 15, 0},
		"output / status",
	)
	b.statusPanel(
		recursiveMachinePoint{width - 9, 0}, "RUNNING", b.runningNet,
	)
	b.statusPanel(
		recursiveMachinePoint{width - 5, 0}, "DONE", donePanelNet,
	)
	b.statusPanel(
		recursiveMachinePoint{width - 1, 0}, "STACK OVERFLOW", overflowNet,
	)
}

// runningStatus derives the shared indication used while execution can advance.
func (b *recursiveMachineBuilder) runningStatus(
	p recursiveMachinePoint,
) *recursiveMachineNet {
	h := b.decider(p, []deciderCondition{
		rmCondition(b.s.arm, "=", 1, ""),
		rmCondition(b.s.mode, "<", recursiveModeDone, "and"),
	}, []deciderOutput{{
		Signal: &b.s.action, CopyCountFromInput: false,
	}})
	b.stateBus.add(h, connectorGreenIn)
	net := b.net(green)
	net.add(h, connectorGreenOut)
	return net
}

// statusMode turns one mode comparison into a reusable status-panel network.
func (b *recursiveMachineBuilder) statusMode(
	p recursiveMachinePoint,
	comparator string,
	mode int,
) *recursiveMachineNet {
	h := b.decider(p, []deciderCondition{
		rmCondition(b.s.mode, comparator, mode, ""),
	}, []deciderOutput{{
		Signal: &b.s.action, CopyCountFromInput: false,
	}})
	b.stateBus.add(h, connectorGreenIn)
	net := b.net(green)
	net.add(h, connectorGreenOut)
	return net
}

// buildRows creates the addressed frame bank that makes bounded recursion
// viable.
func (b *recursiveMachineBuilder) buildRows() {
	start := recursiveMachineFrameY
	fields := append([]signalID{b.s.pc}, b.s.slots...)
	data := append(
		[]*recursiveMachineNet{b.pcData}, b.slotData...,
	)
	for row := range recursiveMachineRows {
		base := start + row*recursiveMachineRowHeight
		label := fmt.Sprintf("frame %02d", row)
		if row == 0 {
			label = "frame 00: root"
		}
		b.staticPanel(
			recursiveMachinePoint{b.frameX, base},
			label,
		)
		address := b.net(red)
		for fieldIndex, field := range fields {
			x := b.frameX +
				fieldIndex*recursiveMachineFrameFieldWidth
			cell := b.cellAt(
				recursiveMachinePoint{x, base + 1},
				data[fieldIndex],
				field,
			)
			read := b.multiplySignals(
				recursiveMachinePoint{x + 3, base + 1},
				field,
				b.s.action,
				field,
			)
			value := b.net(green)
			value.add(cell.output, connectorGreenOut)
			value.add(read, connectorGreenIn)
			address.add(read, connectorRedIn)
			b.stateBus.add(read, connectorGreenOut)

			selector := b.decider(
				recursiveMachinePoint{x + 4, base + 1},
				[]deciderCondition{
					rmCondition(field, "!=", 0, ""),
					rmCondition(
						b.s.target, "=", row+1, "and",
					),
				},
				[]deciderOutput{{
					Signal: &b.s.write, CopyCountFromInput: false,
				}},
			)
			b.commandBus.add(selector, connectorGreenIn)
			b.connectWrite(cell, selector)
		}
		selectRow := b.decider(
			recursiveMachinePoint{
				b.frameX +
					len(fields)*recursiveMachineFrameFieldWidth,
				base + 1,
			},
			[]deciderCondition{
				rmCondition(b.s.sp, "=", row, ""),
			},
			[]deciderOutput{{
				Signal: &b.s.action, CopyCountFromInput: false,
			}},
		)
		b.stateBus.add(selectRow, connectorGreenIn)
		address.add(selectRow, connectorRedOut)
		lamp := b.activityLamp(
			recursiveMachinePoint{
				b.frameX + recursiveMachineFrameLampX,
				base,
			},
			2,
		)
		address.add(lamp, connectorRedIn)
		b.runningNet.add(lamp, connectorGreenIn)
	}
}
