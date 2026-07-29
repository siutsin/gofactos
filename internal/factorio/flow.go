// This file identifies SSA control flow that can execute at runtime.
package factorio

import (
	"go/constant"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// retainedPhiEdge couples a feasible incoming value to its predecessor block.
type retainedPhiEdge struct {
	predecessor *ssa.BasicBlock
	value       ssa.Value
}

// flowFrame tracks one iterative depth-first traversal step.
type flowFrame struct {
	block      *ssa.BasicBlock
	successors []*ssa.BasicBlock
	next       int
}

// feasibleBlocks returns reverse-postorder blocks reachable after resolving
// Boolean constant branches. For acyclic functions, the order keeps producers
// before their feasible consumers. x/tools/ssa retains both raw edges for
// `if true` and `if false`.
func feasibleBlocks(fn *ssa.Function) []*ssa.BasicBlock {
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}
	entry := fn.Blocks[0]
	seen := map[*ssa.BasicBlock]bool{entry: true}
	stack := []flowFrame{{block: entry, successors: feasibleSuccessors(entry)}}
	postorder := make([]*ssa.BasicBlock, 0, len(fn.Blocks))
	for len(stack) > 0 {
		frame := &stack[len(stack)-1]
		if frame.next < len(frame.successors) {
			successor := frame.successors[frame.next]
			frame.next++
			if successor == nil || seen[successor] {
				continue
			}
			seen[successor] = true
			stack = append(stack, flowFrame{
				block:      successor,
				successors: feasibleSuccessors(successor),
			})
			continue
		}
		postorder = append(postorder, frame.block)
		stack = stack[:len(stack)-1]
	}
	slices.Reverse(postorder)
	return postorder
}

// feasibleBlockSet provides membership checks for semantic CFG traversal.
func feasibleBlockSet(fn *ssa.Function) map[*ssa.BasicBlock]bool {
	blocks := feasibleBlocks(fn)
	set := make(map[*ssa.BasicBlock]bool, len(blocks))
	for _, block := range blocks {
		set[block] = true
	}
	return set
}

// feasibleSuccessors returns both runtime branch arms or the sole arm selected
// by a Boolean SSA constant.
func feasibleSuccessors(block *ssa.BasicBlock) []*ssa.BasicBlock {
	if block == nil {
		return nil
	}
	ifi, ok := lastIf(block)
	if !ok {
		return block.Succs
	}
	index, constant := constantBranchIndex(ifi)
	if !constant {
		return block.Succs
	}
	return block.Succs[index : index+1]
}

// feasibleReachableBefore reports whether target is reachable from start
// without traversing stop.
func feasibleReachableBefore(
	start, target, stop *ssa.BasicBlock,
) bool {
	if start == nil || target == nil || start == stop {
		return false
	}
	seen := make(map[*ssa.BasicBlock]bool)
	pending := []*ssa.BasicBlock{start}
	for len(pending) > 0 {
		last := len(pending) - 1
		block := pending[last]
		pending = pending[:last]
		if block == nil || block == stop || seen[block] {
			continue
		}
		if block == target {
			return true
		}
		seen[block] = true
		pending = append(pending, feasibleSuccessors(block)...)
	}
	return false
}

// feasiblePredecessors excludes incoming CFG edges that a Boolean constant
// makes impossible.
func feasiblePredecessors(block *ssa.BasicBlock) []*ssa.BasicBlock {
	if block == nil || block.Parent() == nil {
		return nil
	}
	reachable := feasibleBlockSet(block.Parent())
	return feasiblePredecessorsIn(block, reachable)
}

// feasiblePredecessorsIn filters incoming edges against a known live-block set.
func feasiblePredecessorsIn(
	block *ssa.BasicBlock,
	reachable map[*ssa.BasicBlock]bool,
) []*ssa.BasicBlock {
	predecessors := make([]*ssa.BasicBlock, 0, len(block.Preds))
	for index, predecessor := range block.Preds {
		if reachable[predecessor] && feasiblePredecessorEdge(block, index) {
			predecessors = append(predecessors, predecessor)
		}
	}
	return predecessors
}

// feasibleEntryChain returns the unique live path from the function entry to
// terminal. Extra prefix blocks may contain constant control and one-edge phi
// aliases, but never executable work that loop lowering would discard.
func feasibleEntryChain(
	fn *ssa.Function,
	terminal *ssa.BasicBlock,
) ([]*ssa.BasicBlock, bool) {
	if fn == nil || len(fn.Blocks) == 0 || terminal == nil {
		return nil, false
	}
	entry := fn.Blocks[0]
	chain := []*ssa.BasicBlock{terminal}
	seen := map[*ssa.BasicBlock]bool{terminal: true}
	for chain[len(chain)-1] != entry {
		current := chain[len(chain)-1]
		predecessors := feasiblePredecessors(current)
		if len(predecessors) != 1 {
			return nil, false
		}
		predecessor := predecessors[0]
		if seen[predecessor] {
			return nil, false
		}
		successors := feasibleSuccessors(predecessor)
		if len(successors) != 1 || successors[0] != current {
			return nil, false
		}
		seen[predecessor] = true
		chain = append(chain, predecessor)
	}
	if len(feasiblePredecessors(entry)) != 0 {
		return nil, false
	}
	slices.Reverse(chain)
	for _, block := range chain[:len(chain)-1] {
		if !feasibleControlBlock(block) {
			return nil, false
		}
	}
	return chain, true
}

// feasibleExitChain follows one semantic path from a loop exit to a terminal
// block. The first block may own the loop's result merge; later blocks may
// contain only one-edge aliases and constant control. Callers validate return.
func feasibleExitChain(start *ssa.BasicBlock) ([]*ssa.BasicBlock, bool) {
	if start == nil {
		return nil, false
	}
	chain := []*ssa.BasicBlock{start}
	seen := map[*ssa.BasicBlock]bool{start: true}
	for {
		block := chain[len(chain)-1]
		successors := feasibleSuccessors(block)
		if len(successors) == 0 {
			return chain, true
		}
		if len(successors) != 1 ||
			!feasibleExitControlBlock(block, len(chain) == 1) {
			return nil, false
		}
		next := successors[0]
		if next == nil || seen[next] {
			return nil, false
		}
		predecessors := feasiblePredecessors(next)
		if len(predecessors) != 1 || predecessors[0] != block {
			return nil, false
		}
		seen[next] = true
		chain = append(chain, next)
	}
}

// feasibleExitControlBlock admits the loop's initial merge and later aliases.
func feasibleExitControlBlock(block *ssa.BasicBlock, initial bool) bool {
	transfers := 0
	for _, instruction := range block.Instrs {
		switch value := instruction.(type) {
		case *ssa.DebugRef:
		case *ssa.Phi:
			if !initial && len(feasiblePhiEdges(value)) != 1 {
				return false
			}
		case *ssa.Jump:
			transfers++
		case *ssa.If:
			if _, constant := constantBranchIndex(value); !constant {
				return false
			}
			transfers++
		default:
			return false
		}
	}
	return transfers == 1
}

// feasibleControlBlock accepts metadata, one-edge aliases, and one semantic
// control transfer.
func feasibleControlBlock(block *ssa.BasicBlock) bool {
	transfers := 0
	for _, instruction := range block.Instrs {
		switch value := instruction.(type) {
		case *ssa.DebugRef:
		case *ssa.Phi:
			if len(feasiblePhiEdges(value)) != 1 {
				return false
			}
		case *ssa.Jump:
			transfers++
		case *ssa.If:
			if _, constant := constantBranchIndex(value); !constant {
				return false
			}
			transfers++
		default:
			return false
		}
	}
	return transfers == 1 && len(feasibleSuccessors(block)) == 1
}

// feasibleImmediateDominator returns the closest strict dominator in the
// feasible CFG.
func feasibleImmediateDominator(block *ssa.BasicBlock) *ssa.BasicBlock {
	if block == nil || block.Parent() == nil {
		return nil
	}
	idoms := feasibleImmediateDominators(block.Parent())
	dominator := idoms[block]
	if dominator == block {
		return nil
	}
	return dominator
}

// feasibleDominates reports whether every feasible entry path to block passes
// through dominator.
func feasibleDominates(dominator, block *ssa.BasicBlock) bool {
	if dominator == nil || block == nil ||
		dominator.Parent() != block.Parent() {
		return false
	}
	idoms := feasibleImmediateDominators(block.Parent())
	for current := block; current != nil; {
		if current == dominator {
			return true
		}
		next := idoms[current]
		if next == nil || next == current {
			return false
		}
		current = next
	}
	return false
}

// feasibleImmediateDominators computes immediate dominators in reverse
// postorder using the Cooper-Harvey-Kennedy intersection algorithm.
func feasibleImmediateDominators(
	fn *ssa.Function,
) map[*ssa.BasicBlock]*ssa.BasicBlock {
	blocks := feasibleBlocks(fn)
	idoms := make(map[*ssa.BasicBlock]*ssa.BasicBlock, len(blocks))
	if len(blocks) == 0 {
		return idoms
	}
	order := make(map[*ssa.BasicBlock]int, len(blocks))
	reachable := make(map[*ssa.BasicBlock]bool, len(blocks))
	for index, block := range blocks {
		order[block] = index
		reachable[block] = true
	}
	idoms[blocks[0]] = blocks[0]
	changed := true
	for changed {
		changed = false
		for _, block := range blocks[1:] {
			next := nextFeasibleDominator(block, reachable, idoms, order)
			if next != nil && idoms[block] != next {
				idoms[block] = next
				changed = true
			}
		}
	}
	return idoms
}

// nextFeasibleDominator intersects the already-known predecessor dominators.
func nextFeasibleDominator(
	block *ssa.BasicBlock,
	reachable map[*ssa.BasicBlock]bool,
	idoms map[*ssa.BasicBlock]*ssa.BasicBlock,
	order map[*ssa.BasicBlock]int,
) *ssa.BasicBlock {
	var next *ssa.BasicBlock
	for _, predecessor := range feasiblePredecessorsIn(block, reachable) {
		if idoms[predecessor] == nil {
			continue
		}
		if next == nil {
			next = predecessor
			continue
		}
		next = intersectDominators(next, predecessor, idoms, order)
	}
	return next
}

// intersectDominators finds the common dominator nearest two CFG blocks.
func intersectDominators(
	first, second *ssa.BasicBlock,
	idoms map[*ssa.BasicBlock]*ssa.BasicBlock,
	order map[*ssa.BasicBlock]int,
) *ssa.BasicBlock {
	for first != second {
		for order[first] > order[second] {
			first = idoms[first]
		}
		for order[second] > order[first] {
			second = idoms[second]
		}
	}
	return first
}

// feasiblePredecessorEdge maps a predecessor slot back to the matching
// successor slot, preserving duplicate edges between the same two blocks.
func feasiblePredecessorEdge(block *ssa.BasicBlock, predecessorIndex int) bool {
	successorIndex, ok := predecessorSuccessorIndex(block, predecessorIndex)
	if !ok {
		return false
	}
	predecessor := block.Preds[predecessorIndex]
	constantIndex, constant := constantBranchIndexFromBlock(predecessor)
	return !constant || successorIndex == constantIndex
}

// predecessorSuccessorIndex maps one predecessor occurrence to its matching
// successor occurrence, including duplicate edges between the same blocks.
func predecessorSuccessorIndex(
	block *ssa.BasicBlock,
	predecessorIndex int,
) (int, bool) {
	if block == nil || predecessorIndex < 0 ||
		predecessorIndex >= len(block.Preds) {
		return 0, false
	}
	predecessor := block.Preds[predecessorIndex]
	if predecessor == nil {
		return 0, false
	}
	occurrence := 0
	for _, earlier := range block.Preds[:predecessorIndex] {
		if earlier == predecessor {
			occurrence++
		}
	}
	seen := 0
	for successorIndex, successor := range predecessor.Succs {
		if successor != block {
			continue
		}
		if seen == occurrence {
			return successorIndex, true
		}
		seen++
	}
	return 0, false
}

// constantBranchIndex maps a Boolean SSA constant to its true or false edge.
func constantBranchIndex(ifi *ssa.If) (int, bool) {
	if ifi == nil || ifi.Block() == nil || len(ifi.Block().Succs) != 2 {
		return 0, false
	}
	condition, ok := ifi.Cond.(*ssa.Const)
	if !ok || condition.Value == nil || condition.Value.Kind() != constant.Bool {
		return 0, false
	}
	if constant.BoolVal(condition.Value) {
		return 0, true
	}
	return 1, true
}

// constantBranchIndexFromBlock reports the selected edge of a constant branch.
func constantBranchIndexFromBlock(block *ssa.BasicBlock) (int, bool) {
	ifi, ok := lastIf(block)
	if !ok {
		return 0, false
	}
	return constantBranchIndex(ifi)
}

// feasiblePhiEdges removes incoming values carried only by infeasible CFG
// edges while retaining the original predecessor association.
func feasiblePhiEdges(phi *ssa.Phi) []retainedPhiEdge {
	if phi == nil || phi.Block() == nil ||
		len(phi.Edges) != len(phi.Block().Preds) {
		return nil
	}
	block := phi.Block()
	reachable := feasibleBlockSet(block.Parent())
	edges := make([]retainedPhiEdge, 0, len(phi.Edges))
	for index, predecessor := range block.Preds {
		if !reachable[predecessor] || !feasiblePredecessorEdge(block, index) {
			continue
		}
		edges = append(edges, retainedPhiEdge{
			predecessor: predecessor,
			value:       phi.Edges[index],
		})
	}
	return edges
}

// feasibleValue follows one-edge phi aliases introduced by constant-dead arms.
func feasibleValue(value ssa.Value) ssa.Value {
	seen := make(map[ssa.Value]bool)
	for value != nil && !seen[value] {
		seen[value] = true
		phi, ok := value.(*ssa.Phi)
		if !ok {
			return value
		}
		edges := feasiblePhiEdges(phi)
		if len(edges) != 1 {
			return value
		}
		value = edges[0].value
	}
	return value
}
