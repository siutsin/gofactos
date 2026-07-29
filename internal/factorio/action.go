// This file emits recursive machine instructions and guarded actions.
package factorio

import (
	"fmt"
	"go/token"
)

// buildInstructions gives every compiled opcode its matching circuit action.
func (b *recursiveMachineBuilder) buildInstructions() {
	for _, instruction := range b.program.instructions {
		switch instruction.op {
		case recursiveOpBinary:
			b.buildBinary(instruction)
		case recursiveOpUnary:
			b.buildUnary(instruction)
		case recursiveOpBranch:
			b.buildBranch(instruction)
		case recursiveOpJump:
			b.buildJump(instruction)
		case recursiveOpCall:
			b.buildCall(instruction)
		case recursiveOpResume:
			b.buildResume(instruction)
		case recursiveOpReturn:
			b.buildReturn(instruction)
		default:
			panic("factorio: recursive machine unsupported opcode")
		}
	}
}

// buildBinary lowers arithmetic and comparisons into guarded slot updates.
func (b *recursiveMachineBuilder) buildBinary(
	instruction recursiveInstruction,
) {
	entry, ok := binOpMap[instruction.operator]
	if !ok {
		panic("factorio: recursive machine unsupported binary operator")
	}
	verb := "calculate"
	if entry.entityName == deciderCombinatorName {
		verb = "compare"
	}
	action := b.instructionAction(
		instruction.pc,
		fmt.Sprintf("%s %s", verb, entry.operation),
	)
	if entry.entityName == deciderCombinatorName {
		action.writeComparison(
			b.slotData[instruction.dest],
			instruction.x,
			entry.operation,
			instruction.y,
		)
	} else {
		action.writeArithmetic(
			b.slotData[instruction.dest],
			instruction.x,
			entry.operation,
			instruction.y,
		)
	}
	action.writeConstant(b.pcData, instruction.target)
	action.targetRow(1)
	action.command(b.s.slots[instruction.dest], b.s.pc)
}

// buildUnary lowers the supported integer negation into a guarded slot update.
func (b *recursiveMachineBuilder) buildUnary(
	instruction recursiveInstruction,
) {
	action := b.instructionAction(instruction.pc, "negate")
	if instruction.operator != token.SUB {
		panic("factorio: recursive machine unsupported unary operator")
	}
	action.writeNegated(b.slotData[instruction.dest], instruction.x)
	action.writeConstant(b.pcData, instruction.target)
	action.targetRow(1)
	action.command(b.s.slots[instruction.dest], b.s.pc)
}

// buildBranch emits both runtime arms or the one constant-selected arm.
func (b *recursiveMachineBuilder) buildBranch(
	instruction recursiveInstruction,
) {
	b.buildBranchArm(
		instruction,
		true,
		instruction.target,
		instruction.moves,
	)
	b.buildBranchArm(
		instruction,
		false,
		instruction.alternate,
		instruction.alternateMoves,
	)
}

// buildBranchArm folds constant conditions and emits one guarded branch path.
func (b *recursiveMachineBuilder) buildBranchArm(
	instruction recursiveInstruction,
	want bool,
	target int,
	moves []recursiveMove,
) {
	conditions := b.instructionConditions(instruction.pc)
	if instruction.x.isConstant {
		truth := instruction.x.constant == 1
		if truth != want {
			return
		}
	} else {
		comparator := "="
		if !want {
			comparator = "!="
		}
		conditions = append(conditions,
			rmCondition(
				b.s.slots[instruction.x.slot], comparator, 1, "and",
			),
		)
	}
	action := b.newAction(conditions, fmt.Sprintf(
		"PC %02d: branch %t",
		instruction.pc,
		want,
	))
	commands := []signalID{b.s.pc}
	action.writeConstant(b.pcData, target)
	for _, move := range moves {
		action.writeOperand(b.slotData[move.dest], move.source)
		commands = append(commands, b.s.slots[move.dest])
	}
	action.targetRow(1)
	action.command(commands...)
}

// buildJump transfers control and applies the destination's parallel phi moves.
func (b *recursiveMachineBuilder) buildJump(
	instruction recursiveInstruction,
) {
	action := b.instructionAction(instruction.pc, "jump")
	commands := []signalID{b.s.pc}
	action.writeConstant(b.pcData, instruction.target)
	for _, move := range instruction.moves {
		action.writeOperand(b.slotData[move.dest], move.source)
		commands = append(commands, b.s.slots[move.dest])
	}
	action.targetRow(1)
	action.command(commands...)
}

// buildCall protects stack bounds while entering a new addressed child frame.
func (b *recursiveMachineBuilder) buildCall(
	instruction recursiveInstruction,
) {
	setupConditions := b.instructionConditions(instruction.pc)
	setupConditions = append(setupConditions,
		rmCondition(b.s.sp, "<", recursiveMachineDepth, "and"),
	)
	setup := b.newAction(
		setupConditions,
		fmt.Sprintf("PC %02d: call", instruction.pc),
	)
	setup.targetRow(1)
	setup.writeConstant(b.pcData, instruction.continuation)
	setup.writeConstant(b.modeData, recursiveModeEnter)
	setup.command(b.s.pc, b.s.mode)

	enter := b.newAction([]deciderCondition{
		rmCondition(b.s.mode, "=", recursiveModeEnter, ""),
		rmCondition(b.s.pc, "=", instruction.continuation, "and"),
	}, "enter child frame")
	commands := []signalID{b.s.pc, b.s.sp, b.s.mode}
	enter.targetRow(2)
	enter.writeConstant(b.pcData, instruction.target)
	for i, argument := range instruction.args {
		slot := b.program.params[i]
		enter.writeOperand(b.slotData[slot], argument)
		commands = append(commands, b.s.slots[slot])
	}
	enter.writeStateDelta(b.spData, b.s.sp, 1)
	enter.writeConstant(b.modeData, recursiveModeRun)
	enter.command(commands...)

	overflowConditions := b.instructionConditions(instruction.pc)
	overflowConditions = append(overflowConditions,
		rmCondition(b.s.sp, ">=", recursiveMachineDepth, "and"),
	)
	overflow := b.newAction(
		overflowConditions,
		fmt.Sprintf("PC %02d: stack full", instruction.pc),
	)
	overflow.writeConstant(b.modeData, recursiveModeOverflow)
	overflow.command(b.s.mode)
}

// buildResume copies a child result into its caller before execution continues.
func (b *recursiveMachineBuilder) buildResume(
	resume recursiveInstruction,
) {
	conditions := []deciderCondition{
		rmCondition(b.s.mode, "=", recursiveModeResume, ""),
		rmCondition(b.s.pc, "=", resume.pc, "and"),
	}
	action := b.newAction(
		conditions,
		fmt.Sprintf("PC %02d: resume caller", resume.pc),
	)
	action.targetRow(1)
	action.writeStateValue(b.slotData[resume.dest], b.s.ret)
	action.writeConstant(b.pcData, resume.target)
	action.writeConstant(b.modeData, recursiveModeRun)
	action.command(b.s.slots[resume.dest], b.s.pc, b.s.mode)
}

// buildReturn separates nested returns from publication of the root result.
func (b *recursiveMachineBuilder) buildReturn(
	instruction recursiveInstruction,
) {
	nestedConditions := b.instructionConditions(instruction.pc)
	nestedConditions = append(nestedConditions,
		rmCondition(b.s.sp, ">", 0, "and"),
	)
	nested := b.newAction(
		nestedConditions,
		fmt.Sprintf("PC %02d: return to caller", instruction.pc),
	)
	nested.writeOperand(b.retData, instruction.x)
	nested.writeConstant(b.modeData, recursiveModePop)
	nested.command(b.s.ret, b.s.mode)

	rootConditions := b.instructionConditions(instruction.pc)
	rootConditions = append(rootConditions,
		rmCondition(b.s.sp, "=", 0, "and"),
	)
	root := b.newAction(
		rootConditions,
		fmt.Sprintf("PC %02d: finish root call", instruction.pc),
	)
	root.writeOperand(b.resultData, instruction.x)
	root.writeConstant(b.modeData, recursiveModePresent)
	root.command(b.s.result, b.s.mode)
}

// instructionAction gives an opcode the standard running-mode and PC guard.
func (b *recursiveMachineBuilder) instructionAction(
	pc int,
	label string,
) *recursiveMachineAction {
	return b.newAction(
		b.instructionConditions(pc),
		fmt.Sprintf("PC %02d: %s", pc, label),
	)
}

// instructionConditions centralises the guard shared by executable opcodes.
func (b *recursiveMachineBuilder) instructionConditions(
	pc int,
) []deciderCondition {
	return []deciderCondition{
		rmCondition(b.s.mode, "=", recursiveModeRun, ""),
		rmCondition(b.s.pc, "=", pc, "and"),
	}
}

type recursiveMachineAction struct {
	b     *recursiveMachineBuilder
	net   *recursiveMachineNet
	start recursiveMachinePoint
}

// newAction creates an armed predicate network for one exclusive state change.
func (b *recursiveMachineBuilder) newAction(
	conditions []deciderCondition,
	label string,
) *recursiveMachineAction {
	b.layout.group(48)
	conditions = append(conditions,
		rmCondition(b.s.arm, "=", 1, "and"),
	)
	start := b.layout.take()
	h := b.decider(start, conditions, []deciderOutput{{
		Signal: &b.s.action, CopyCountFromInput: false,
	}})
	b.stateBus.add(h, connectorGreenIn)
	net := b.net(red)
	net.add(h, connectorRedOut)
	lamp := b.activityLamp(
		recursiveMachinePoint{start.x, start.y + 2},
		1,
	)
	net.add(lamp, connectorRedIn)
	b.staticPanel(
		recursiveMachinePoint{start.x + 1, start.y + 2},
		label,
	)
	return &recursiveMachineAction{b: b, net: net, start: start}
}

// targetRow addresses the frame an action must read or update.
func (a *recursiveMachineAction) targetRow(offset int) {
	b := a.b
	raw := b.arithConstant(
		b.layout.take(), b.s.sp, "+", offset, privateData,
	)
	b.stateBus.add(raw, connectorGreenIn)
	rawNet := b.net(green)
	rawNet.add(raw, connectorGreenOut)
	target := b.multiplySignals(
		b.layout.take(), privateData, b.s.action, b.s.target,
	)
	rawNet.add(target, connectorGreenIn)
	a.net.add(target, connectorRedIn)
	b.commandBus.add(target, connectorGreenOut)
}

// command commits selected field writes only on an incoming clock step.
func (a *recursiveMachineAction) command(fields ...signalID) {
	b := a.b
	seen := make(map[signalID]bool, len(fields))
	outputs := make([]deciderOutput, 0, len(fields))
	for _, field := range fields {
		if seen[field] {
			continue
		}
		seen[field] = true
		field := field
		outputs = append(outputs, deciderOutput{
			Signal: &field, CopyCountFromInput: false,
		})
	}
	h := b.decider(b.layout.take(), []deciderCondition{
		rmCondition(b.s.action, "!=", 0, ""),
		rmCondition(b.s.step, "!=", 0, "and"),
	}, outputs)
	a.net.add(h, connectorRedIn)
	b.stepBus.add(h, connectorGreenIn)
	b.commandBus.add(h, connectorGreenOut)
	b.layout.padFrom(
		a.start,
		recursiveMachineActionLabelWidth,
	)
}

// writePublicArgument brings one external argument into guarded frame data.
func (a *recursiveMachineAction) writePublicArgument(
	index int,
	output *recursiveMachineNet,
) {
	b := a.b
	h := b.multiplySignals(
		b.layout.take(), b.public.args[index], b.s.action, b.s.data,
	)
	b.e.bind(b.self.port(fmt.Sprintf("arg%d", index)), h, connectorGreenIn)
	a.net.add(h, connectorRedIn)
	output.add(h, connectorGreenOut)
}

// writeConstant turns an action predicate into a constant state update.
func (a *recursiveMachineAction) writeConstant(
	output *recursiveMachineNet,
	value int,
) {
	b := a.b
	h := b.arithConstant(
		b.layout.take(), b.s.action, "*", value, b.s.data,
	)
	a.net.add(h, connectorRedIn)
	output.add(h, connectorGreenOut)
}

// writeStateValue copies shared state only while its owning action is active.
func (a *recursiveMachineAction) writeStateValue(
	output *recursiveMachineNet,
	value signalID,
) {
	b := a.b
	h := b.multiplySignals(
		b.layout.take(), value, b.s.action, b.s.data,
	)
	b.stateBus.add(h, connectorGreenIn)
	a.net.add(h, connectorRedIn)
	output.add(h, connectorGreenOut)
}

// writeStateDelta supports stack movement without exposing ungated arithmetic.
func (a *recursiveMachineAction) writeStateDelta(
	output *recursiveMachineNet,
	value signalID,
	delta int,
) {
	b := a.b
	raw := b.arithConstant(
		b.layout.take(), value, "+", delta, privateData,
	)
	b.stateBus.add(raw, connectorGreenIn)
	rawNet := b.net(green)
	rawNet.add(raw, connectorGreenOut)
	a.gateRawValue(rawNet, output)
}

// writeOperand selects the correct update path for constants and stored values.
func (a *recursiveMachineAction) writeOperand(
	output *recursiveMachineNet,
	operand recursiveOperand,
) {
	if operand.isConstant {
		a.writeConstant(output, operand.constant)
		return
	}
	a.writeStateValue(output, a.b.s.slots[operand.slot])
}

// writeNegated provides the recursive program's sole supported unary operation.
func (a *recursiveMachineAction) writeNegated(
	output *recursiveMachineNet,
	operand recursiveOperand,
) {
	a.writeArithmetic(output, operand, "*", recursiveOperand{
		constant: -1, isConstant: true,
	})
}

// writeArithmetic prevents inactive opcode results from reaching state storage.
func (a *recursiveMachineAction) writeArithmetic(
	output *recursiveMachineNet,
	x recursiveOperand,
	op string,
	y recursiveOperand,
) {
	b := a.b
	raw := b.arithOperands(
		b.layout.take(), x, op, y, privateData,
	)
	b.connectOperands(raw, x, y)
	rawNet := b.net(green)
	rawNet.add(raw, connectorGreenOut)
	a.gateRawValue(rawNet, output)
}

// writeComparison encodes Go Boolean results as the machine's signed signals.
func (a *recursiveMachineAction) writeComparison(
	output *recursiveMachineNet,
	x recursiveOperand,
	comparator string,
	y recursiveOperand,
) {
	b := a.b
	trueSignal := privateData
	falseSignal := privateInc
	condition := b.operandCondition(x, comparator, y)
	rawNet := b.net(green)
	trueValue := b.decider(b.layout.take(), []deciderCondition{condition},
		[]deciderOutput{{
			Signal: &trueSignal, CopyCountFromInput: false,
		}})
	b.stateBus.add(trueValue, connectorGreenIn)
	rawNet.add(trueValue, connectorGreenOut)

	condition.Comparator = negateComparator(condition.Comparator)
	falseValue := b.decider(
		b.layout.take(),
		[]deciderCondition{condition},
		[]deciderOutput{{
			Signal: &falseSignal, CopyCountFromInput: false,
		}},
	)
	b.stateBus.add(falseValue, connectorGreenIn)
	falseNet := b.net(green)
	falseNet.add(falseValue, connectorGreenOut)
	negative := b.arithConstant(
		b.layout.take(), falseSignal, "*", -1, trueSignal,
	)
	falseNet.add(negative, connectorGreenIn)
	rawNet.add(negative, connectorGreenOut)

	a.gateRawValue(rawNet, output)
}

// gateRawValue exposes a computed value only while this action is selected.
func (a *recursiveMachineAction) gateRawValue(
	rawNet *recursiveMachineNet,
	output *recursiveMachineNet,
) {
	b := a.b
	gate := b.multiplySignals(
		b.layout.take(), privateData, b.s.action, b.s.data,
	)
	rawNet.add(gate, connectorGreenIn)
	a.net.add(gate, connectorRedIn)
	output.add(gate, connectorGreenOut)
}

// operandCondition normalises constant and signal placement for a decider.
func (b *recursiveMachineBuilder) operandCondition(
	x recursiveOperand,
	comparator string,
	y recursiveOperand,
) deciderCondition {
	if x.isConstant {
		return rmSignalCondition(
			b.s.slots[y.slot], rmSwapComparator(comparator), x.constant,
		)
	}
	condition := deciderCondition{
		FirstSignal: &b.s.slots[x.slot], Comparator: comparator,
	}
	if y.isConstant {
		condition.Constant = &y.constant
	} else {
		condition.SecondSignal = &b.s.slots[y.slot]
	}
	return condition
}

// rmSignalCondition constructs the common signal-to-constant comparison form.
func rmSignalCondition(
	first signalID,
	comparator string,
	constant int,
) deciderCondition {
	return deciderCondition{
		FirstSignal: &first, Comparator: comparator, Constant: &constant,
	}
}

// rmSwapComparator preserves meaning when a constant operand moves to the
// right.
func rmSwapComparator(comparator string) string {
	switch comparator {
	case "<":
		return ">"
	case "<=", "≤":
		return "≥"
	case ">":
		return "<"
	case ">=", "≥":
		return "≤"
	default:
		return comparator
	}
}

// connectOperands joins arithmetic to state only when it consumes stored
// values.
func (b *recursiveMachineBuilder) connectOperands(
	h handle,
	operands ...recursiveOperand,
) {
	for _, operand := range operands {
		if !operand.isConstant {
			b.stateBus.add(h, connectorGreenIn)
			return
		}
	}
}
