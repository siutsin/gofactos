// This file resolves selected SSA returns and branch merges.
package factorio

import (
	"fmt"

	"golang.org/x/tools/go/ssa"
)

// branchInfo is the single runtime result branch: its block, Boolean condition
// value encoded as 1/-1, and then/else successor blocks.
type branchInfo struct {
	block *ssa.BasicBlock
	cond  ssa.Value
	then  *ssa.BasicBlock
	els   *ssa.BasicBlock
}

// retInfo is one return: the block it sits in and the value it returns.
type retInfo struct {
	block *ssa.BasicBlock
	value ssa.Value
}

// ret defers return selection until all feasible blocks have been visited. The
// result net is then resolved, so a two-return branch can become one phi.
func (s *selector) ret(v *ssa.Return) error {
	if len(v.Results) != 1 {
		return fmt.Errorf("select: expected one return value, got %d", len(v.Results))
	}
	s.returns = append(s.returns, retInfo{block: v.Block(), value: v.Results[0]})
	return nil
}

// resolveResult chooses the direct or merged net that represents the function.
// One feasible return uses its value directly, two need a result merge, and
// anything else is unsupported.
func (s *selector) resolveResult(fn *ssa.Function) error {
	switch len(s.returns) {
	case 0:
		return fmt.Errorf("select %s: no return value", fn.Name())
	case 1:
		pp, err := s.portFor(s.returns[0].value)
		if err != nil {
			return err
		}
		s.resultNet = s.netOf(pp)
		return nil
	case 2:
		return s.buildMerge()
	default:
		return fmt.Errorf("select %s: %d return paths unsupported", fn.Name(), len(s.returns))
	}
}

// buildMerge gives a two-return result branch one physical net after proving
// that branch dominates both returns. Each arm may cross later supported blocks
// before its distinct return. The true value feeds phi.a, the false value feeds
// phi.b, and cond selects between them. The wire is the phi node.
func (s *selector) buildMerge() error {
	if s.branch == nil {
		return fmt.Errorf("select: two returns without a branch is unsupported")
	}
	thenReturn, ok := s.uniqueReachableReturn(s.branch.then)
	if !ok {
		return fmt.Errorf("select: branch then arm has no unique return")
	}
	elseReturn, ok := s.uniqueReachableReturn(s.branch.els)
	if !ok {
		return fmt.Errorf("select: branch else arm has no unique return")
	}
	if thenReturn.block == elseReturn.block {
		return fmt.Errorf("select: branch arms do not lead to distinct returns")
	}
	if !feasibleDominates(s.branch.block, thenReturn.block) ||
		!feasibleDominates(s.branch.block, elseReturn.block) {
		return fmt.Errorf("select: result branch must dominate both returns")
	}
	ph := newInstance(&phi{})
	s.add(ph)
	if s.path != "" {
		ph.port("out").ssaName = s.path + ".result"
	}
	if err := s.use(thenReturn.value, ph.port("a")); err != nil {
		return err
	}
	if err := s.use(elseReturn.value, ph.port("b")); err != nil {
		return err
	}
	if err := s.use(s.branch.cond, ph.port("cond")); err != nil {
		return err
	}
	s.resultNet = s.netOf(ph.port("out"))
	return nil
}

// uniqueReachableReturn finds the one return an arm can reach. An arm may pass
// through a value merge before returning, but reaching both returns is
// ambiguous and remains unsupported.
func (s *selector) uniqueReachableReturn(start *ssa.BasicBlock) (retInfo, bool) {
	seen := make(map[*ssa.BasicBlock]bool)
	blocks := []*ssa.BasicBlock{start}
	found := -1
	for len(blocks) > 0 {
		block := blocks[len(blocks)-1]
		blocks = blocks[:len(blocks)-1]
		if seen[block] {
			continue
		}
		seen[block] = true
		for i, result := range s.returns {
			if result.block != block {
				continue
			}
			if found >= 0 && found != i {
				return retInfo{}, false
			}
			found = i
		}
		blocks = append(blocks, feasibleSuccessors(block)...)
	}
	if found < 0 {
		return retInfo{}, false
	}
	return s.returns[found], true
}
