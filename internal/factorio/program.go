// This file compiles supported self-recursive SSA into a bounded program.
package factorio

import (
	"fmt"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

const maxRecursiveProgramSlots = 10

type recursiveValueKind uint8

const (
	recursiveValueInt recursiveValueKind = iota
	recursiveValueBool
)

type recursiveOpcode uint8

const (
	recursiveOpBinary recursiveOpcode = iota
	recursiveOpUnary
	recursiveOpBranch
	recursiveOpJump
	recursiveOpCall
	recursiveOpResume
	recursiveOpReturn
)

type recursiveOperand struct {
	slot       int
	constant   int
	isConstant bool
}

type recursiveMove struct {
	dest   int
	source recursiveOperand
}

// recursiveInstruction is one executable machine step. Edge-specific phi moves
// are parallel assignments: every source is read before any destination is set.
type recursiveInstruction struct {
	pc             int
	op             recursiveOpcode
	operator       token.Token
	dest           int
	x              recursiveOperand
	y              recursiveOperand
	args           []recursiveOperand
	target         int
	alternate      int
	continuation   int
	moves          []recursiveMove
	alternateMoves []recursiveMove
}

// recursiveProgram is the bounded executable form of one direct-self-
// recursive function. PCs start at one; instruction pc is stored at pc-1.
type recursiveProgram struct {
	fn           *ssa.Function
	entry        int
	params       []int
	slotCount    int
	instructions []recursiveInstruction
}

type recursiveProgramBuilder struct {
	fn        *ssa.Function
	blocks    []*ssa.BasicBlock
	program   *recursiveProgram
	slotFor   map[ssa.Value]int
	pcFor     map[ssa.Instruction]int
	resumeFor map[*ssa.Call]int
	firstPC   map[*ssa.BasicBlock]int
	selfCalls int
	nextPC    int
}

// planRecursiveProgram validates and compiles the supported recursion subset
// so circuit construction can consume a closed, bounded instruction stream.
func planRecursiveProgram(fn *ssa.Function) (*recursiveProgram, error) {
	builder, err := newRecursiveProgramBuilder(fn)
	if err != nil {
		return nil, err
	}
	if err := builder.allocateSlots(); err != nil {
		return nil, err
	}
	if builder.selfCalls == 0 {
		return nil, builder.errorf("expected at least one direct self call")
	}
	if builder.program.slotCount > maxRecursiveProgramSlots {
		return nil, builder.errorf(
			"requires %d value slots; limit is %d",
			builder.program.slotCount,
			maxRecursiveProgramSlots,
		)
	}
	if err := builder.assignPCs(); err != nil {
		return nil, err
	}
	if err := builder.compile(); err != nil {
		return nil, err
	}
	return builder.program, nil
}

// newRecursiveProgramBuilder establishes validated state for one compilation.
func newRecursiveProgramBuilder(
	fn *ssa.Function,
) (*recursiveProgramBuilder, error) {
	if fn == nil {
		return nil, fmt.Errorf(
			"select: unsupported recursive body <nil>: missing function",
		)
	}
	builder := &recursiveProgramBuilder{
		fn:        fn,
		blocks:    feasibleBlocks(fn),
		slotFor:   make(map[ssa.Value]int),
		pcFor:     make(map[ssa.Instruction]int),
		resumeFor: make(map[*ssa.Call]int),
		firstPC:   make(map[*ssa.BasicBlock]int),
		nextPC:    1,
	}
	if err := builder.validateFunction(); err != nil {
		return nil, err
	}
	builder.program = &recursiveProgram{
		fn:     fn,
		params: make([]int, len(fn.Params)),
	}
	return builder, nil
}

// validateFunction rejects function shapes the runtime machine cannot
// represent.
func (b *recursiveProgramBuilder) validateFunction() error {
	fn := b.fn
	if fn.Signature.Recv() != nil || fn.Parent() != nil {
		return b.errorf("function must be a selected top-level function")
	}
	if fn.Signature.Variadic() || isGenericFunction(fn) {
		return b.errorf("generic and variadic functions are unsupported")
	}
	for i, parameter := range fn.Params {
		if _, ok := recursiveKind(parameter.Type()); !ok {
			return b.errorf(
				"parameter %d has type %s; want int or bool",
				i+1,
				parameter.Type(),
			)
		}
	}
	results := fn.Signature.Results()
	if results.Len() != 1 {
		return b.errorf(
			"expected exactly one int or bool result, got %d",
			results.Len(),
		)
	}
	if _, ok := recursiveKind(results.At(0).Type()); !ok {
		return b.errorf(
			"result has type %s; want int or bool",
			results.At(0).Type(),
		)
	}
	if len(b.blocks) == 0 {
		return b.errorf("function body is unavailable")
	}
	if hasControlFlowCycle(fn) {
		return b.errorf("control-flow loops are unsupported")
	}
	return nil
}

// recursiveKind defines the value types that recursive slots can preserve.
func recursiveKind(t types.Type) (recursiveValueKind, bool) {
	basic, ok := unaliasedBasic(t)
	if !ok {
		return 0, false
	}
	if basic.Kind() == types.Int {
		return recursiveValueInt, true
	}
	if basic.Kind() == types.Bool {
		return recursiveValueBool, true
	}
	return 0, false
}

// recursiveOperandKind normalises constants before enforcing the slot type set.
func recursiveOperandKind(value ssa.Value) (recursiveValueKind, bool) {
	t := value.Type()
	if _, ok := value.(*ssa.Const); ok {
		t = types.Default(t)
	}
	return recursiveKind(t)
}

// allocateSlots fixes the frame layout before program counters are assigned.
func (b *recursiveProgramBuilder) allocateSlots() error {
	if err := b.allocateParameterSlots(); err != nil {
		return err
	}
	if err := b.allocateInstructionSlots(); err != nil {
		return err
	}
	return b.validateInstructions()
}

// allocateParameterSlots gives each public argument a stable frame location.
func (b *recursiveProgramBuilder) allocateParameterSlots() error {
	for i, parameter := range b.fn.Params {
		slot, err := b.allocateSlot(parameter)
		if err != nil {
			return err
		}
		b.program.params[i] = slot
	}
	return nil
}

// allocateInstructionSlots reserves storage for every produced runtime value.
func (b *recursiveProgramBuilder) allocateInstructionSlots() error {
	for _, block := range b.blocks {
		for _, instruction := range block.Instrs {
			if value, ok := recursiveProducedValue(instruction); ok {
				if _, err := b.allocateSlot(value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// validateInstructions prevents unsupported SSA from reaching circuit building.
func (b *recursiveProgramBuilder) validateInstructions() error {
	for _, block := range b.blocks {
		for _, instruction := range block.Instrs {
			if err := b.validateInstruction(instruction); err != nil {
				return err
			}
		}
	}
	return nil
}

// allocateSlot keeps repeated SSA uses bound to one frame location.
func (b *recursiveProgramBuilder) allocateSlot(
	value ssa.Value,
) (int, error) {
	if slot, ok := b.slotFor[value]; ok {
		return slot, nil
	}
	_, ok := recursiveKind(value.Type())
	if !ok {
		return 0, b.errorf(
			"value %s has type %s; want int or bool",
			value.Name(),
			value.Type(),
		)
	}
	slot := len(b.slotFor)
	b.slotFor[value] = slot
	b.program.slotCount++
	return slot, nil
}

// recursiveProducedValue identifies instructions whose results require slots.
func recursiveProducedValue(
	instruction ssa.Instruction,
) (ssa.Value, bool) {
	switch value := instruction.(type) {
	case *ssa.BinOp:
		return value, true
	case *ssa.UnOp:
		return value, true
	case *ssa.Phi:
		return value, true
	case *ssa.Call:
		return value, true
	default:
		return nil, false
	}
}

// validateInstruction dispatches each SSA form to its representation checks.
func (b *recursiveProgramBuilder) validateInstruction(
	instruction ssa.Instruction,
) error {
	switch value := instruction.(type) {
	case *ssa.DebugRef, *ssa.Jump:
		return nil
	case *ssa.BinOp:
		return b.validateBinary(value)
	case *ssa.UnOp:
		return b.validateUnary(value)
	case *ssa.Phi:
		return b.validatePhi(value)
	case *ssa.If:
		return b.validateBranch(value)
	case *ssa.Return:
		return b.validateReturn(value)
	case *ssa.Call:
		return b.validateSelfCall(value)
	case *ssa.Go, *ssa.Defer:
		return b.errorf("non-ordinary calls are unsupported")
	default:
		return b.errorf("instruction %T is unsupported", instruction)
	}
}

// validateBinary limits binary operations to combinator-supported operands.
func (b *recursiveProgramBuilder) validateBinary(value *ssa.BinOp) error {
	entry, ok := binOpMap[value.Op]
	if !ok {
		return b.errorf("binary operator %s is unsupported", value.Op)
	}
	// A decider names a frame slot on its left-hand side and has no
	// first-constant field, so a comparison between two constants has no slot
	// to read. Reject it rather than silently compare slot 0. Arithmetic is
	// unaffected: an arithmetic combinator takes a constant on both sides.
	if entry.entityName == deciderCombinatorName {
		_, constantX := value.X.(*ssa.Const)
		_, constantY := value.Y.(*ssa.Const)
		if constantX && constantY {
			return b.errorf(
				"comparison %s between two constants is unsupported",
				value.Op,
			)
		}
	}
	return b.validateOperands(value.X, value.Y)
}

// validateUnary keeps unary lowering within the machine's integer negation.
func (b *recursiveProgramBuilder) validateUnary(value *ssa.UnOp) error {
	if value.Op != token.SUB {
		return b.errorf("unary operator %s is unsupported", value.Op)
	}
	kind, ok := recursiveKind(value.X.Type())
	if !ok || kind != recursiveValueInt {
		return b.errorf("unary - requires an int operand")
	}
	return nil
}

// validatePhi protects edge assignments from malformed SSA and bad operands.
func (b *recursiveProgramBuilder) validatePhi(value *ssa.Phi) error {
	if len(value.Edges) != len(value.Block().Preds) {
		return b.errorf(
			"phi %s has %d edges for %d predecessors",
			value.Name(),
			len(value.Edges),
			len(value.Block().Preds),
		)
	}
	edges := feasiblePhiEdges(value)
	operands := make([]ssa.Value, len(edges))
	for index, edge := range edges {
		operands[index] = edge.value
	}
	return b.validateOperands(operands...)
}

// validateBranch requires a Boolean condition and the two raw SSA successors.
func (b *recursiveProgramBuilder) validateBranch(value *ssa.If) error {
	kind, ok := recursiveOperandKind(value.Cond)
	if !ok || kind != recursiveValueBool {
		return b.errorf("branch condition must be bool")
	}
	if len(value.Block().Succs) != 2 {
		return b.errorf(
			"branch has %d successors; want 2",
			len(value.Block().Succs),
		)
	}
	return nil
}

// validateReturn guarantees the machine has one representable result to
// publish.
func (b *recursiveProgramBuilder) validateReturn(value *ssa.Return) error {
	if len(value.Results) != 1 {
		return b.errorf(
			"return has %d values; want 1",
			len(value.Results),
		)
	}
	return b.validateOperands(value.Results...)
}

// validateOperands applies the shared runtime-operand contract to a value list.
func (b *recursiveProgramBuilder) validateOperands(
	values ...ssa.Value,
) error {
	for _, value := range values {
		if _, err := b.operand(value); err != nil {
			return err
		}
	}
	return nil
}

// validateSelfCall confines runtime recursion to direct calls of the same root.
func (b *recursiveProgramBuilder) validateSelfCall(call *ssa.Call) error {
	callee := call.Common().StaticCallee()
	if callee != b.fn {
		name := call.Common().String()
		if callee != nil {
			name = callee.Name()
		}
		return b.errorf(
			"call to %s is unsupported; only direct self calls are supported",
			name,
		)
	}
	if len(call.Common().Args) != len(b.fn.Params) {
		return b.errorf(
			"self call has %d arguments; want %d",
			len(call.Common().Args),
			len(b.fn.Params),
		)
	}
	if err := b.validateOperands(call.Common().Args...); err != nil {
		return err
	}
	b.selfCalls++
	return nil
}

// assignPCs creates stable addresses, including return points after self-calls.
func (b *recursiveProgramBuilder) assignPCs() error {
	for _, block := range b.blocks {
		for _, instruction := range block.Instrs {
			if !recursiveExecutable(instruction) {
				continue
			}
			if b.firstPC[block] == 0 {
				b.firstPC[block] = b.nextPC
			}
			b.pcFor[instruction] = b.nextPC
			b.nextPC++
			if call, ok := instruction.(*ssa.Call); ok {
				b.resumeFor[call] = b.nextPC
				b.nextPC++
			}
		}
		if b.firstPC[block] == 0 {
			return b.errorf("basic block %d has no executable instruction", block.Index)
		}
	}
	b.program.entry = b.firstPC[b.blocks[0]]
	return nil
}

// recursiveExecutable distinguishes runtime steps from SSA bookkeeping.
func recursiveExecutable(instruction ssa.Instruction) bool {
	switch instruction.(type) {
	case *ssa.DebugRef, *ssa.Phi:
		return false
	default:
		return true
	}
}

// compile fills the addressed program only after validation and slot
// allocation.
func (b *recursiveProgramBuilder) compile() error {
	b.program.instructions = make(
		[]recursiveInstruction,
		b.nextPC-1,
	)
	for _, block := range b.blocks {
		for index, instruction := range block.Instrs {
			if !recursiveExecutable(instruction) {
				continue
			}
			compiled, resume, err := b.compileInstruction(
				block,
				index,
				instruction,
			)
			if err != nil {
				return err
			}
			b.program.instructions[compiled.pc-1] = compiled
			if resume != nil {
				b.program.instructions[resume.pc-1] = *resume
			}
		}
	}
	return nil
}

// compileInstruction selects the encoded form for one executable SSA step.
func (b *recursiveProgramBuilder) compileInstruction(
	block *ssa.BasicBlock,
	index int,
	instruction ssa.Instruction,
) (recursiveInstruction, *recursiveInstruction, error) {
	pc := b.pcFor[instruction]
	switch value := instruction.(type) {
	case *ssa.BinOp:
		return b.compileBinary(block, index, pc, value)
	case *ssa.UnOp:
		return b.compileUnary(block, index, pc, value)
	case *ssa.If:
		return b.compileBranch(block, pc, value)
	case *ssa.Jump:
		return b.compileJump(block, pc)
	case *ssa.Call:
		return b.compileCall(block, index, pc, value)
	case *ssa.Return:
		return b.compileReturn(pc, value)
	default:
		return recursiveInstruction{}, nil, b.errorf(
			"instruction %T is unsupported",
			instruction,
		)
	}
}

// compileBinary preserves a binary result and its following control transfer.
func (b *recursiveProgramBuilder) compileBinary(
	block *ssa.BasicBlock,
	index int,
	pc int,
	value *ssa.BinOp,
) (recursiveInstruction, *recursiveInstruction, error) {
	x, err := b.operand(value.X)
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	y, err := b.operand(value.Y)
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	next, err := b.continuation(block, index)
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	return recursiveInstruction{
		pc:       pc,
		op:       recursiveOpBinary,
		operator: value.Op,
		dest:     b.slotFor[value],
		x:        x,
		y:        y,
		target:   next,
	}, nil, nil
}

// compileUnary preserves a negated result and its following control transfer.
func (b *recursiveProgramBuilder) compileUnary(
	block *ssa.BasicBlock,
	index int,
	pc int,
	value *ssa.UnOp,
) (recursiveInstruction, *recursiveInstruction, error) {
	x, err := b.operand(value.X)
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	next, err := b.continuation(block, index)
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	return recursiveInstruction{
		pc:       pc,
		op:       recursiveOpUnary,
		operator: value.Op,
		dest:     b.slotFor[value],
		x:        x,
		target:   next,
	}, nil, nil
}

// compileBranch records feasible destinations and their parallel phi moves.
func (b *recursiveProgramBuilder) compileBranch(
	block *ssa.BasicBlock,
	pc int,
	value *ssa.If,
) (recursiveInstruction, *recursiveInstruction, error) {
	condition, err := b.operand(value.Cond)
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	if successorIndex, constant := constantBranchIndex(value); constant {
		moves, moveErr := b.edgeMoves(block, successorIndex)
		if moveErr != nil {
			return recursiveInstruction{}, nil, moveErr
		}
		target := b.firstPC[block.Succs[successorIndex]]
		return recursiveInstruction{
			pc:             pc,
			op:             recursiveOpBranch,
			x:              condition,
			target:         target,
			alternate:      target,
			moves:          moves,
			alternateMoves: moves,
		}, nil, nil
	}
	trueMoves, err := b.edgeMoves(block, 0)
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	falseMoves, err := b.edgeMoves(block, 1)
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	return recursiveInstruction{
		pc:             pc,
		op:             recursiveOpBranch,
		x:              condition,
		target:         b.firstPC[block.Succs[0]],
		alternate:      b.firstPC[block.Succs[1]],
		moves:          trueMoves,
		alternateMoves: falseMoves,
	}, nil, nil
}

// compileJump carries edge-specific phi assignments across an unconditional
// edge.
func (b *recursiveProgramBuilder) compileJump(
	block *ssa.BasicBlock,
	pc int,
) (recursiveInstruction, *recursiveInstruction, error) {
	if len(block.Succs) != 1 {
		return recursiveInstruction{}, nil, b.errorf(
			"jump has %d successors; want 1",
			len(block.Succs),
		)
	}
	moves, err := b.edgeMoves(block, 0)
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	return recursiveInstruction{
		pc:     pc,
		op:     recursiveOpJump,
		target: b.firstPC[block.Succs[0]],
		moves:  moves,
	}, nil, nil
}

// compileCall pairs child-frame entry with the caller's private resume address.
func (b *recursiveProgramBuilder) compileCall(
	block *ssa.BasicBlock,
	index int,
	pc int,
	call *ssa.Call,
) (recursiveInstruction, *recursiveInstruction, error) {
	args := make([]recursiveOperand, len(call.Common().Args))
	for i, argument := range call.Common().Args {
		operand, err := b.operand(argument)
		if err != nil {
			return recursiveInstruction{}, nil, err
		}
		args[i] = operand
	}
	next, err := b.continuation(block, index)
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	resumePC := b.resumeFor[call]
	compiled := recursiveInstruction{
		pc:           pc,
		op:           recursiveOpCall,
		dest:         b.slotFor[call],
		args:         args,
		target:       b.program.entry,
		continuation: resumePC,
	}
	resume := &recursiveInstruction{
		pc:     resumePC,
		op:     recursiveOpResume,
		dest:   b.slotFor[call],
		target: next,
	}
	return compiled, resume, nil
}

// compileReturn records the value consumed by caller or public result handling.
func (b *recursiveProgramBuilder) compileReturn(
	pc int,
	value *ssa.Return,
) (recursiveInstruction, *recursiveInstruction, error) {
	result, err := b.operand(value.Results[0])
	if err != nil {
		return recursiveInstruction{}, nil, err
	}
	return recursiveInstruction{
		pc: pc,
		op: recursiveOpReturn,
		x:  result,
	}, nil, nil
}

// continuation finds the next address needed after a value-producing step.
func (b *recursiveProgramBuilder) continuation(
	block *ssa.BasicBlock,
	index int,
) (int, error) {
	for _, instruction := range block.Instrs[index+1:] {
		if pc := b.pcFor[instruction]; pc != 0 {
			return pc, nil
		}
	}
	return 0, b.errorf(
		"instruction %d in block %d has no continuation",
		index,
		block.Index,
	)
}

// edgeMoves translates successor phis into simultaneous frame assignments.
func (b *recursiveProgramBuilder) edgeMoves(
	from *ssa.BasicBlock,
	successorIndex int,
) ([]recursiveMove, error) {
	to := from.Succs[successorIndex]
	predIndex, ok := recursivePredecessorIndex(
		from,
		to,
		successorIndex,
	)
	if !ok {
		return nil, b.errorf(
			"cannot map edge from block %d to block %d",
			from.Index,
			to.Index,
		)
	}
	var moves []recursiveMove
	for _, instruction := range to.Instrs {
		phi, ok := instruction.(*ssa.Phi)
		if !ok {
			continue
		}
		source, err := b.operand(phi.Edges[predIndex])
		if err != nil {
			return nil, err
		}
		moves = append(moves, recursiveMove{
			dest:   b.slotFor[phi],
			source: source,
		})
	}
	return moves, nil
}

// recursivePredecessorIndex disambiguates duplicate CFG edges for phi lookup.
func recursivePredecessorIndex(
	from *ssa.BasicBlock,
	to *ssa.BasicBlock,
	successorIndex int,
) (int, bool) {
	occurrence := 0
	for _, successor := range from.Succs[:successorIndex] {
		if successor == to {
			occurrence++
		}
	}
	seen := 0
	for index, predecessor := range to.Preds {
		if predecessor != from {
			continue
		}
		if seen == occurrence {
			return index, true
		}
		seen++
	}
	return 0, false
}

// operand encodes a validated SSA value as either a constant or frame slot.
func (b *recursiveProgramBuilder) operand(
	value ssa.Value,
) (recursiveOperand, error) {
	if value == nil {
		return recursiveOperand{}, b.errorf("nil operand is unsupported")
	}
	_, ok := recursiveOperandKind(value)
	if !ok {
		return recursiveOperand{}, b.errorf(
			"operand %s has type %s; want int or bool",
			value.Name(),
			value.Type(),
		)
	}
	if constantValue, isConstant := value.(*ssa.Const); isConstant {
		encoded, err := constInt(constantValue)
		if err != nil {
			return recursiveOperand{}, b.errorf(
				"constant %s is unsupported: %v",
				constantValue,
				err,
			)
		}
		return recursiveOperand{
			constant:   encoded,
			isConstant: true,
		}, nil
	}
	slot, ok := b.slotFor[value]
	if !ok {
		return recursiveOperand{}, b.errorf(
			"value %s has no recursive slot",
			value.Name(),
		)
	}
	return recursiveOperand{slot: slot}, nil
}

// errorf keeps recursive-body rejections identifiable and source-specific.
func (b *recursiveProgramBuilder) errorf(
	format string,
	args ...any,
) error {
	return fmt.Errorf(
		"select: unsupported recursive body %s: %s",
		b.fn.Name(),
		fmt.Sprintf(format, args...),
	)
}
