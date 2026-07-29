// This file selects physical netlist modules for supported SSA instructions.
package factorio

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// selected is the Select phase output: the netlist (module instances and
// public nets) plus the net carrying the function's return value.
type selected struct {
	insts       []*instance
	nets        []*netlistNet
	resultNet   *netlistNet
	clockPeriod int
}

// selectFunc is the Select-phase entry that validates and lowers an SSA root.
// It is the instruction-selection analogue: each materialised SSA value maps
// to a module output, and operand use becomes a public net from producer to
// reader.
//
// Supported today: straight-line integer arithmetic, two-arm result branches,
// mid-function phi merges, one counting loop, direct static calls, and bounded
// direct self-recursion over the scalar SSA subset. Mutual recursion, methods,
// closures, more than one loop, and unsupported types return an explicit error.
func selectFunc(fn *ssa.Function, opts ...Option) (*selected, error) {
	if fn.Signature.Recv() != nil {
		return nil, fmt.Errorf("select: methods are unsupported")
	}
	if fn.Parent() != nil {
		return nil, fmt.Errorf("select: closures are unsupported")
	}
	if err := supportedSignature(fn); err != nil {
		return nil, err
	}
	s := &selector{
		selectorFrame: selectorFrame{
			producer: map[ssa.Value]*port{},
		},
		clockPeriod: clockPeriod,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s.selectRoot(fn)
}

// selectRoot combines call planning, body selection, and result decoration.
func (s *selector) selectRoot(fn *ssa.Function) (*selected, error) {
	program, err := planCallProgram(fn)
	if err != nil {
		return nil, err
	}
	recursive := false
	if program != nil {
		s.plan = program.root
		recursive = program.recursive != nil
	}

	if err := s.addParameters(fn); err != nil {
		return nil, err
	}

	if recursive {
		if err := s.selectRecursive(program.recursive); err != nil {
			return nil, err
		}
	} else {
		if err := s.selectProgramBody(fn); err != nil {
			return nil, err
		}
	}
	s.addDigitDisplay(fn)
	s.tagInputs(fn)
	s.pruneDeadInstances()
	return &selected{
		insts:       s.insts,
		nets:        s.nets,
		resultNet:   s.resultNet,
		clockPeriod: s.clockPeriod,
	}, nil
}

// selectProgramBody chooses the validated call plan when the program needs one.
func (s *selector) selectProgramBody(fn *ssa.Function) error {
	if s.plan != nil && s.plan.instructions != nil {
		return s.selectPlannedBody(s.plan)
	}
	return s.selectBody(fn)
}

// selectRecursive represents a validated self-cycle with one bounded machine.
// Its pulse remains unwired until Clock adds the shared clock.
func (s *selector) selectRecursive(program *recursiveProgram) error {
	if program == nil || program.fn == nil {
		return fmt.Errorf(
			"select: recursive program has no function",
		)
	}
	fn := program.fn
	if len(program.params) != len(fn.Params) {
		return fmt.Errorf("select: recursive parameter plan is inconsistent")
	}
	machine := newInstance(newRecursiveMachine(program))
	s.add(machine)
	for index, parameter := range fn.Params {
		producer, ok := s.producer[parameter]
		if !ok || producer == nil {
			return fmt.Errorf(
				"select: recursive parameter %s has no producer",
				parameter.Name(),
			)
		}
		argument := s.netOf(producer)
		input := machine.port(fmt.Sprintf("arg%d", index))
		argument.readers = append(argument.readers, input)
		input.net = argument
	}
	s.resultNet = s.netOf(machine.port("result"))
	return nil
}

// addParameters gives every root parameter an editable in-game source.
func (s *selector) addParameters(fn *ssa.Function) error {
	if err := s.validateParamNames(fn); err != nil {
		return err
	}
	for _, parameter := range fn.Params {
		value, err := s.parameterValue(parameter)
		if err != nil {
			return err
		}
		in := newInstance(newConstSrc(value))
		s.add(in)
		s.producer[parameter] = in.port("out")
	}
	return nil
}

// parameterValue supplies a representable initial signal for one parameter.
// The default
// is 1 so a parameter is never silently absent from a Factorio wire.
func (s *selector) parameterValue(parameter *ssa.Parameter) (int, error) {
	value := 1
	if configured, ok := s.paramValues[parameter.Name()]; ok {
		value = configured
	}
	if isInteger(parameter.Type()) {
		if err := validateFactorioInt(int64(value)); err != nil {
			return 0, fmt.Errorf(
				"select: parameter %s value %d %w",
				parameter.Name(),
				value,
				err,
			)
		}
	}
	if isBool(parameter.Type()) {
		// Booleans use 1/-1 because zero is an absent signal.
		if value == 0 {
			value = -1
		} else {
			value = 1
		}
	}
	return value, nil
}

// validateParamNames rejects --set entries that name no parameter, so a typo
// fails loudly rather than being silently ignored.
func (s *selector) validateParamNames(fn *ssa.Function) error {
	if len(s.paramValues) == 0 {
		return nil
	}
	known := make(map[string]bool, len(fn.Params))
	for _, p := range fn.Params {
		known[p.Name()] = true
	}
	var unknown []string
	for name := range s.paramValues {
		if !known[name] {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("select: --set names unknown parameter(s): %s", strings.Join(unknown, ", "))
	}
	return nil
}

// selectBody chooses the supported loop or acyclic lowering for one function.
func (s *selector) selectBody(fn *ssa.Function) error {
	if header := loopHeader(fn); header != nil {
		if loopHeaderCount(fn) > 1 {
			return fmt.Errorf("select: more than one loop is unsupported")
		}
		return s.loopSelect(header)
	}
	if hasControlFlowCycle(fn) {
		return fmt.Errorf("select: irreducible control-flow cycle is unsupported")
	}
	s.hasPhi = functionHasPhi(fn)
	for _, b := range feasibleBlocks(fn) {
		for _, instr := range b.Instrs {
			if err := s.instr(instr); err != nil {
				return err
			}
		}
	}
	return s.resolveResult(fn)
}

// pruneDeadInstances removes unused SSA modules before netlist verification.
// The SSA builder retains discarded computations such as `_ = a * b`, while
// parameter binding can create a source with no live consumer. Left in, either
// makes Verify reject the floating output. Liveness starts from the result
// driver, teaching display, and every register, then walks backwards through
// inputs. A dead chain is removed whole and each surviving net keeps only its
// live readers, so Allocate counts only live intermediate nets. The display,
// not every result reader, is a root: dead user code may also read the result
// (`r := a + b; _ = r * r; return r`).
func (s *selector) pruneDeadInstances() {
	if s.resultNet == nil {
		return
	}
	live := s.liveInstances()
	s.retainLiveInstances(live)
	s.retainLiveNets(live)
}

// liveInstances traces modules required by the result, display, and registers.
func (s *selector) liveInstances() map[*instance]bool {
	live := map[*instance]bool{}
	var stack []*instance
	stack = markLiveInstance(live, stack, s.resultNet.driver.inst)
	stack = markLiveInstance(live, stack, s.display)
	// Registers remain roots even when unread: in scalar-loop lowering the
	// index register is a visible phi node that does not feed the result.
	for _, in := range s.insts {
		if _, ok := in.comp.(*register); ok {
			stack = markLiveInstance(live, stack, in)
		}
	}
	for len(stack) > 0 {
		in := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, p := range in.ports {
			if p.spec.kind == portIn && p.net != nil {
				stack = markLiveInstance(
					live,
					stack,
					p.net.driver.inst,
				)
			}
		}
	}
	return live
}

// markLiveInstance adds an unseen module to the backwards liveness walk.
func markLiveInstance(
	live map[*instance]bool,
	stack []*instance,
	in *instance,
) []*instance {
	if in == nil || live[in] {
		return stack
	}
	live[in] = true
	return append(stack, in)
}

// retainLiveInstances removes modules outside the computed live set.
func (s *selector) retainLiveInstances(live map[*instance]bool) {
	s.insts = slices.DeleteFunc(s.insts, func(in *instance) bool {
		return !live[in]
	})
}

// retainLiveNets removes connections whose producer or consumer is dead.
func (s *selector) retainLiveNets(live map[*instance]bool) {
	// Drop nets a dead module drives, and prune dead readers from the rest, so
	// Emit lowers no edge into a removed module.
	s.nets = slices.DeleteFunc(s.nets, func(n *netlistNet) bool {
		return !live[n.driver.inst]
	})
	for _, n := range s.nets {
		n.readers = slices.DeleteFunc(n.readers, func(r *port) bool {
			return !live[r.inst]
		})
	}
}

// tagInputs preserves signature order when Allocate chooses input signals.
// It marks each parameter's net as an input carrying its signature
// position, so Allocate maps it to the matching letter (the first parameter to
// signal-A). A parameter that is never used has no net and is skipped.
func (s *selector) tagInputs(fn *ssa.Function) {
	for i, p := range fn.Params {
		pp, ok := s.producer[p]
		if !ok || pp.net == nil {
			continue
		}
		pp.net.isInput = true
		pp.net.inputIndex = i
	}
}

// addDigitDisplay makes a supported return value visible in the blueprint.
// A boolean return
// gets the boolDisplay TRUE/FALSE panel; an integer return gets the multi-digit
// numeric chain. Other result types get no display module. Both terminate in
// display panels and add no public output net.
func (s *selector) addDigitDisplay(fn *ssa.Function) {
	if s.resultNet == nil {
		return
	}
	var dd *instance
	switch {
	case isBoolResult(fn):
		dd = newInstance(&boolDisplay{})
	case isIntResult(fn):
		dd = newInstance(&digitDisplay{})
	default:
		return
	}
	s.add(dd)
	s.display = dd
	in := dd.port("in")
	s.resultNet.readers = append(s.resultNet.readers, in)
	in.net = s.resultNet
}

// supportedSignature rejects parameters and results that are not exact int or
// bool values. Other integer types have overflow and comparison semantics that
// the signed 32-bit Factorio network cannot preserve.
func supportedSignature(fn *ssa.Function) error {
	for _, p := range fn.Params {
		if !isIntOrBool(p.Type()) {
			return fmt.Errorf("select: unsupported parameter type %s", p.Type())
		}
	}
	res := fn.Signature.Results()
	for v := range res.Variables() {
		if !isIntOrBool(v.Type()) {
			return fmt.Errorf("select: unsupported result type %s", v.Type())
		}
	}
	return nil
}

// isIntOrBool reports whether t is exactly the predeclared int or bool type.
func isIntOrBool(t types.Type) bool {
	basic, ok := unaliasedBasic(t)
	return ok && (basic.Kind() == types.Int || basic.Kind() == types.Bool)
}

// unaliasedBasic strips aliases while leaving defined named types intact.
func unaliasedBasic(t types.Type) (*types.Basic, bool) {
	basic, ok := types.Unalias(t).(*types.Basic)
	return basic, ok
}

// isInteger reports whether t is a basic integer type.
func isInteger(t types.Type) bool {
	b, ok := unaliasedBasic(t)
	return ok && b.Info()&types.IsInteger != 0
}

// isBool reports whether t is a basic boolean type, so its parameter source can
// be seeded through the 1/-1 boolean encoding.
func isBool(t types.Type) bool {
	b, ok := unaliasedBasic(t)
	return ok && b.Info()&types.IsBoolean != 0
}

// isBoolResult reports whether fn returns a single boolean.
func isBoolResult(fn *ssa.Function) bool {
	return basicResult(fn, types.IsBoolean)
}

// isIntResult reports whether fn returns a single integer.
func isIntResult(fn *ssa.Function) bool {
	return basicResult(fn, types.IsInteger)
}

// basicResult reports whether fn returns a single basic type in the given info
// class (e.g. boolean or integer).
func basicResult(fn *ssa.Function, info types.BasicInfo) bool {
	res := fn.Signature.Results()
	if res.Len() != 1 {
		return false
	}
	b, ok := unaliasedBasic(res.At(0).Type())
	return ok && b.Info()&info != 0
}

// selector accumulates the netlist while walking feasible SSA blocks. producer
// maps each materialised SSA value to the module output port that drives it.
// branch records the single result split, and returns collects feasible return
// blocks so the merge is built once the function has been walked.
type selector struct {
	insts       []*instance
	nets        []*netlistNet
	display     *instance      // the result's digit display, the live-set sink
	paramValues map[string]int // parameter initial values keyed by name (--set)
	clockPeriod int
	selectorFrame
}

// selectorFrame is the invocation-local selector state saved while a callee is
// expanded into the shared module and net slices.
type selectorFrame struct {
	producer   map[ssa.Value]*port
	branch     *branchInfo
	returns    []retInfo
	hasPhi     bool
	resultNet  *netlistNet
	plan       *invocationPlan
	path       string
	paramAlias map[ssa.Value]string
}

// add records a selected module in stable emission order.
func (s *selector) add(in *instance) { s.insts = append(s.insts, in) }

// produce binds an SSA value to its physical producer and teaching label.
// It stamps v's name
// (t0, t1, ...) on the port, so a panel label can show the dump's identifier.
// netOf copies the name onto the net. Parameters, literals, and the
// synthesised two-return merge set the producer without a name.
func (s *selector) produce(v ssa.Value, pp *port) {
	s.producer[v] = pp
	pp.ssaName = s.qualifiedName(v.Name())
}

// instr dispatches each supported SSA instruction to its module selector.
func (s *selector) instr(instr ssa.Instruction) error {
	switch v := instr.(type) {
	case *ssa.DebugRef, *ssa.Jump:
		return nil // metadata / pure control flow: no module in a dataflow netlist
	case *ssa.BinOp:
		return s.binOp(v)
	case *ssa.UnOp:
		return s.unOp(v)
	case *ssa.Phi:
		return s.phiNode(v)
	case *ssa.If:
		return s.ifInstr(v)
	case *ssa.Return:
		return s.ret(v)
	case *ssa.Call:
		return s.call(v)
	default:
		return fmt.Errorf("select: unsupported instruction %T", instr)
	}
}

// call expands a validated ordinary callee without adding a copy module. Its
// parameters reuse the caller argument producers, and its result producer
// becomes the producer of the caller's call value without a copy module.
func (s *selector) call(v *ssa.Call) error {
	if s.plan == nil {
		return fmt.Errorf("select: call has no validated plan")
	}
	child, ok := s.plan.calls[v]
	if !ok {
		return fmt.Errorf("select: call has no physical invocation plan")
	}
	args := v.Common().Args
	ports := make([]*port, len(args))
	labels := make([]string, len(args))
	for i, argument := range args {
		producer, err := s.portFor(argument)
		if err != nil {
			return err
		}
		ports[i] = producer
		labels[i] = s.valueLabel(argument)
	}
	result, err := s.selectInvocation(child, ports, labels)
	if err != nil {
		return err
	}
	s.producer[v] = result.driver
	return nil
}

// selectInvocation isolates callee-local SSA state while sharing the netlist.
func (s *selector) selectInvocation(
	plan *invocationPlan,
	arguments []*port,
	labels []string,
) (*netlistNet, error) {
	if len(arguments) != len(plan.fn.Params) || len(labels) != len(arguments) {
		return nil, fmt.Errorf("select: invalid call plan for %s", plan.fn.Name())
	}
	saved := s.saveFrame()
	defer s.restoreFrame(saved)

	s.selectorFrame = selectorFrame{
		producer:   make(map[ssa.Value]*port),
		hasPhi:     plan.hasPhi,
		plan:       plan,
		path:       plan.path,
		paramAlias: make(map[ssa.Value]string),
	}
	for i := range len(arguments) {
		parameter := plan.fn.Params[i]
		s.producer[parameter] = arguments[i]
		if labels[i] != "" {
			s.paramAlias[parameter] = labels[i]
		}
	}
	if selectErr := s.selectPlannedBody(plan); selectErr != nil {
		return nil, selectErr
	}
	if s.resultNet == nil {
		return nil, fmt.Errorf("select %s: no result net", plan.fn.Name())
	}
	return s.resultNet, nil
}

// selectPlannedBody lowers only instructions approved by call planning.
func (s *selector) selectPlannedBody(plan *invocationPlan) error {
	s.hasPhi = plan.hasPhi
	for _, instruction := range plan.instructions {
		if err := s.instr(instruction); err != nil {
			return err
		}
	}
	return s.resolveResult(plan.fn)
}

// saveFrame preserves caller-local selection state during callee expansion.
func (s *selector) saveFrame() selectorFrame {
	return s.selectorFrame
}

// restoreFrame resumes caller selection after a callee has been expanded.
func (s *selector) restoreFrame(frame selectorFrame) {
	s.selectorFrame = frame
}

// qualifiedName prevents expanded callees from sharing ambiguous panel names.
func (s *selector) qualifiedName(name string) string {
	if name == "" || s.path == "" {
		return name
	}
	return s.path + "." + name
}

// valueLabel keeps call arguments recognisable across invocation boundaries.
func (s *selector) valueLabel(value ssa.Value) string {
	if alias, ok := s.paramAlias[value]; ok {
		return alias
	}
	if _, parameter := value.(*ssa.Parameter); parameter {
		return ""
	}
	if _, literal := value.(*ssa.Const); literal {
		return ""
	}
	return s.qualifiedName(value.Name())
}

// binOp selects arithmetic or comparison hardware for a binary SSA value.
func (s *selector) binOp(v *ssa.BinOp) error {
	entry, ok := binOpMap[v.Op]
	if !ok {
		return fmt.Errorf("select: unsupported operator %s", v.Op)
	}
	if entry.entityName == deciderCombinatorName {
		return s.compareOp(v, entry)
	}
	in := newInstance(newArith(entry.operation))
	s.add(in)
	if err := s.use(v.X, in.port("a")); err != nil {
		return err
	}
	if err := s.use(v.Y, in.port("b")); err != nil {
		return err
	}
	s.produce(v, in.port("out"))
	return nil
}

// compareOp preserves both true and false as present values on a condition net.
func (s *selector) compareOp(v *ssa.BinOp, entry operationEntry) error {
	// A constant right operand bakes into the decider; otherwise compare two
	// signals, a cmp b, via the variable compare.
	if c, ok := v.Y.(*ssa.Const); ok {
		k, err := constInt(c)
		if err != nil {
			return err
		}
		cmp := newCompare(entry.operation, k)
		// A Boolean constant is encoded as the 1/-1 sentinel, so its label reads
		// as true/false; an integer constant keeps its literal value.
		cmp.boolean = c.Value != nil && c.Value.Kind() == constant.Bool
		in := newInstance(cmp)
		s.add(in)
		if err := s.use(v.X, in.port("a")); err != nil {
			return err
		}
		s.produce(v, in.port("cond"))
		return nil
	}
	in := newInstance(newCompareVar(entry.operation))
	s.add(in)
	if err := s.use(v.X, in.port("a")); err != nil {
		return err
	}
	if err := s.use(v.Y, in.port("b")); err != nil {
		return err
	}
	s.produce(v, in.port("cond"))
	return nil
}

// unOp selects the supported integer-negation module and rejects other unary
// operations.
func (s *selector) unOp(v *ssa.UnOp) error {
	if v.Op != token.SUB {
		return fmt.Errorf("select: unsupported unary operator %s", v.Op)
	}
	in := newInstance(&neg{})
	s.add(in)
	if err := s.use(v.X, in.port("in")); err != nil {
		return err
	}
	s.produce(v, in.port("out"))
	return nil
}

// ifInstr preserves the branch information needed to build a return merge.
// A branch represented by a phi is only a control-flow marker and is skipped;
// an independent return branch is kept even when the function has other phis.
// A literal-constant branch is also skipped because feasible traversal has
// already selected its only possible arm. Succs[0] is the true block and
// Succs[1] the false block.
func (s *selector) ifInstr(v *ssa.If) error {
	b := v.Block()
	if len(b.Succs) != 2 {
		return fmt.Errorf("select: if with %d successors", len(b.Succs))
	}
	if _, constant := constantBranchIndex(v); constant {
		return nil
	}
	if s.hasPhi && branchControlsPhi(b) {
		return nil
	}
	if s.branch != nil {
		return fmt.Errorf("select: more than one branch is unsupported")
	}
	s.branch = &branchInfo{
		block: b,
		cond:  v.Cond,
		then:  b.Succs[0],
		els:   b.Succs[1],
	}
	return nil
}

// branchControlsPhi reports whether b immediately dominates a phi merge in the
// feasible CFG, the relationship phiNode uses to find its controlling branch.
func branchControlsPhi(b *ssa.BasicBlock) bool {
	for _, block := range feasibleBlocks(b.Parent()) {
		if feasibleImmediateDominator(block) != b {
			continue
		}
		for _, instr := range block.Instrs {
			phi, ok := instr.(*ssa.Phi)
			if ok && len(feasiblePhiEdges(phi)) > 1 {
				return true
			}
		}
	}
	return false
}

// phiNode gives a two-input SSA value merge a physical phi module. It finds the
// controlling branch through the phi block's feasible immediate dominator and
// maps each incoming edge to an arm: the condition-true path feeds phi.a, the
// false path feeds phi.b, and cond selects between them. Constant-control
// blocks may lie within an arm, but only one runtime branch controls the merge.
func (s *selector) phiNode(v *ssa.Phi) error {
	b := v.Block()
	if len(v.Edges) != len(b.Preds) {
		return fmt.Errorf("select: phi with %d edges is unsupported", len(v.Edges))
	}
	edges := feasiblePhiEdges(v)
	if len(edges) == 1 {
		return s.aliasPhi(v, edges[0].value)
	}
	if len(edges) != 2 {
		return fmt.Errorf("select: phi with %d feasible edges is unsupported", len(edges))
	}
	idom := feasibleImmediateDominator(b)
	if idom == nil {
		return fmt.Errorf("select: phi has no dominator")
	}
	ifi, ok := lastIf(idom)
	if !ok {
		return fmt.Errorf("select: phi has no controlling if")
	}
	return s.mergePhi(v, ifi, edges)
}

// aliasPhi maps a one-feasible-edge phi directly to its live producer.
func (s *selector) aliasPhi(v *ssa.Phi, value ssa.Value) error {
	producer, err := s.portFor(value)
	if err != nil {
		return err
	}
	s.producer[v] = producer
	return nil
}

// mergePhi builds the physical two-input merge for a runtime branch.
func (s *selector) mergePhi(
	v *ssa.Phi,
	ifi *ssa.If,
	edges []retainedPhiEdge,
) error {
	thenVal, elseVal, ok := phiArmValues(v.Block(), ifi.Block(), edges)
	if !ok {
		return fmt.Errorf("select: could not map phi arms")
	}
	ph := newInstance(&phi{})
	s.add(ph)
	if err := s.use(thenVal, ph.port("a")); err != nil {
		return err
	}
	if err := s.use(elseVal, ph.port("b")); err != nil {
		return err
	}
	if err := s.use(ifi.Cond, ph.port("cond")); err != nil {
		return err
	}
	s.produce(v, ph.port("out"))
	return nil
}

// phiArmValues maps retained predecessors to the controlling branch's arms.
func phiArmValues(
	block, dominator *ssa.BasicBlock,
	edges []retainedPhiEdge,
) (ssa.Value, ssa.Value, bool) {
	if block == nil || dominator == nil || len(dominator.Succs) != 2 {
		return nil, nil, false
	}
	var thenVal, elseVal ssa.Value
	for _, edge := range edges {
		isTrue := branchArmReachesPredecessor(
			dominator,
			block,
			edge.predecessor,
			0,
		)
		isFalse := branchArmReachesPredecessor(
			dominator,
			block,
			edge.predecessor,
			1,
		)
		if isTrue == isFalse {
			return nil, nil, false
		}
		if isTrue {
			if thenVal != nil {
				return nil, nil, false
			}
			thenVal = edge.value
		} else {
			if elseVal != nil {
				return nil, nil, false
			}
			elseVal = edge.value
		}
	}
	return thenVal, elseVal, thenVal != nil && elseVal != nil
}

// branchArmReachesPredecessor identifies an incoming merge edge with one arm.
func branchArmReachesPredecessor(
	branch, merge, predecessor *ssa.BasicBlock,
	successorIndex int,
) bool {
	successor := branch.Succs[successorIndex]
	if successor == merge {
		return predecessor == branch
	}
	return feasibleReachableBefore(successor, predecessor, merge)
}

// lastIf finds a block's terminating branch for phi and loop recognition.
func lastIf(b *ssa.BasicBlock) (*ssa.If, bool) {
	if len(b.Instrs) == 0 {
		return nil, false
	}
	ifi, ok := b.Instrs[len(b.Instrs)-1].(*ssa.If)
	return ifi, ok
}

// functionHasPhi lets branch selection distinguish phi-control branches from
// independent result branches without scanning dominators in phi-free code.
func functionHasPhi(fn *ssa.Function) bool {
	for _, b := range feasibleBlocks(fn) {
		for _, instr := range b.Instrs {
			phi, ok := instr.(*ssa.Phi)
			if ok && len(feasiblePhiEdges(phi)) > 1 {
				return true
			}
		}
	}
	return false
}

// loopSelect validates one detected loop and chooses scalar or recurrence
// hardware. Clock later drives its gated state updates.
func (s *selector) loopSelect(header *ssa.BasicBlock) error {
	cmp, ok := loopCondition(header)
	if !ok {
		return fmt.Errorf("select: loop condition is not a comparison")
	}
	bound, ok := loopBound(cmp, header)
	if !ok {
		return fmt.Errorf("select: could not identify the loop bound")
	}
	if err := validateLoopBound(bound); err != nil {
		return err
	}
	counter, err := validateLoopShape(cmp, header)
	if err != nil {
		return err
	}
	scalar, err := classifyScalarLoopResult(
		header.Parent(),
		header,
		counter,
	)
	if err != nil {
		return err
	}
	if scalar.matched {
		if boundErr := validateScalarLoopBound(bound); boundErr != nil {
			return boundErr
		}
		if flowErr := validateScalarLoopControlFlow(
			header,
			cmp,
			bound,
			counter,
		); flowErr != nil {
			return flowErr
		}
		if instrErr := validateScalarLoopInstructions(header.Parent()); instrErr != nil {
			return instrErr
		}
		return s.buildLoopStop(bound, scalar.increment)
	}

	period := effectiveClockPeriod(s.clockPeriod)
	loop, err := analyseRecurrenceLoopWithBudget(
		header.Parent(),
		header,
		cmp,
		bound,
		counter,
		clockedStateSettleBudgetFor(period),
	)
	if err != nil {
		return err
	}
	return s.buildRecurrenceLoop(loop)
}

// buildLoopStop gives a scalar loop clocked state and a terminating pulse gate.
// The index is a clocked register stepping
// i = phi(0, i + 1), with a stop gate that passes the clock pulse only while
// i < bound. Once i reaches the bound the gate closes and the registers freeze,
// so the display settles on the function's result. The result is its own
// register,
// result = phi(0, result + inc). Both phi nodes, the index and the result, stay
// visible. Clock drives the gate's pulse port.
func (s *selector) buildLoopStop(bound ssa.Value, inc int) error {
	// Index register: i = phi(0, i + 1), gated to stop at the bound.
	idxReg := &register{}
	regI := newInstance(idxReg)
	a1 := newInstance(newArith("+")) // i + 1
	s.add(regI)
	s.add(a1)
	one := s.constSource(1)
	idxReg.inc = one.net // the +1 constant names the index phi's increment
	s.wire(regI.port("value"), a1.port("a"))
	s.wire(one, a1.port("b"))
	s.wire(a1.port("out"), regI.port("next"))

	// Stop gate: pass the clock pulse while i < bound, then freeze the registers.
	// Clock drives the gate's pulse port.
	gate := newInstance(&stopGate{})
	s.add(gate)
	s.wire(regI.port("value"), gate.port("index"))
	if err := s.use(bound, gate.port("bound")); err != nil {
		return err
	}
	s.wire(gate.port("gated"), regI.port("pulse"))

	// The result is its own register, result = phi(0, result + inc), gated by the
	// same pulse so it advances in lockstep with the index and settles together.
	resReg := &register{}
	regR := newInstance(resReg)
	ra1 := newInstance(newArith("+")) // result + inc
	s.add(regR)
	s.add(ra1)
	incSrc := s.constSource(inc)
	resReg.inc = incSrc.net // the +inc constant names the result phi's increment
	s.wire(regR.port("value"), ra1.port("a"))
	s.wire(incSrc, ra1.port("b"))
	s.wire(ra1.port("out"), regR.port("next"))
	s.wire(gate.port("gated"), regR.port("pulse"))

	s.resultNet = s.netOf(regR.port("value"))
	return nil
}

// buildRecurrenceLoop gives validated parallel state simultaneous updates.
// Every phi producer exists before body additions are lowered, so each next
// equation reads the old state and all registers latch on one gated pulse.
func (s *selector) buildRecurrenceLoop(loop recurrenceLoop) error {
	for _, alias := range loop.aliases {
		producer, err := s.portFor(alias.value)
		if err != nil {
			return err
		}
		s.producer[alias.phi] = producer
	}
	registers := make(map[*ssa.Phi]*instance, len(loop.states))
	for _, state := range loop.states {
		reg := newInstance(newRegisterWithInitial(state.initial))
		s.add(reg)
		s.produce(state.phi, reg.port("value"))
		registers[state.phi] = reg
	}

	for _, op := range loop.bodyOps {
		if err := s.binOp(op); err != nil {
			return err
		}
	}

	gate := newInstance(newStopGateWithWarmup(
		effectiveClockPeriod(s.clockPeriod),
	))
	s.add(gate)
	if err := s.use(loop.counter, gate.port("index")); err != nil {
		return err
	}
	if err := s.use(loop.bound, gate.port("bound")); err != nil {
		return err
	}
	for _, state := range loop.states {
		reg := registers[state.phi]
		if err := s.use(state.next, reg.port("next")); err != nil {
			return err
		}
		s.wire(gate.port("gated"), reg.port("pulse"))
	}

	s.resultNet = s.netOf(registers[loop.result].port("value"))
	return nil
}

// wire connects synthesised module ports that have no direct SSA use relation.
func (s *selector) wire(driver, reader *port) {
	n := s.netOf(driver)
	n.readers = append(n.readers, reader)
	reader.net = n
}

// constSource gives synthesised loop values a labelled constant producer.
func (s *selector) constSource(val int) *port {
	in := newInstance(newConstSrc(val))
	s.add(in)
	out := in.port("out")
	out.litLabel = fmt.Sprintf("%d", val)
	s.netOf(out)
	return out
}

// use connects an SSA operand consumer to its selected producer.
func (s *selector) use(v ssa.Value, reader *port) error {
	pp, err := s.portFor(v)
	if err != nil {
		return err
	}
	reader.ssaName = s.valueLabel(v)
	n := s.netOf(pp)
	n.readers = append(n.readers, reader)
	reader.net = n
	return nil
}

// portFor resolves an SSA operand, first unwrapping one-feasible-edge phi
// aliases and then materialising literal sources when needed.
func (s *selector) portFor(v ssa.Value) (*port, error) {
	v = feasibleValue(v)
	if v == nil {
		return nil, fmt.Errorf("select: nil value has no producer")
	}
	if pp, ok := s.producer[v]; ok {
		if pp == nil {
			return nil, fmt.Errorf(
				"select: value %s has nil producer",
				v.Name(),
			)
		}
		return pp, nil
	}
	c, ok := v.(*ssa.Const)
	if !ok {
		return nil, fmt.Errorf("select: value %s has no producer", v.Name())
	}
	val, err := constInt(c)
	if err != nil {
		return nil, err
	}
	in := newInstance(newConstSrc(val))
	s.add(in)
	pp := in.port("out")
	pp.litLabel = constLabel(c, val)
	s.producer[v] = pp
	return pp, nil
}

// constLabel formats literal values for panels, retaining Boolean words.
func constLabel(c *ssa.Const, val int) string {
	if c.Value != nil && c.Value.Kind() == constant.Bool {
		if constant.BoolVal(c.Value) {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%d", val)
}

// netOf gives all consumers of one producer the same public net identity.
func (s *selector) netOf(pp *port) *netlistNet {
	if pp.net != nil {
		return pp.net
	}
	n := &netlistNet{driver: pp, ssaName: pp.ssaName, litLabel: pp.litLabel}
	pp.net = n
	s.nets = append(s.nets, n)
	return n
}

// constInt converts supported SSA constants to Factorio signal values. Booleans
// use the 1/-1 encoding: true
// is 1 and false is -1, never 0, since zero is indistinguishable from an absent
// signal on a Factorio wire (docs/backend.md, "Boolean Encoding").
func constInt(c *ssa.Const) (int, error) {
	if c.Value == nil {
		return 0, nil
	}
	if c.Value.Kind() == constant.Bool {
		if constant.BoolVal(c.Value) {
			return 1, nil
		}
		return -1, nil
	}
	// Int64Val panics on a non-integer kind (a float constant, say), so guard
	// the kind before calling it rather than relying on its ok return.
	if c.Value.Kind() != constant.Int {
		return 0, fmt.Errorf("select: non-integer constant %v", c.Value)
	}
	i, ok := constant.Int64Val(c.Value)
	if !ok {
		return 0, fmt.Errorf("select: constant %v exceeds int64", c.Value)
	}
	if err := validateFactorioInt(i); err != nil {
		return 0, fmt.Errorf("select: constant %v %w", c.Value, err)
	}
	return int(i), nil
}

// validateFactorioInt rejects values that Factorio signals cannot preserve.
// The message is a fragment; callers prepend the offending subject.
func validateFactorioInt(value int64) error {
	if value < math.MinInt32 || value > math.MaxInt32 {
		return fmt.Errorf("is outside Factorio signed 32-bit range")
	}
	return nil
}
