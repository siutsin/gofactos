// This file plans supported call graphs before they become physical circuits.
package factorio

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// maxExpandedInvocations bounds the physical tree allocated by expand.
const maxExpandedInvocations = 1_024

// callProgram is the immutable result of call-graph preflight. Ordinary calls
// use an expanded invocation tree. A supported direct self-cycle carries its
// compiled runtime program instead.
type callProgram struct {
	root      *invocationPlan
	recursive *recursiveProgram
}

// invocationPlan is one physical expansion of a source function. A source call
// instruction can occur in several physical invocations, so each parent owns
// its own child map and expansion path.
type invocationPlan struct {
	fn           *ssa.Function
	path         string
	instructions []ssa.Instruction
	hasPhi       bool
	calls        map[*ssa.Call]*invocationPlan
}

type callGraph struct {
	root      *ssa.Function
	nodes     map[*ssa.Function]*callNode
	edgeCount int
}

type callNode struct {
	fn           *ssa.Function
	instructions []ssa.Instruction
	hasPhi       bool
	calls        []callSite
}

type callSite struct {
	instr   *ssa.Call
	callee  *ssa.Function
	block   int
	index   int
	ordinal int
}

// planCallProgram validates every call reachable through feasible control flow
// before Select creates the real netlist. A function with no such calls
// receives an empty root plan so existing no-call diagnostics and output remain
// unchanged.
func planCallProgram(root *ssa.Function) (*callProgram, error) {
	graph, err := buildCallGraph(root)
	if err != nil {
		return nil, err
	}
	if graph.edgeCount == 0 {
		return &callProgram{root: &invocationPlan{fn: root}}, nil
	}
	if loopErr := graph.rejectLoops(); loopErr != nil {
		return nil, loopErr
	}

	components := graph.components()
	if validationErr := graph.validateBodies(components); validationErr != nil {
		return nil, validationErr
	}
	recursive, err := graph.recursiveFunctions(components)
	if err != nil {
		return nil, err
	}
	if len(recursive) > 1 {
		names := make([]string, len(recursive))
		for i, fn := range recursive {
			names[i] = fn.Name()
		}
		sort.Strings(names)
		return nil, fmt.Errorf(
			"select: more than one recursive machine: %s",
			strings.Join(names, ", "),
		)
	}
	if len(recursive) == 1 {
		return recursiveCallProgram(root, recursive[0])
	}
	invocations := graph.expandedInvocationCount(
		root,
		make(map[*ssa.Function]int, len(graph.nodes)),
	)
	if invocations > maxExpandedInvocations {
		return nil, fmt.Errorf(
			"select: call expansion exceeds %d physical invocations",
			maxExpandedInvocations,
		)
	}
	return &callProgram{root: graph.expand(root, "")}, nil
}

// rejectLoops rejects call graphs that combine calls with unsupported CFG
// loops.
func (graph *callGraph) rejectLoops() error {
	for _, fn := range graph.functions() {
		if hasControlFlowCycle(fn) {
			return fmt.Errorf(
				"select: calls involving loops are unsupported: %s",
				fn.Name(),
			)
		}
	}
	return nil
}

// recursiveFunctions separates supported self-cycles from mutual recursion.
func (graph *callGraph) recursiveFunctions(
	components [][]*ssa.Function,
) ([]*ssa.Function, error) {
	var recursive []*ssa.Function
	for _, component := range components {
		if len(component) > 1 {
			names := make([]string, len(component))
			for index, fn := range component {
				names[index] = fn.Name()
			}
			sort.Strings(names)
			return nil, fmt.Errorf(
				"select: mutual recursion is unsupported: %s",
				strings.Join(names, ", "),
			)
		}
		fn := component[0]
		if !graph.hasEdge(fn, fn) {
			continue
		}
		recursive = append(recursive, fn)
	}
	return recursive, nil
}

// recursiveCallProgram routes a root self-cycle through the runtime machine.
func recursiveCallProgram(
	root *ssa.Function,
	recursive *ssa.Function,
) (*callProgram, error) {
	if recursive != root {
		return nil, fmt.Errorf(
			"select: recursive function must be the selected root: %s",
			recursive.Name(),
		)
	}
	program, err := planRecursiveProgram(root)
	if err != nil {
		return nil, err
	}
	return &callProgram{
		root:      &invocationPlan{fn: root},
		recursive: program,
	}, nil
}

// validateBodies ensures every acyclic function can use ordinary lowering.
func (graph *callGraph) validateBodies(
	components [][]*ssa.Function,
) error {
	cyclic := make(map[*ssa.Function]bool)
	for _, component := range components {
		if len(component) > 1 || graph.hasEdge(component[0], component[0]) {
			for _, fn := range component {
				cyclic[fn] = true
			}
		}
	}
	for _, fn := range graph.functions() {
		if cyclic[fn] {
			continue
		}
		node := graph.nodes[fn]
		instructions, err := validateCallBody(node)
		if err != nil {
			role := "callee"
			if node.fn == graph.root {
				role = "root"
			}
			return fmt.Errorf(
				"select: unsupported %s body %s: %w",
				role,
				node.fn.Name(),
				err,
			)
		}
		node.instructions = instructions
		node.hasPhi = functionHasPhi(fn)
	}
	return nil
}

// buildCallGraph discovers calls reachable through feasible control flow.
func buildCallGraph(root *ssa.Function) (*callGraph, error) {
	graph := &callGraph{
		root:  root,
		nodes: make(map[*ssa.Function]*callNode),
	}
	if err := graph.visit(root); err != nil {
		return nil, err
	}
	return graph, nil
}

// visit adds one function and recursively discovers its static callees.
func (graph *callGraph) visit(fn *ssa.Function) error {
	if _, ok := graph.nodes[fn]; ok {
		return nil
	}
	node := &callNode{fn: fn}
	graph.nodes[fn] = node
	for _, block := range feasibleBlocks(fn) {
		if err := graph.visitBlock(node, block, block.Index); err != nil {
			return err
		}
	}
	node.numberCalls()
	return nil
}

// visitBlock records calls in source order for stable physical expansion.
func (graph *callGraph) visitBlock(
	node *callNode,
	block *ssa.BasicBlock,
	blockIndex int,
) error {
	for instrIndex, instruction := range block.Instrs {
		callInstruction, ok := instruction.(ssa.CallInstruction)
		if !ok {
			continue
		}
		call := callInstruction.Value()
		if call == nil {
			return fmt.Errorf(
				"select: non-ordinary call in %s is unsupported",
				node.fn.Name(),
			)
		}
		callee, err := classifyStaticCall(graph.root, node.fn, call)
		if err != nil {
			return err
		}
		node.calls = append(node.calls, callSite{
			instr:  call,
			callee: callee,
			block:  blockIndex,
			index:  instrIndex,
		})
		graph.edgeCount++
		if err := graph.visit(callee); err != nil {
			return err
		}
	}
	return nil
}

// classifyStaticCall admits only calls that this package can safely lower.
func classifyStaticCall(
	root *ssa.Function,
	caller *ssa.Function,
	call *ssa.Call,
) (*ssa.Function, error) {
	common := call.Common()
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return nil, unsupportedCall("built-in call", builtin.Name(), caller)
	}
	callee := common.StaticCallee()
	if callee == nil {
		return nil, unsupportedCall("dynamic call", common.String(), caller)
	}
	if callee.Signature.Recv() != nil || callee.Parent() != nil {
		return nil, unsupportedCall(
			"method or closure call",
			callee.Name(),
			caller,
		)
	}
	if isGenericFunction(callee) || callee.Signature.Variadic() {
		return nil, unsupportedCall(
			"generic or variadic call",
			callee.Name(),
			caller,
		)
	}
	// Imported functions may be bodyless and synthetic; keep them external.
	if callee.Synthetic != "" &&
		(callee.Pkg == nil ||
			callee.Pkg == root.Pkg ||
			len(callee.Blocks) > 0) {
		return nil, unsupportedCall("synthetic call", callee.Name(), caller)
	}
	if callee.Pkg != root.Pkg {
		return nil, unsupportedCall("external call", callee.Name(), caller)
	}
	if err := supportedCallSignature(callee, len(common.Args)); err != nil {
		return nil, fmt.Errorf(
			"select: unsupported parameter or result signature %s: %w",
			callee.Name(),
			err,
		)
	}
	if len(callee.Blocks) == 0 {
		return nil, unsupportedCall("bodyless call", callee.Name(), caller)
	}
	return callee, nil
}

// unsupportedCall gives rejected call categories one consistent diagnostic.
func unsupportedCall(category, callee string, caller *ssa.Function) error {
	return fmt.Errorf(
		"select: %s %s in %s is unsupported",
		category,
		callee,
		caller.Name(),
	)
}

// isGenericFunction detects generic forms that require unsupported monomorphy.
func isGenericFunction(fn *ssa.Function) bool {
	params := fn.TypeParams()
	return (params != nil && params.Len() > 0) ||
		len(fn.TypeArgs()) > 0 ||
		(fn.Origin() != nil && fn.Origin() != fn)
}

// supportedCallSignature protects call wiring from arity and type mismatches.
func supportedCallSignature(fn *ssa.Function, argumentCount int) error {
	if err := supportedSignature(fn); err != nil {
		return err
	}
	if fn.Signature.Results().Len() != 1 {
		return fmt.Errorf(
			"expected one result, got %d",
			fn.Signature.Results().Len(),
		)
	}
	if len(fn.Params) != argumentCount {
		return fmt.Errorf(
			"expected %d arguments, got %d",
			len(fn.Params),
			argumentCount,
		)
	}
	return nil
}

// numberCalls assigns stable callee ordinals for readable invocation paths.
func (node *callNode) numberCalls() {
	sort.SliceStable(node.calls, func(i, j int) bool {
		a, b := node.calls[i], node.calls[j]
		if a.instr.Pos() != b.instr.Pos() {
			return a.instr.Pos() < b.instr.Pos()
		}
		if a.block != b.block {
			return a.block < b.block
		}
		return a.index < b.index
	})
	counts := make(map[*ssa.Function]int)
	for i := range node.calls {
		counts[node.calls[i].callee]++
		node.calls[i].ordinal = counts[node.calls[i].callee]
	}
}

// validateCallBody proves a function body can use the ordinary selector path.
func validateCallBody(node *callNode) ([]ssa.Instruction, error) {
	s := &selector{selectorFrame: selectorFrame{
		producer: make(map[ssa.Value]*port),
	}}
	for _, parameter := range node.fn.Params {
		in := newInstance(newConstSrc(1))
		s.add(in)
		s.producer[parameter] = in.port("out")
	}
	s.hasPhi = functionHasPhi(node.fn)
	var instructions []ssa.Instruction
	for _, block := range feasibleBlocks(node.fn) {
		for _, instr := range block.Instrs {
			instructions = append(instructions, instr)
			if err := validateCallInstruction(s, instr); err != nil {
				return nil, err
			}
		}
	}
	if err := s.resolveResult(node.fn); err != nil {
		return nil, err
	}
	return instructions, nil
}

// validateCallInstruction substitutes a source for calls during body preflight.
func validateCallInstruction(s *selector, instruction ssa.Instruction) error {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return s.instr(instruction)
	}
	for _, argument := range call.Common().Args {
		if _, err := s.portFor(argument); err != nil {
			return err
		}
	}
	in := newInstance(newConstSrc(1))
	s.add(in)
	s.produce(call, in.port("out"))
	return nil
}

// hasEdge supports cycle classification without exposing graph internals.
func (graph *callGraph) hasEdge(from, to *ssa.Function) bool {
	for _, site := range graph.nodes[from].calls {
		if site.callee == to {
			return true
		}
	}
	return false
}

// components identifies recursion through the graph's strongly connected
// components using Tarjan's algorithm. Components and their members are sorted
// for stable diagnostics.
func (graph *callGraph) components() [][]*ssa.Function {
	finder := newComponentFinder(graph)
	for _, fn := range graph.functions() {
		if finder.indexes[fn] == 0 {
			finder.connect(fn)
		}
	}
	return finder.components
}

type componentFinder struct {
	graph      *callGraph
	index      int
	indexes    map[*ssa.Function]int
	low        map[*ssa.Function]int
	onStack    map[*ssa.Function]bool
	stack      []*ssa.Function
	components [][]*ssa.Function
}

// newComponentFinder initialises Tarjan state for one independent graph scan.
func newComponentFinder(graph *callGraph) *componentFinder {
	return &componentFinder{
		graph:   graph,
		indexes: make(map[*ssa.Function]int),
		low:     make(map[*ssa.Function]int),
		onStack: make(map[*ssa.Function]bool),
	}
}

// connect visits one function while maintaining Tarjan's low-link invariant.
func (finder *componentFinder) connect(fn *ssa.Function) {
	finder.index++
	finder.indexes[fn] = finder.index
	finder.low[fn] = finder.index
	finder.stack = append(finder.stack, fn)
	finder.onStack[fn] = true
	for _, site := range finder.graph.nodes[fn].calls {
		finder.connectEdge(fn, site.callee)
	}
	if finder.low[fn] == finder.indexes[fn] {
		finder.popComponent(fn)
	}
}

// connectEdge folds one call edge into Tarjan's low-link calculation.
func (finder *componentFinder) connectEdge(from, to *ssa.Function) {
	if finder.indexes[to] == 0 {
		finder.connect(to)
		finder.low[from] = min(finder.low[from], finder.low[to])
		return
	}
	if finder.onStack[to] {
		finder.low[from] = min(finder.low[from], finder.indexes[to])
	}
}

// popComponent closes a detected recursion group for later validation.
func (finder *componentFinder) popComponent(root *ssa.Function) {
	var component []*ssa.Function
	for {
		last := finder.stack[len(finder.stack)-1]
		finder.stack = finder.stack[:len(finder.stack)-1]
		finder.onStack[last] = false
		component = append(component, last)
		if last == root {
			break
		}
	}
	sortFunctions(component)
	finder.components = append(finder.components, component)
}

// sortFunctions makes graph traversal and diagnostics reproducible.
func sortFunctions(functions []*ssa.Function) {
	sort.Slice(functions, func(i, j int) bool {
		if functions[i].Pos() != functions[j].Pos() {
			return functions[i].Pos() < functions[j].Pos()
		}
		return functions[i].String() < functions[j].String()
	})
}

// functions provides the stable traversal order used by every graph pass.
func (graph *callGraph) functions() []*ssa.Function {
	functions := make([]*ssa.Function, 0, len(graph.nodes))
	for fn := range graph.nodes {
		functions = append(functions, fn)
	}
	sortFunctions(functions)
	return functions
}

// expandedInvocationCount prevents acyclic source calls from exploding layout.
func (graph *callGraph) expandedInvocationCount(
	fn *ssa.Function,
	memo map[*ssa.Function]int,
) int {
	if count, ok := memo[fn]; ok {
		return count
	}

	count := 1
	for _, site := range graph.nodes[fn].calls {
		childCount := graph.expandedInvocationCount(site.callee, memo)
		if childCount > maxExpandedInvocations-count {
			count = maxExpandedInvocations + 1
			break
		}
		count += childCount
	}
	memo[fn] = count
	return count
}

// expand gives every feasible source call site its own invocation subtree.
func (graph *callGraph) expand(fn *ssa.Function, path string) *invocationPlan {
	node := graph.nodes[fn]
	plan := &invocationPlan{
		fn:           fn,
		path:         path,
		instructions: node.instructions,
		hasPhi:       node.hasPhi,
		calls:        make(map[*ssa.Call]*invocationPlan),
	}
	for _, site := range node.calls {
		segment := fmt.Sprintf("%s#%d", site.callee.Name(), site.ordinal)
		childPath := segment
		if path != "" {
			childPath = path + "/" + segment
		}
		plan.calls[site.instr] = graph.expand(site.callee, childPath)
	}
	return plan
}
