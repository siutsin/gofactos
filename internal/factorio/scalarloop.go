// This file recognises supported scalar loops in SSA control flow.
package factorio

import (
	"fmt"
	"go/token"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// scalarLoopResult distinguishes a valid scalar loop, a scalar-family
// validation error, and a recurrence candidate. matched remains true when a
// scalar-family loop is invalid, preventing recurrence fallback.
type scalarLoopResult struct {
	increment int
	matched   bool
}

// scalarLoopCFG names the blocks and feedback shared by supported scalar loops.
type scalarLoopCFG struct {
	blocks    map[*ssa.BasicBlock]bool
	entry     *ssa.BasicBlock
	backedge  *ssa.BasicBlock
	condition *ssa.BasicBlock
	exit      *ssa.BasicBlock
	next      ssa.Value
}

// validateScalarLoopControlFlow admits only the two counted CFGs that fixed-
// count hardware represents: a pre-test loop or Go's rotated integer range.
func validateScalarLoopControlFlow(
	header *ssa.BasicBlock,
	cmp *ssa.BinOp,
	bound ssa.Value,
	counter *ssa.Phi,
) error {
	cfg, ok := analyseScalarLoopCFG(header, cmp, counter)
	if !ok {
		return unsupportedScalarLoopControlFlow()
	}
	if cmp.X == counter && cfg.condition == header && validScalarPretestControlFlow(
		header,
		cfg.entry,
		cfg.backedge,
		cfg.exit,
	) {
		return nil
	}
	if cmp.X == cfg.next && validScalarRangeControlFlow(
		header,
		cfg.entry,
		cfg.backedge,
		cfg.condition,
		cfg.exit,
		bound,
	) {
		return nil
	}
	return unsupportedScalarLoopControlFlow()
}

// analyseScalarLoopCFG extracts the common entry, loop, exit, and feedback.
func analyseScalarLoopCFG(
	header *ssa.BasicBlock,
	cmp *ssa.BinOp,
	counter *ssa.Phi,
) (scalarLoopCFG, bool) {
	if header == nil || cmp == nil || counter == nil ||
		header.Parent() == nil ||
		counter.Block() != header ||
		len(feasiblePredecessors(header)) != 2 {
		return scalarLoopCFG{}, false
	}

	entry, backedge, ok := scalarLoopPredecessors(header)
	if !ok {
		return scalarLoopCFG{}, false
	}
	blocks := naturalLoopBlocks(header)
	condition := cmp.Block()
	if condition == nil || !blocks[condition] {
		return scalarLoopCFG{}, false
	}
	conditionSuccessors := feasibleSuccessors(condition)
	if len(conditionSuccessors) != 2 ||
		!blocks[conditionSuccessors[0]] ||
		blocks[conditionSuccessors[1]] {
		return scalarLoopCFG{}, false
	}
	exit := conditionSuccessors[1]
	if entry == exit ||
		blocks[entry] ||
		blocks[exit] ||
		!blocks[backedge] ||
		!blocks[conditionSuccessors[0]] {
		return scalarLoopCFG{}, false
	}
	if !validScalarLoopBody(header, condition, blocks) {
		return scalarLoopCFG{}, false
	}
	if !scalarLoopCoversFunction(header.Parent(), blocks, entry, exit) {
		return scalarLoopCFG{}, false
	}

	initial, next, ok := loopPhiEdges(counter, header)
	if !ok || !isConstInt(initial, 0) ||
		!isUnitCounterStep(next, counter) {
		return scalarLoopCFG{}, false
	}
	return scalarLoopCFG{
		blocks:    blocks,
		entry:     entry,
		backedge:  backedge,
		condition: condition,
		exit:      exit,
		next:      next,
	}, true
}

// scalarLoopPredecessors separates the unique entry and back edge by dominance.
func scalarLoopPredecessors(
	header *ssa.BasicBlock,
) (entry, backedge *ssa.BasicBlock, ok bool) {
	for _, pred := range feasiblePredecessors(header) {
		if pred == nil {
			return nil, nil, false
		}
		if header.Dominates(pred) {
			if backedge != nil {
				return nil, nil, false
			}
			backedge = pred
			continue
		}
		if entry != nil {
			return nil, nil, false
		}
		entry = pred
	}
	return entry, backedge, entry != nil && backedge != nil
}

// validScalarLoopBody checks the supported feasible iteration path and rejects
// alternate entries into the body.
func validScalarLoopBody(
	header, condition *ssa.BasicBlock,
	blocks map[*ssa.BasicBlock]bool,
) bool {
	for block := range blocks {
		if !validScalarLoopBlock(header, condition, block, blocks) {
			return false
		}
	}
	return true
}

// validScalarLoopBlock keeps one block on the sole feasible iteration path.
func validScalarLoopBlock(
	header, condition, block *ssa.BasicBlock,
	blocks map[*ssa.BasicBlock]bool,
) bool {
	if block == nil || !header.Dominates(block) {
		return false
	}
	if block == header {
		return true
	}
	successors := feasibleSuccessors(block)
	if block == condition {
		if len(successors) != 2 || !blocks[successors[0]] ||
			blocks[successors[1]] {
			return false
		}
	} else if len(successors) != 1 || !blocks[successors[0]] {
		return false
	}
	for _, predecessor := range feasiblePredecessors(block) {
		if predecessor == nil || !blocks[predecessor] {
			return false
		}
	}
	return true
}

// scalarLoopCoversFunction rejects feasible control flow outside the validated
// entry, loop, and exit chains.
func scalarLoopCoversFunction(
	fn *ssa.Function,
	blocks map[*ssa.BasicBlock]bool,
	entry, exit *ssa.BasicBlock,
) bool {
	feasible := feasibleBlocks(fn)
	entryChain, entryOK := feasibleEntryChain(fn, entry)
	exitChain, exitOK := feasibleExitChain(exit)
	if fn == nil || entry == nil || exit == nil ||
		!entryOK || !exitOK ||
		entry == exit || blocks[entry] || blocks[exit] ||
		len(feasible) != len(blocks)+len(entryChain)+len(exitChain) {
		return false
	}
	covered := make(
		map[*ssa.BasicBlock]bool,
		len(entryChain)+len(blocks)+len(exitChain),
	)
	for _, block := range entryChain {
		covered[block] = true
	}
	for block := range blocks {
		covered[block] = true
	}
	for _, block := range exitChain {
		covered[block] = true
	}
	for _, block := range feasible {
		if !covered[block] {
			return false
		}
	}
	return true
}

// validScalarPretestControlFlow recognises a single feasible canonical entry.
func validScalarPretestControlFlow(
	header, entry, backedge, exit *ssa.BasicBlock,
) bool {
	entrySuccessors := feasibleSuccessors(entry)
	exitPredecessors := feasiblePredecessors(exit)
	return backedge != header &&
		header.Succs[0] != header &&
		len(entrySuccessors) == 1 &&
		entrySuccessors[0] == header &&
		len(exitPredecessors) == 1 &&
		exitPredecessors[0] == header
}

// validScalarRangeControlFlow recognises Go's rotated integer-range CFG.
func validScalarRangeControlFlow(
	header, entry, backedge, condition, exit *ssa.BasicBlock,
	bound ssa.Value,
) bool {
	entrySuccessors := feasibleSuccessors(entry)
	conditionSuccessors := feasibleSuccessors(condition)
	exitPredecessors := feasiblePredecessors(exit)
	if backedge != condition ||
		len(entrySuccessors) != 2 ||
		entrySuccessors[0] != header ||
		entrySuccessors[1] != exit ||
		len(conditionSuccessors) != 2 ||
		conditionSuccessors[0] != header ||
		conditionSuccessors[1] != exit ||
		len(exitPredecessors) != 2 ||
		!slices.Contains(exitPredecessors, entry) ||
		!slices.Contains(exitPredecessors, condition) {
		return false
	}
	ifi, ok := lastIf(entry)
	if !ok {
		return false
	}
	entryCmp, ok := ifi.Cond.(*ssa.BinOp)
	return ok &&
		entryCmp.Op == token.LSS &&
		isConstInt(entryCmp.X, 0) &&
		scalarValuesMatch(entryCmp.Y, bound)
}

// unsupportedScalarLoopControlFlow keeps scalar CFG failures consistent.
func unsupportedScalarLoopControlFlow() error {
	return fmt.Errorf("select: unsupported scalar loop control flow")
}

// validateScalarLoopInstructions rejects feasible operations the synthesised
// scalar circuit would otherwise discard, including work outside the loop.
func validateScalarLoopInstructions(fn *ssa.Function) error {
	for _, block := range feasibleBlocks(fn) {
		for _, instruction := range block.Instrs {
			switch instruction := instruction.(type) {
			case *ssa.BinOp:
				if _, ok := binOpMap[instruction.Op]; !ok {
					return fmt.Errorf(
						"select: unsupported operator %s",
						instruction.Op,
					)
				}
			case *ssa.DebugRef,
				*ssa.Phi,
				*ssa.If,
				*ssa.Jump,
				*ssa.Return:
				continue
			default:
				return fmt.Errorf(
					"select: scalar loop instruction %T is unsupported",
					instruction,
				)
			}
		}
	}
	return nil
}

// naturalLoopBlocks finds the header and blocks on paths back to it.
func naturalLoopBlocks(header *ssa.BasicBlock) map[*ssa.BasicBlock]bool {
	blocks := map[*ssa.BasicBlock]bool{header: true}
	var pending []*ssa.BasicBlock
	for _, pred := range feasiblePredecessors(header) {
		if header.Dominates(pred) && pred != header {
			pending = append(pending, pred)
		}
	}
	for len(pending) > 0 {
		last := len(pending) - 1
		block := pending[last]
		pending = pending[:last]
		if blocks[block] {
			continue
		}
		blocks[block] = true
		pending = append(pending, feasiblePredecessors(block)...)
	}
	return blocks
}

// classifyScalarLoopResult preserves the scalar loop family while leaving
// loops with multiple non-counter states for recurrence analysis.
func classifyScalarLoopResult(
	fn *ssa.Function,
	header *ssa.BasicBlock,
	counter *ssa.Phi,
) (scalarLoopResult, error) {
	matched := scalarLoopResult{matched: true}
	ret, err := loopReturnValue(fn)
	if err != nil {
		return matched, err
	}
	if ret == counter {
		matched.increment = 1
		return matched, nil
	}

	result, err := classifyScalarResult(ret, header)
	if err != nil {
		return matched, err
	}
	if result.isZero {
		return matched, nil
	}

	statePhis := scalarStatePhis(header, counter, result.phi)
	if len(statePhis) > 1 {
		return scalarLoopResult{}, nil
	}
	if len(statePhis) != 1 ||
		!scalarReturnMatchesState(
			result.phi,
			statePhis[0],
			header,
		) {
		return matched, unsupportedScalarLoopResult()
	}

	matched.increment, err = scalarLoopIncrement(statePhis[0], header)
	if err != nil {
		return matched, err
	}
	return matched, nil
}

type scalarResult struct {
	phi    *ssa.Phi
	isZero bool
}

// classifyScalarResult recognises the return forms supported by scalar loops.
func classifyScalarResult(
	result ssa.Value,
	header *ssa.BasicBlock,
) (scalarResult, error) {
	if phi, ok := result.(*ssa.Phi); ok {
		if scalarResultIsZero(phi, header) {
			return scalarResult{isZero: true}, nil
		}
		return scalarResult{phi: phi}, nil
	}
	if scalarResultIsZero(result, header) {
		return scalarResult{isZero: true}, nil
	}
	return scalarResult{}, unsupportedScalarLoopResult()
}

// scalarResultIsZero recognises the valid constant-zero loop result.
func scalarResultIsZero(result ssa.Value, header *ssa.BasicBlock) bool {
	return scalarValueIsZero(result, header, make(map[ssa.Value]bool))
}

// scalarValueIsZero proves a constant or every feasible phi input is zero.
func scalarValueIsZero(
	value ssa.Value,
	header *ssa.BasicBlock,
	seen map[ssa.Value]bool,
) bool {
	value = loopInvariantValue(value, header)
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	defer delete(seen, value)
	if c, ok := value.(*ssa.Const); ok {
		integer, err := constInt(c)
		return err == nil && integer == 0
	}
	phi, ok := value.(*ssa.Phi)
	if !ok {
		return false
	}
	edges := feasiblePhiEdges(phi)
	if len(edges) == 0 {
		return false
	}
	for _, edge := range edges {
		if !scalarValueIsZero(edge.value, header, seen) {
			return false
		}
	}
	return true
}

// scalarStatePhis finds clocked accumulator state, excluding the counter and
// stable identity aliases that do not directly form the result.
func scalarStatePhis(
	header *ssa.BasicBlock,
	counter, result *ssa.Phi,
) []*ssa.Phi {
	var phis []*ssa.Phi
	for _, instr := range header.Instrs {
		phi, isPhi := instr.(*ssa.Phi)
		if isPhi && phi != counter &&
			(phi == result || !loopPhiIsIdentity(phi, header)) {
			phis = append(phis, phi)
		}
	}
	return phis
}

// loopPhiIsIdentity reports state whose feasible feedback is the state itself.
func loopPhiIsIdentity(phi *ssa.Phi, header *ssa.BasicBlock) bool {
	_, next, ok := loopPhiEdges(phi, header)
	return ok && next == phi
}

// scalarLoopIncrement validates and extracts a constant accumulator step.
func scalarLoopIncrement(
	state *ssa.Phi,
	header *ssa.BasicBlock,
) (int, error) {
	initial, next, ok := loopPhiEdges(state, header)
	if !ok {
		return 0, unsupportedScalarLoopBody()
	}
	if err := validateScalarLoopInitial(initial); err != nil {
		return 0, err
	}
	if next == state {
		return 0, nil
	}
	update, ok := next.(*ssa.BinOp)
	if !ok || update.Op != token.ADD {
		return 0, unsupportedScalarLoopBody()
	}
	increment := scalarIncrementConstant(update, state)
	if !validScalarIntegerConst(increment) {
		return 0, unsupportedScalarLoopBody()
	}
	value, err := constInt(increment)
	if err != nil {
		return 0, unsupportedScalarLoopBody()
	}
	return value, nil
}

// validateScalarLoopInitial enforces the zero entry assumed by scalar hardware.
func validateScalarLoopInitial(initial ssa.Value) error {
	c, ok := initial.(*ssa.Const)
	if !ok || c.Value == nil || !isInteger(c.Type()) {
		return fmt.Errorf("select: loop accumulator must start at 0")
	}
	value, err := constInt(c)
	if err != nil {
		return fmt.Errorf("select: loop accumulator must start at 0")
	}
	if value != 0 {
		return fmt.Errorf(
			"select: loop accumulator must start at 0, got %d",
			value,
		)
	}
	return nil
}

// scalarIncrementConstant accepts either operand order for state plus constant.
func scalarIncrementConstant(
	update *ssa.BinOp,
	state *ssa.Phi,
) *ssa.Const {
	x := loopInvariantValue(update.X, state.Block())
	y := loopInvariantValue(update.Y, state.Block())
	if x == state {
		increment, ok := y.(*ssa.Const)
		if ok {
			return increment
		}
		return nil
	}
	if y == state {
		increment, ok := x.(*ssa.Const)
		if ok {
			return increment
		}
	}
	return nil
}

// scalarReturnMatchesState ties an exit phi back to the accumulator it exposes.
func scalarReturnMatchesState(
	result, state *ssa.Phi,
	header *ssa.BasicBlock,
) bool {
	if result == state {
		return true
	}
	initial, next, ok := loopPhiEdges(state, header)
	edges := feasiblePhiEdges(result)
	if !ok || len(edges) != 2 {
		return false
	}
	match := scalarReturnMatch{initial: initial, next: next}
	for _, edge := range edges {
		if !match.add(header, edge.predecessor, edge.value) {
			return false
		}
	}
	return match.entrySeen && match.backSeen
}

type scalarReturnMatch struct {
	initial   ssa.Value
	next      ssa.Value
	entrySeen bool
	backSeen  bool
}

// add records one exit-phi edge while rejecting duplicate predecessor roles.
func (m *scalarReturnMatch) add(
	header, pred *ssa.BasicBlock,
	got ssa.Value,
) bool {
	if pred == nil {
		return false
	}
	want := m.initial
	if header.Dominates(pred) {
		if m.backSeen {
			return false
		}
		m.backSeen = true
		want = m.next
	} else {
		if m.entrySeen {
			return false
		}
		m.entrySeen = true
	}
	return scalarValuesMatch(got, want)
}

// scalarValuesMatch unwraps feasible aliases, then matches identical values or
// equal integer constants.
func scalarValuesMatch(a, b ssa.Value) bool {
	a = feasibleValue(a)
	b = feasibleValue(b)
	if a == b {
		return true
	}
	aConst, aOK := a.(*ssa.Const)
	bConst, bOK := b.(*ssa.Const)
	if !aOK || !bOK ||
		!validScalarIntegerConst(aConst) ||
		!validScalarIntegerConst(bConst) {
		return false
	}
	aValue, aErr := constInt(aConst)
	bValue, bErr := constInt(bConst)
	return aErr == nil && bErr == nil && aValue == bValue
}

// validScalarIntegerConst guards constant conversion during loop recognition.
func validScalarIntegerConst(c *ssa.Const) bool {
	return c != nil && c.Value != nil && isInteger(c.Type())
}

// unsupportedScalarLoopBody keeps scalar-loop shape failures consistent.
func unsupportedScalarLoopBody() error {
	return fmt.Errorf(
		"select: unsupported loop body, only " +
			"`result += constant` is supported",
	)
}

// unsupportedScalarLoopResult keeps scalar return-shape failures consistent.
func unsupportedScalarLoopResult() error {
	return fmt.Errorf(
		"select: loop result must be a loop-carried " +
			"accumulator or constant 0",
	)
}

// loopPhiEdges distinguishes loop entry state from back-edge state. The
// initial edge comes from outside the dominated loop; the next edge comes from
// its back-edge block.
func loopPhiEdges(
	phi *ssa.Phi,
	header *ssa.BasicBlock,
) (initial, next ssa.Value, ok bool) {
	if phi == nil || phi.Block() != header {
		return nil, nil, false
	}
	edges := feasiblePhiEdges(phi)
	if len(edges) != 2 {
		return nil, nil, false
	}
	for _, edge := range edges {
		pred := edge.predecessor
		if pred == nil {
			return nil, nil, false
		}
		if header.Dominates(pred) {
			if next != nil {
				return nil, nil, false
			}
			next = edge.value
			continue
		}
		if initial != nil {
			return nil, nil, false
		}
		initial = edge.value
	}
	initial = feasibleValue(initial)
	next = feasibleValue(next)
	return initial, next, initial != nil && next != nil
}

// loopReturnValue finds the sole value the loop hardware must display.
func loopReturnValue(fn *ssa.Function) (ssa.Value, error) {
	var result ssa.Value
	for _, b := range feasibleBlocks(fn) {
		for _, instr := range b.Instrs {
			r, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}
			if len(r.Results) != 1 || result != nil {
				return nil, fmt.Errorf(
					"select: loop must have exactly one single-value return",
				)
			}
			result = feasibleValue(r.Results[0])
		}
	}
	if result == nil {
		return nil, fmt.Errorf(
			"select: loop must have exactly one single-value return",
		)
	}
	return result, nil
}

// loopHeader finds the loop selection may lower by traversing feasible edges
// and applying x/tools' raw-CFG dominance test. It returns nil when no such
// back edge remains feasible.
func loopHeader(fn *ssa.Function) *ssa.BasicBlock {
	for _, b := range feasibleBlocks(fn) {
		for _, succ := range feasibleSuccessors(b) {
			if succ.Dominates(b) {
				return succ
			}
		}
	}
	return nil
}

// loopCondition finds the counted guard in either the natural header or the
// back-edge block used by Go's rotated integer-range CFG.
func loopCondition(header *ssa.BasicBlock) (*ssa.BinOp, bool) {
	if cmp, ok := blockLoopCondition(header); ok {
		return cmp, true
	}
	_, backedge, ok := scalarLoopPredecessors(header)
	if !ok || backedge == header {
		return nil, false
	}
	return blockLoopCondition(backedge)
}

// blockLoopCondition extracts a comparison from one loop control block.
func blockLoopCondition(block *ssa.BasicBlock) (*ssa.BinOp, bool) {
	ifi, ok := lastIf(block)
	if !ok {
		return nil, false
	}
	cmp, ok := ifi.Cond.(*ssa.BinOp)
	return cmp, ok
}

// loopHeaderCount applies raw-CFG dominance to feasible edges, so constant-dead
// loops do not count against Select's one-loop limit.
func loopHeaderCount(fn *ssa.Function) int {
	seen := map[*ssa.BasicBlock]bool{}
	for _, b := range feasibleBlocks(fn) {
		for _, succ := range feasibleSuccessors(b) {
			if succ.Dominates(b) {
				seen[succ] = true
			}
		}
	}
	return len(seen)
}

// hasControlFlowCycle reports whether a function's feasible CFG is cyclic.
func hasControlFlowCycle(fn *ssa.Function) bool {
	if fn == nil || len(fn.Blocks) == 0 {
		return false
	}
	visited := make(map[*ssa.BasicBlock]bool, len(fn.Blocks))
	active := make(map[*ssa.BasicBlock]bool, len(fn.Blocks))
	var visit func(*ssa.BasicBlock) bool
	visit = func(block *ssa.BasicBlock) bool {
		if active[block] {
			return true
		}
		if visited[block] {
			return false
		}
		visited[block] = true
		active[block] = true
		if slices.ContainsFunc(feasibleSuccessors(block), visit) {
			return true
		}
		active[block] = false
		return false
	}

	return visit(fn.Blocks[0])
}

// loopBound separates the stable limit from the value produced inside the loop.
func loopBound(cmp *ssa.BinOp, header *ssa.BasicBlock) (ssa.Value, bool) {
	x := loopInvariantValue(cmp.X, header)
	y := loopInvariantValue(cmp.Y, header)
	xIn := definedInLoop(x, header)
	yIn := definedInLoop(y, header)
	if xIn && !yIn {
		return y, true
	}
	if yIn && !xIn {
		return x, true
	}
	return nil, false
}

// loopInvariantValue unwraps one-edge aliases, then header state whose feasible
// feedback is itself.
func loopInvariantValue(value ssa.Value, header *ssa.BasicBlock) ssa.Value {
	seen := make(map[ssa.Value]bool)
	for value != nil && !seen[value] {
		value = feasibleValue(value)
		seen[value] = true
		phi, ok := value.(*ssa.Phi)
		if !ok || phi.Block() != header {
			return value
		}
		initial, next, ok := loopPhiEdges(phi, header)
		if !ok || next != phi {
			return value
		}
		value = initial
	}
	return value
}

// validateLoopBound rejects negative constants outside the supported model.
func validateLoopBound(bound ssa.Value) error {
	c, ok := bound.(*ssa.Const)
	if !ok {
		return nil
	}
	n, err := constInt(c)
	if err != nil {
		return err
	}
	if n < 0 {
		return fmt.Errorf(
			"select: loop bound must be non-negative, got %d",
			n,
		)
	}
	return nil
}

// validateScalarLoopBound limits iteration counts to stable selectable inputs.
func validateScalarLoopBound(bound ssa.Value) error {
	if isInteger(bound.Type()) {
		switch bound.(type) {
		case *ssa.Parameter, *ssa.Const:
			return nil
		}
	}
	return fmt.Errorf(
		"select: scalar loop bound must be an integer parameter or constant",
	)
}

// validateLoopShape prevents unsupported loops from being silently miscompiled.
// The counter
// must start at 0, step by 1, and the comparator must be a strict <. The stop
// gate (i < bound) relies on this shape. gofactos lowers only it (forI,
// forRange); other shapes would silently miscompile, so they fail here with a
// clear error.
func validateLoopShape(
	cmp *ssa.BinOp,
	header *ssa.BasicBlock,
) (*ssa.Phi, error) {
	if cmp.Op != token.LSS {
		return nil, fmt.Errorf(
			"select: unsupported loop comparator %s, only < is supported",
			cmp.Op,
		)
	}
	x := loopInvariantValue(cmp.X, header)
	y := loopInvariantValue(cmp.Y, header)
	if !definedInLoop(x, header) || definedInLoop(y, header) {
		return nil, fmt.Errorf(
			"select: loop condition must be counter < bound",
		)
	}
	phi := loopCounterPhi(x)
	if phi == nil {
		return nil, fmt.Errorf("select: unsupported loop counter shape")
	}
	if err := validateCounterStart(phi); err != nil {
		return nil, err
	}
	return phi, nil
}

// validateCounterStart centralises the induction rule shared by loop models. It
// rejects phis that do not initialise to 0 and
// step by 1. Both the scalar loop shape check and the recurrence analyser
// enforce this rule, so it lives in one place with one message.
func validateCounterStart(counter *ssa.Phi) error {
	if !loopStartsAtZeroStepsByOne(counter) {
		return fmt.Errorf(
			"select: unsupported loop, the counter must " +
				"start at 0 and step by 1",
		)
	}
	return nil
}

// loopCounterPhi finds the induction state behind either supported comparison
// form. The
// counter appears directly (forI: phi < n) or as the incremented value
// (forRange: phi+1 < n), so both forms unwrap to the same phi.
func loopCounterPhi(v ssa.Value) *ssa.Phi {
	v = feasibleValue(v)
	if phi, ok := v.(*ssa.Phi); ok {
		return phi
	}
	if bin, ok := v.(*ssa.BinOp); ok && bin.Op == token.ADD {
		x := feasibleValue(bin.X)
		y := feasibleValue(bin.Y)
		if phi, ok := x.(*ssa.Phi); ok && isConstInt(y, 1) {
			return phi
		}
		if phi, ok := y.(*ssa.Phi); ok && isConstInt(x, 1) {
			return phi
		}
	}
	return nil
}

// loopStartsAtZeroStepsByOne recognises the counter progression required by
// the stop gate.
func loopStartsAtZeroStepsByOne(phi *ssa.Phi) bool {
	edges := feasiblePhiEdges(phi)
	if len(edges) != 2 {
		return false
	}
	var zeroInit, unitStep bool
	for _, edge := range edges {
		if isConstInt(feasibleValue(edge.value), 0) {
			zeroInit = true
			continue
		}
		if isUnitCounterStep(edge.value, phi) {
			unitStep = true
		}
	}
	return zeroInit && unitStep
}

// isUnitCounterStep recognises one addition of 1 to the given counter phi.
func isUnitCounterStep(v ssa.Value, counter *ssa.Phi) bool {
	v = feasibleValue(v)
	bin, ok := v.(*ssa.BinOp)
	if !ok {
		return false
	}
	x := feasibleValue(bin.X)
	y := feasibleValue(bin.Y)
	return bin.Op == token.ADD &&
		((x == counter && isConstInt(y, 1)) ||
			(y == counter && isConstInt(x, 1)))
}

// isConstInt gives loop recognition a safe exact-constant predicate.
func isConstInt(v ssa.Value, want int) bool {
	c, ok := v.(*ssa.Const)
	if !ok {
		return false
	}
	n, err := constInt(c)
	return err == nil && n == want
}

// definedInLoop separates evolving loop values from stable external inputs. It
// reports whether v is produced by an instruction in a block the
// loop header dominates (the counter), rather than a parameter, constant, or
// value defined before the loop (the bound).
func definedInLoop(v ssa.Value, header *ssa.BasicBlock) bool {
	v = feasibleValue(v)
	instr, ok := v.(ssa.Instruction)
	if !ok {
		return false
	}
	b := instr.Block()
	return b != nil && header.Dominates(b)
}
