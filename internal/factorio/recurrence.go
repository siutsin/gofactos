// This file validates canonical counted parallel additive recurrences and
// returns the pure descriptor that buildRecurrenceLoop lowers. Analysis never
// mutates selector or SSA state, so it re-checks facts loopSelect has already
// vetted and remains safe to call with unvetted inputs.
package factorio

import (
	"fmt"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// recurrenceState describes one validated header phi in SSA order.
type recurrenceState struct {
	phi     *ssa.Phi
	initial int
	next    ssa.Value
}

// recurrenceAlias maps loop-invariant header state to its entry value.
type recurrenceAlias struct {
	phi   *ssa.Phi
	value ssa.Value
}

// recurrenceLoop is the pure analysis output consumed by later lowering.
// result is always one of states' phis, so the lowering can index its
// registers by result without a missing-entry case.
type recurrenceLoop struct {
	bound   ssa.Value
	counter *ssa.Phi
	result  *ssa.Phi
	states  []recurrenceState
	aliases []recurrenceAlias
	bodyOps []*ssa.BinOp
}

// analyseRecurrenceLoopWithBudget exposes settling depth to focused validation
// while keeping production analysis tied to the clocked-state budget.
func analyseRecurrenceLoopWithBudget(
	fn *ssa.Function,
	header *ssa.BasicBlock,
	cmp *ssa.BinOp,
	bound ssa.Value,
	counter *ssa.Phi,
	settleBudget int,
) (recurrenceLoop, error) {
	shape, err := analyseRecurrenceShape(
		fn,
		header,
		cmp,
		bound,
		counter,
	)
	if err != nil {
		return recurrenceLoop{}, err
	}
	bodySet := recurrenceBodySet(shape.bodyOps)
	allPhiSet := recurrencePhiSet(shape.phis)
	var aliases []recurrenceAlias
	shape.phis, aliases = partitionRecurrencePhis(
		shape.phis,
		header,
		shape.result,
		counter,
		bodySet,
	)
	if len(shape.phis) < 3 {
		return recurrenceLoop{}, fmt.Errorf(
			"select: recurrence loop needs at least two non-counter states",
		)
	}
	err = validateRecurrenceStateTypes(shape.phis)
	if err != nil {
		return recurrenceLoop{}, err
	}
	err = validateRecurrenceOperations(
		fn,
		shape.bodyOps,
		allPhiSet,
		bodySet,
		settleBudget,
	)
	if err != nil {
		return recurrenceLoop{}, err
	}
	states, err := analyseRecurrenceStates(
		shape.phis,
		header,
		allPhiSet,
		bodySet,
	)
	if err != nil {
		return recurrenceLoop{}, err
	}
	err = validateRecurrenceUses(
		states,
		shape.bodyOps,
		shape.result,
		counter,
		allPhiSet,
		bodySet,
	)
	if err != nil {
		return recurrenceLoop{}, err
	}

	return recurrenceLoop{
		bound:   bound,
		counter: counter,
		result:  shape.result,
		states:  states,
		aliases: aliases,
		bodyOps: shape.bodyOps,
	}, nil
}

type recurrenceShape struct {
	phis    []*ssa.Phi
	bodyOps []*ssa.BinOp
	result  *ssa.Phi
}

// analyseRecurrenceShape proves the CFG and SSA skeleton expected by lowering.
func analyseRecurrenceShape(
	fn *ssa.Function,
	header *ssa.BasicBlock,
	cmp *ssa.BinOp,
	bound ssa.Value,
	counter *ssa.Phi,
) (recurrenceShape, error) {
	if fn == nil || header == nil {
		return recurrenceShape{}, fmt.Errorf(
			"select: recurrence loop has unsupported control flow",
		)
	}
	if ifi, ok := lastIf(header); ok && ifi.Cond != cmp {
		return recurrenceShape{}, fmt.Errorf(
			"select: recurrence header if uses an ambiguous condition",
		)
	}
	blocks, err := analyseRecurrenceBlocks(fn, header)
	if err != nil {
		return recurrenceShape{}, err
	}
	err = validateRecurrencePrefix(blocks.prefix)
	if err != nil {
		return recurrenceShape{}, err
	}
	phis, err := validateRecurrenceHeader(
		header,
		cmp,
		bound,
		counter,
	)
	if err != nil {
		return recurrenceShape{}, err
	}
	bodyOps, err := validateRecurrenceBody(blocks.body)
	if err != nil {
		return recurrenceShape{}, err
	}
	err = validateRecurrenceBodyTypes(bodyOps)
	if err != nil {
		return recurrenceShape{}, err
	}
	result, err := validateRecurrenceExit(
		blocks.exit[len(blocks.exit)-1],
		header,
		counter,
	)
	if err != nil {
		return recurrenceShape{}, err
	}
	err = validateNoRecurrencePhisOutside(fn, header)
	if err != nil {
		return recurrenceShape{}, err
	}
	return recurrenceShape{
		phis:    phis,
		bodyOps: bodyOps,
		result:  result,
	}, nil
}

// validateRecurrenceStateTypes keeps loop registers within integer signals.
func validateRecurrenceStateTypes(phis []*ssa.Phi) error {
	for _, phi := range phis {
		if !types.Identical(phi.Type(), types.Typ[types.Int]) {
			return fmt.Errorf(
				"select: recurrence state type %s is unsupported; "+
					"only int is supported",
				phi.Type(),
			)
		}
	}
	return nil
}

// validateRecurrenceBodyTypes keeps intermediate additions within integer
// wires.
func validateRecurrenceBodyTypes(bodyOps []*ssa.BinOp) error {
	for _, op := range bodyOps {
		if !types.Identical(op.Type(), types.Typ[types.Int]) {
			return fmt.Errorf(
				"select: recurrence addition type %s is unsupported; "+
					"only int is supported",
				op.Type(),
			)
		}
	}
	return nil
}

// recurrencePhiSet gives dependency checks an explicit header-state boundary.
func recurrencePhiSet(phis []*ssa.Phi) map[*ssa.Phi]bool {
	set := make(map[*ssa.Phi]bool, len(phis))
	for _, phi := range phis {
		set[phi] = true
	}
	return set
}

// recurrenceBodySet gives dependency checks an explicit body-operation
// boundary.
func recurrenceBodySet(ops []*ssa.BinOp) map[*ssa.BinOp]bool {
	set := make(map[*ssa.BinOp]bool, len(ops))
	for _, op := range ops {
		set[op] = true
	}
	return set
}

// partitionRecurrencePhis separates stable aliases from clocked state before
// type and initial-value validation. Aliases unrelated to the result need no
// lowering.
func partitionRecurrencePhis(
	phis []*ssa.Phi,
	header *ssa.BasicBlock,
	result, counter *ssa.Phi,
	bodyOps map[*ssa.BinOp]bool,
) ([]*ssa.Phi, []recurrenceAlias) {
	reachable := recurrenceReachablePhis(phis, header, result, bodyOps)
	retained := phis[:0]
	var aliases []recurrenceAlias
	for _, phi := range phis {
		if phi == counter || phi == result ||
			!loopPhiIsIdentity(phi, header) {
			retained = append(retained, phi)
			continue
		}
		if reachable[phi] {
			initial, _, ok := loopPhiEdges(phi, header)
			if ok {
				aliases = append(aliases, recurrenceAlias{
					phi:   phi,
					value: loopInvariantValue(initial, header),
				})
			}
		}
	}
	return retained, aliases
}

// recurrenceReachablePhis traces header state backward from the result.
func recurrenceReachablePhis(
	phis []*ssa.Phi,
	header *ssa.BasicBlock,
	result *ssa.Phi,
	bodyOps map[*ssa.BinOp]bool,
) map[*ssa.Phi]bool {
	phiSet := recurrencePhiSet(phis)
	reachable := make(map[*ssa.Phi]bool, len(phis))
	seen := make(map[ssa.Value]bool)
	var visitValue func(ssa.Value)
	var visitState func(*ssa.Phi)
	visitValue = func(value ssa.Value) {
		value = feasibleValue(value)
		if value == nil || seen[value] {
			return
		}
		seen[value] = true
		switch value := value.(type) {
		case *ssa.Phi:
			visitState(value)
		case *ssa.BinOp:
			if bodyOps[value] {
				visitValue(value.X)
				visitValue(value.Y)
			}
		}
	}
	visitState = func(phi *ssa.Phi) {
		if !phiSet[phi] || reachable[phi] {
			return
		}
		reachable[phi] = true
		_, next, ok := loopPhiEdges(phi, header)
		if ok {
			visitValue(next)
		}
	}
	visitState(result)
	return reachable
}

// validateRecurrenceOperations protects combinational lowering from bad inputs,
// dependency order, and paths that cannot settle within one state update.
func validateRecurrenceOperations(
	fn *ssa.Function,
	bodyOps []*ssa.BinOp,
	phiSet map[*ssa.Phi]bool,
	bodySet map[*ssa.BinOp]bool,
	settleBudget int,
) error {
	if err := validateRecurrenceBodyOperands(
		fn,
		bodyOps,
		phiSet,
		bodySet,
	); err != nil {
		return err
	}
	if err := validateRecurrenceBodyDependencies(bodyOps, bodySet); err != nil {
		return err
	}
	return validateRecurrenceBodyDepth(bodyOps, settleBudget)
}

// analyseRecurrenceStates preserves SSA phi order in the lowering descriptor.
func analyseRecurrenceStates(
	phis []*ssa.Phi,
	header *ssa.BasicBlock,
	phiSet map[*ssa.Phi]bool,
	bodySet map[*ssa.BinOp]bool,
) ([]recurrenceState, error) {
	states := make([]recurrenceState, 0, len(phis))
	for _, phi := range phis {
		state, err := analyseRecurrenceState(
			phi,
			header,
			phiSet,
			bodySet,
		)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

// analyseRecurrenceState extracts one register's initial and feedback values.
func analyseRecurrenceState(
	phi *ssa.Phi,
	header *ssa.BasicBlock,
	phiSet map[*ssa.Phi]bool,
	bodySet map[*ssa.BinOp]bool,
) (recurrenceState, error) {
	initial, next, ok := loopPhiEdges(phi, header)
	if !ok {
		return recurrenceState{}, fmt.Errorf(
			"select: recurrence phi has %d edges for %d predecessors",
			len(phi.Edges),
			len(feasiblePredecessors(header)),
		)
	}
	c, ok := initial.(*ssa.Const)
	if !ok || !isInteger(c.Type()) {
		return recurrenceState{}, fmt.Errorf(
			"select: recurrence phi initial value must be " +
				"an integer constant",
		)
	}
	initialValue, err := constInt(c)
	if err != nil {
		return recurrenceState{}, fmt.Errorf(
			"select: recurrence phi initial value must be "+
				"an integer constant: %w",
			err,
		)
	}
	if !recurrenceNextAllowed(next, phiSet, bodySet) {
		return recurrenceState{}, fmt.Errorf(
			"select: recurrence phi next value %T is unsupported",
			next,
		)
	}
	return recurrenceState{
		phi:     phi,
		initial: initialValue,
		next:    next,
	}, nil
}

// validateRecurrenceUses rejects dead work and state unrelated to the result.
func validateRecurrenceUses(
	states []recurrenceState,
	bodyOps []*ssa.BinOp,
	result, counter *ssa.Phi,
	phiSet map[*ssa.Phi]bool,
	bodySet map[*ssa.BinOp]bool,
) error {
	if err := validateRecurrenceBodyUse(
		states,
		bodyOps,
		bodySet,
	); err != nil {
		return err
	}
	return validateRecurrenceStateUse(
		states,
		result,
		counter,
		phiSet,
		bodySet,
	)
}

type recurrenceBlocks struct {
	prefix    []*ssa.BasicBlock
	preheader *ssa.BasicBlock
	body      []*ssa.BasicBlock
	exit      []*ssa.BasicBlock
}

// analyseRecurrenceBlocks names the feasible entry, body, and exit paths.
func analyseRecurrenceBlocks(
	fn *ssa.Function,
	header *ssa.BasicBlock,
) (recurrenceBlocks, error) {
	if fn == nil || header == nil {
		return recurrenceBlocks{}, fmt.Errorf(
			"select: recurrence loop has unsupported control flow",
		)
	}
	preheader, backedge, err := recurrenceHeaderPredecessors(header)
	if err != nil {
		return recurrenceBlocks{}, err
	}
	if len(feasiblePredecessors(header)) != 2 ||
		len(feasibleSuccessors(header)) != 2 {
		return recurrenceBlocks{}, fmt.Errorf(
			"select: recurrence header must have two " +
				"predecessors and two successors",
		)
	}
	body, ok := recurrenceBodyBlocks(header)
	if !ok {
		return recurrenceBlocks{}, fmt.Errorf(
			"select: recurrence loop has unsupported control flow",
		)
	}
	exit, ok := feasibleExitChain(feasibleSuccessors(header)[1])
	if !ok {
		return recurrenceBlocks{}, fmt.Errorf(
			"select: recurrence loop has unsupported control flow",
		)
	}
	prefix, ok := feasibleEntryChain(fn, preheader)
	if !ok || len(feasibleBlocks(fn)) !=
		len(prefix)+len(body)+len(exit)+1 {
		return recurrenceBlocks{}, fmt.Errorf(
			"select: recurrence loop has unsupported control flow",
		)
	}
	blocks := recurrenceBlocks{
		prefix:    prefix,
		preheader: preheader,
		body:      body,
		exit:      exit,
	}
	err = validateRecurrenceBlockShape(
		blocks,
		header,
		backedge,
	)
	if err != nil {
		return recurrenceBlocks{}, err
	}
	return blocks, nil
}

// recurrenceHeaderPredecessors distinguishes entry from the dominated back
// edge.
func recurrenceHeaderPredecessors(
	header *ssa.BasicBlock,
) (*ssa.BasicBlock, *ssa.BasicBlock, error) {
	var preheader, backedge *ssa.BasicBlock
	reachable := feasibleBlockSet(header.Parent())
	for index, pred := range header.Preds {
		if err := validateRecurrenceHeaderEdge(header, pred, index); err != nil {
			return nil, nil, err
		}
		if !reachable[pred] || !feasiblePredecessorEdge(header, index) {
			continue
		}
		if header.Dominates(pred) {
			if backedge != nil {
				return nil, nil, fmt.Errorf(
					"select: recurrence header has ambiguous back edges",
				)
			}
			backedge = pred
			continue
		}
		if preheader != nil {
			return nil, nil, fmt.Errorf(
				"select: recurrence header has ambiguous entry edges",
			)
		}
		preheader = pred
	}
	return preheader, backedge, nil
}

// validateRecurrenceHeaderEdge preserves precise malformed-CFG diagnostics.
func validateRecurrenceHeaderEdge(
	header, predecessor *ssa.BasicBlock,
	index int,
) error {
	if _, ok := predecessorSuccessorIndex(header, index); ok {
		return nil
	}
	if predecessor != nil && header.Dominates(predecessor) {
		return fmt.Errorf(
			"select: recurrence header has ambiguous back edges",
		)
	}
	return fmt.Errorf(
		"select: recurrence header has ambiguous entry edges",
	)
}

// validateRecurrenceBlockShape excludes extra control flow the circuit lacks.
func validateRecurrenceBlockShape(
	blocks recurrenceBlocks,
	header, backedge *ssa.BasicBlock,
) error {
	if blocks.preheader == nil ||
		backedge == nil ||
		len(blocks.body) == 0 ||
		backedge != blocks.body[len(blocks.body)-1] {
		return fmt.Errorf(
			"select: recurrence loop has an unsupported entry or back edge",
		)
	}
	if !recurrenceBlocksDistinct(blocks, header) {
		return fmt.Errorf(
			"select: recurrence loop blocks must be distinct",
		)
	}
	if !validRecurrencePreheader(blocks.preheader, header) {
		return fmt.Errorf(
			"select: recurrence preheader must jump only to the header",
		)
	}
	if !validRecurrenceBody(blocks.body, header) {
		return fmt.Errorf(
			"select: recurrence body must be one feasible back-edge path",
		)
	}
	if len(blocks.exit) == 0 ||
		!validRecurrenceExit(blocks.exit[0], header) {
		return fmt.Errorf(
			"select: recurrence exit must follow only the header",
		)
	}
	return nil
}

// recurrenceBlocksDistinct prevents one CFG block from assuming two loop roles.
func recurrenceBlocksDistinct(
	blocks recurrenceBlocks,
	header *ssa.BasicBlock,
) bool {
	if blocks.preheader == header {
		return false
	}
	seen := map[*ssa.BasicBlock]bool{
		header:           true,
		blocks.preheader: true,
	}
	for _, block := range blocks.body {
		if seen[block] {
			return false
		}
		seen[block] = true
	}
	for _, block := range blocks.exit {
		if seen[block] {
			return false
		}
		seen[block] = true
	}
	return true
}

// validRecurrencePreheader recognises the final edge of the feasible entry path.
func validRecurrencePreheader(
	preheader, header *ssa.BasicBlock,
) bool {
	successors := feasibleSuccessors(preheader)
	return len(successors) == 1 && successors[0] == header
}

// validRecurrenceBody recognises one feasible path from header back to header.
func validRecurrenceBody(body []*ssa.BasicBlock, header *ssa.BasicBlock) bool {
	if len(body) == 0 {
		return false
	}
	previous := header
	for index, block := range body {
		predecessors := feasiblePredecessors(block)
		if len(predecessors) != 1 || predecessors[0] != previous {
			return false
		}
		successors := feasibleSuccessors(block)
		if len(successors) != 1 {
			return false
		}
		next := header
		if index+1 < len(body) {
			next = body[index+1]
		}
		if successors[0] != next {
			return false
		}
		previous = block
	}
	return feasibleSuccessors(header)[0] == body[0]
}

// recurrenceBodyBlocks follows the sole feasible loop-body path back to header.
func recurrenceBodyBlocks(header *ssa.BasicBlock) ([]*ssa.BasicBlock, bool) {
	if header == nil || len(feasibleSuccessors(header)) != 2 {
		return nil, false
	}
	var blocks []*ssa.BasicBlock
	seen := make(map[*ssa.BasicBlock]bool)
	for block := feasibleSuccessors(header)[0]; block != header; {
		if block == nil || seen[block] {
			return nil, false
		}
		seen[block] = true
		blocks = append(blocks, block)
		successors := feasibleSuccessors(block)
		if len(successors) != 1 {
			return nil, false
		}
		block = successors[0]
	}
	return blocks, len(blocks) > 0
}

// validRecurrenceExit recognises the first feasible exit block after the
// header; feasibleExitChain validates the remaining path to the terminal.
func validRecurrenceExit(exit, header *ssa.BasicBlock) bool {
	predecessors := feasiblePredecessors(exit)
	return len(predecessors) == 1 &&
		predecessors[0] == header
}

// validateRecurrencePrefix excludes work before the first state snapshot.
func validateRecurrencePrefix(prefix []*ssa.BasicBlock) error {
	for _, block := range prefix {
		if err := validateRecurrencePrefixBlock(block); err != nil {
			return err
		}
	}
	return nil
}

// validateRecurrencePrefixBlock admits control and one-edge phi aliases only.
func validateRecurrencePrefixBlock(block *ssa.BasicBlock) error {
	transfers := 0
	for _, instruction := range block.Instrs {
		switch value := instruction.(type) {
		case *ssa.DebugRef:
		case *ssa.Jump:
			transfers++
		case *ssa.If:
			if _, constant := constantBranchIndex(value); !constant {
				return unsupportedRecurrencePrefixInstruction(instruction)
			}
			transfers++
		case *ssa.Phi:
			if len(feasiblePhiEdges(value)) != 1 {
				return unsupportedRecurrencePrefixInstruction(instruction)
			}
		default:
			return unsupportedRecurrencePrefixInstruction(instruction)
		}
	}
	if transfers != 1 || len(feasibleSuccessors(block)) != 1 {
		return fmt.Errorf(
			"select: recurrence preheader must contain one control transfer",
		)
	}
	return nil
}

// unsupportedRecurrencePrefixInstruction names rejected entry work uniformly.
func unsupportedRecurrencePrefixInstruction(instruction ssa.Instruction) error {
	return fmt.Errorf(
		"select: recurrence preheader instruction %T is unsupported",
		instruction,
	)
}

// validateRecurrenceHeader binds the counter test to the canonical loop guard.
func validateRecurrenceHeader(
	header *ssa.BasicBlock,
	cmp *ssa.BinOp,
	bound ssa.Value,
	counter *ssa.Phi,
) ([]*ssa.Phi, error) {
	if cmp == nil || counter == nil || cmp.Block() != header {
		return nil, fmt.Errorf(
			"select: recurrence header comparison is malformed",
		)
	}
	if cmp.Op != token.LSS {
		return nil, fmt.Errorf(
			"select: recurrence comparator %s is unsupported",
			cmp.Op,
		)
	}
	if feasibleValue(cmp.X) != counter ||
		loopInvariantValue(cmp.Y, header) != bound {
		return nil, fmt.Errorf(
			"select: recurrence condition must be counter < bound",
		)
	}
	if err := validateRecurrenceBound(bound, header.Parent()); err != nil {
		return nil, err
	}

	info, err := inspectRecurrenceHeader(header, cmp, counter)
	if err != nil {
		return nil, err
	}
	if info.comparisons != 1 || info.branches != 1 {
		return nil, fmt.Errorf(
			"select: recurrence header needs one comparison and one if",
		)
	}
	if !info.foundCounter {
		return nil, fmt.Errorf(
			"select: recurrence counter is not a header phi",
		)
	}
	err = validateCounterStart(counter)
	if err != nil {
		return nil, err
	}
	return info.phis, nil
}

type recurrenceHeaderInfo struct {
	phis         []*ssa.Phi
	comparisons  int
	branches     int
	foundCounter bool
}

// inspectRecurrenceHeader gathers allowed header roles in one validation pass.
func inspectRecurrenceHeader(
	header *ssa.BasicBlock,
	cmp *ssa.BinOp,
	counter *ssa.Phi,
) (recurrenceHeaderInfo, error) {
	var info recurrenceHeaderInfo
	for _, instr := range header.Instrs {
		if err := info.record(instr, cmp, counter); err != nil {
			return recurrenceHeaderInfo{}, err
		}
	}
	return info, nil
}

// record classifies one header instruction for complete shape validation.
func (info *recurrenceHeaderInfo) record(
	instr ssa.Instruction,
	cmp *ssa.BinOp,
	counter *ssa.Phi,
) error {
	switch value := instr.(type) {
	case *ssa.DebugRef:
	case *ssa.Phi:
		info.phis = append(info.phis, value)
		info.foundCounter = info.foundCounter || value == counter
	case *ssa.BinOp:
		if value != cmp {
			return fmt.Errorf(
				"select: recurrence header instruction %T "+
					"is unsupported",
				instr,
			)
		}
		info.comparisons++
	case *ssa.If:
		if value.Cond != cmp {
			return fmt.Errorf(
				"select: recurrence header if uses " +
					"an ambiguous condition",
			)
		}
		info.branches++
	default:
		return fmt.Errorf(
			"select: recurrence header instruction %T "+
				"is unsupported",
			instr,
		)
	}
	return nil
}

// validateRecurrenceBound limits iteration counts to representable stable
// inputs.
func validateRecurrenceBound(
	bound ssa.Value,
	fn *ssa.Function,
) error {
	switch v := bound.(type) {
	case *ssa.Parameter:
		for _, parameter := range fn.Params {
			if v == parameter && isInteger(v.Type()) {
				return nil
			}
		}
	case *ssa.Const:
		if !isInteger(v.Type()) {
			break
		}
		return validateLoopBound(v)
	}
	return fmt.Errorf(
		"select: recurrence bound must be an integer " +
			"parameter or constant",
	)
}

// validateRecurrenceBody admits additions and constant control along the sole
// feasible back-edge path.
func validateRecurrenceBody(body []*ssa.BasicBlock) ([]*ssa.BinOp, error) {
	var ops []*ssa.BinOp
	for _, block := range body {
		blockOps, err := validateRecurrenceBodyBlock(block)
		if err != nil {
			return nil, err
		}
		ops = append(ops, blockOps...)
	}
	return ops, nil
}

// validateRecurrenceBodyBlock validates one block on the feasible body path.
func validateRecurrenceBodyBlock(block *ssa.BasicBlock) ([]*ssa.BinOp, error) {
	var ops []*ssa.BinOp
	transfers := 0
	for _, instruction := range block.Instrs {
		op, transfer, err := classifyRecurrenceBodyInstruction(instruction)
		if err != nil {
			return nil, err
		}
		if op != nil {
			ops = append(ops, op)
		}
		if transfer {
			transfers++
		}
	}
	if transfers != 1 {
		return nil, fmt.Errorf(
			"select: recurrence body block must contain one control transfer",
		)
	}
	return ops, nil
}

// classifyRecurrenceBodyInstruction recognises work and semantic transfers.
func classifyRecurrenceBodyInstruction(
	instruction ssa.Instruction,
) (*ssa.BinOp, bool, error) {
	switch value := instruction.(type) {
	case *ssa.DebugRef:
		return nil, false, nil
	case *ssa.Jump:
		return nil, true, nil
	case *ssa.If:
		if _, constant := constantBranchIndex(value); constant {
			return nil, true, nil
		}
	case *ssa.Phi:
		if len(feasiblePhiEdges(value)) == 1 {
			return nil, false, nil
		}
	case *ssa.BinOp:
		if value.Op != token.ADD {
			return nil, false, fmt.Errorf(
				"select: recurrence body operator %s is unsupported",
				value.Op,
			)
		}
		return value, false, nil
	}
	return nil, false, fmt.Errorf(
		"select: recurrence body instruction %T is unsupported",
		instruction,
	)
}

// validateRecurrenceExit identifies the state register exposed as loop output.
func validateRecurrenceExit(
	exit, header *ssa.BasicBlock,
	counter *ssa.Phi,
) (*ssa.Phi, error) {
	ret, err := recurrenceExitReturn(exit)
	if err != nil {
		return nil, err
	}
	result, ok := feasibleValue(ret.Results[0]).(*ssa.Phi)
	if !ok || result.Block() != header {
		return nil, fmt.Errorf(
			"select: recurrence exit must return a header phi",
		)
	}
	if result == counter {
		return nil, fmt.Errorf(
			"select: recurrence exit must return " +
				"a non-counter header phi",
		)
	}
	if err := validateRecurrenceExitInstructions(exit); err != nil {
		return nil, err
	}
	return result, nil
}

// recurrenceExitReturn finds the feasible exit's sole single-value return.
func recurrenceExitReturn(exit *ssa.BasicBlock) (*ssa.Return, error) {
	var ret *ssa.Return
	for _, instr := range exit.Instrs {
		if candidate, ok := instr.(*ssa.Return); ok {
			if ret != nil {
				return nil, fmt.Errorf(
					"select: recurrence exit has multiple returns",
				)
			}
			ret = candidate
		}
	}
	if ret == nil || len(ret.Results) != 1 {
		return nil, fmt.Errorf(
			"select: recurrence exit must return one value",
		)
	}
	return ret, nil
}

// validateRecurrenceExitInstructions admits the return and one-edge aliases.
func validateRecurrenceExitInstructions(exit *ssa.BasicBlock) error {
	for _, instr := range exit.Instrs {
		switch value := instr.(type) {
		case *ssa.DebugRef, *ssa.Return:
		case *ssa.Phi:
			if len(feasiblePhiEdges(value)) == 1 {
				continue
			}
			return fmt.Errorf(
				"select: recurrence exit instruction %T is unsupported",
				instr,
			)
		default:
			return fmt.Errorf(
				"select: recurrence exit instruction %T is unsupported",
				instr,
			)
		}
	}
	return nil
}

// validateNoRecurrencePhisOutside keeps all simultaneous state in one header.
func validateNoRecurrencePhisOutside(
	fn *ssa.Function,
	header *ssa.BasicBlock,
) error {
	for _, block := range feasibleBlocks(fn) {
		if block == header {
			continue
		}
		for _, instr := range block.Instrs {
			phi, ok := instr.(*ssa.Phi)
			if ok && len(feasiblePhiEdges(phi)) != 1 {
				return fmt.Errorf(
					"select: recurrence phi outside the header " +
						"is unsupported",
				)
			}
		}
	}
	return nil
}

// validateRecurrenceBodyOperands confines additions to stable lowering inputs.
func validateRecurrenceBodyOperands(
	fn *ssa.Function,
	ops []*ssa.BinOp,
	phis map[*ssa.Phi]bool,
	bodyOps map[*ssa.BinOp]bool,
) error {
	params := make(map[*ssa.Parameter]bool, len(fn.Params))
	for _, parameter := range fn.Params {
		params[parameter] = true
	}
	for _, op := range ops {
		for _, operand := range []ssa.Value{op.X, op.Y} {
			if recurrenceOperandAllowed(
				operand,
				phis,
				bodyOps,
				params,
			) {
				continue
			}
			return fmt.Errorf(
				"select: recurrence body operand %T is unsupported",
				operand,
			)
		}
	}
	return nil
}

// recurrenceOperandAllowed defines values safe for combinational recurrence
// use.
func recurrenceOperandAllowed(
	value ssa.Value,
	phis map[*ssa.Phi]bool,
	bodyOps map[*ssa.BinOp]bool,
	params map[*ssa.Parameter]bool,
) bool {
	value = feasibleValue(value)
	switch value := value.(type) {
	case *ssa.Phi:
		return phis[value]
	case *ssa.BinOp:
		return bodyOps[value]
	case *ssa.Parameter:
		return params[value] && isInteger(value.Type())
	case *ssa.Const:
		if !isInteger(value.Type()) {
			return false
		}
		_, err := constInt(value)
		return err == nil
	default:
		return false
	}
}

// validateRecurrenceBodyDependencies requires every body-operation dependency
// to precede its use so combinators can follow a topological order. The
// position check rejects self-dependencies and cycles in malformed SSA too.
func validateRecurrenceBodyDependencies(
	ops []*ssa.BinOp,
	bodyOps map[*ssa.BinOp]bool,
) error {
	positions := make(map[*ssa.BinOp]int, len(ops))
	for i, op := range ops {
		positions[op] = i
	}
	for i, op := range ops {
		for _, operand := range []ssa.Value{op.X, op.Y} {
			operand = feasibleValue(operand)
			dependency, ok := operand.(*ssa.BinOp)
			if !ok || !bodyOps[dependency] {
				continue
			}
			if positions[dependency] >= i {
				return fmt.Errorf(
					"select: recurrence body operation %s "+
						"must precede its use",
					dependency.Name(),
				)
			}
		}
	}
	return nil
}

// validateRecurrenceBodyDepth enforces the clock's combinational settle budget.
func validateRecurrenceBodyDepth(
	ops []*ssa.BinOp,
	settleBudget int,
) error {
	depths := make(map[*ssa.BinOp]int, len(ops))
	for _, op := range ops {
		depth := 1
		for _, operand := range []ssa.Value{op.X, op.Y} {
			operand = feasibleValue(operand)
			dependency, ok := operand.(*ssa.BinOp)
			if !ok {
				continue
			}
			depth = max(depth, depths[dependency]+1)
		}
		if depth > settleBudget {
			return fmt.Errorf(
				"select: recurrence body addition depth %d exceeds "+
					"%d-tick settling budget",
				depth,
				settleBudget,
			)
		}
		depths[op] = depth
	}
	return nil
}

// recurrenceNextAllowed confines phi feedback to validated state and body work.
func recurrenceNextAllowed(
	next ssa.Value,
	phis map[*ssa.Phi]bool,
	bodyOps map[*ssa.BinOp]bool,
) bool {
	next = feasibleValue(next)
	if phi, ok := next.(*ssa.Phi); ok {
		return phis[phi]
	}
	if op, ok := next.(*ssa.BinOp); ok {
		return bodyOps[op]
	}
	return false
}

// validateRecurrenceBodyUse prevents unobservable additions from being emitted.
func validateRecurrenceBodyUse(
	states []recurrenceState,
	ops []*ssa.BinOp,
	bodyOps map[*ssa.BinOp]bool,
) error {
	used := make(map[*ssa.BinOp]bool, len(ops))
	var mark func(ssa.Value)
	mark = func(value ssa.Value) {
		value = feasibleValue(value)
		op, ok := value.(*ssa.BinOp)
		if !ok || !bodyOps[op] || used[op] {
			return
		}
		used[op] = true
		mark(op.X)
		mark(op.Y)
	}
	for _, state := range states {
		mark(state.next)
	}
	for _, op := range ops {
		if !used[op] {
			return fmt.Errorf(
				"select: recurrence body operation %s "+
					"does not contribute to state",
				op.Name(),
			)
		}
	}
	return nil
}

// validateRecurrenceStateUse keeps every non-counter register result-relevant.
func validateRecurrenceStateUse(
	states []recurrenceState,
	result, counter *ssa.Phi,
	phis map[*ssa.Phi]bool,
	bodyOps map[*ssa.BinOp]bool,
) error {
	reachable := recurrenceReachableStates(states, result, phis, bodyOps)
	for _, state := range states {
		if state.phi == counter || reachable[state.phi] {
			continue
		}
		return fmt.Errorf(
			"select: recurrence state %s is unrelated to the result",
			state.phi.Name(),
		)
	}
	return nil
}

// recurrenceReachableStates traces state dependencies backward from the result.
func recurrenceReachableStates(
	states []recurrenceState,
	result *ssa.Phi,
	phis map[*ssa.Phi]bool,
	bodyOps map[*ssa.BinOp]bool,
) map[*ssa.Phi]bool {
	stateByPhi := make(
		map[*ssa.Phi]recurrenceState,
		len(states),
	)
	for _, state := range states {
		stateByPhi[state.phi] = state
	}
	reachable := make(map[*ssa.Phi]bool, len(states))
	var visitValue func(ssa.Value)
	var visitState func(*ssa.Phi)
	visitValue = func(value ssa.Value) {
		value = feasibleValue(value)
		switch value := value.(type) {
		case *ssa.Phi:
			if phis[value] {
				visitState(value)
			}
		case *ssa.BinOp:
			if bodyOps[value] {
				visitValue(value.X)
				visitValue(value.Y)
			}
		}
	}
	visitState = func(phi *ssa.Phi) {
		if reachable[phi] {
			return
		}
		state, ok := stateByPhi[phi]
		if !ok {
			return
		}
		reachable[phi] = true
		visitValue(state.next)
	}
	visitState(result)
	return reachable
}
