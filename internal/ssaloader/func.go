// This file selects named SSA functions for the CLI front ends.
package ssaloader

import (
	"go/types"
	"sort"

	"golang.org/x/tools/go/ssa"
)

// includable reports whether fn is source code the commands can process.
func includable(fn *ssa.Function) bool {
	if fn == nil || fn.Synthetic != "" {
		return false
	}
	object := fn.Object()
	if object == nil || object.Name() != "init" {
		return true
	}
	return fn.Signature.Recv() != nil
}

// matchesFilter reports whether fn has the optional requested name.
func matchesFilter(fn *ssa.Function, funcFilter string) bool {
	return funcFilter == "" || fn.Name() == funcFilter
}

// collectClosures recursively collects anonymous functions nested
// inside fn and its descendants. Each closure is checked against
// the same inclusion criteria as top-level functions.
//
// For example, given:
//
//	func MakeAdder(n int) func(int) int {
//		return func(x int) int { return x + n }
//	}
//
// Calling collectClosures on MakeAdder returns [MakeAdder$1].
func collectClosures(fn *ssa.Function, funcFilter string) []*ssa.Function {
	var result []*ssa.Function
	for _, anon := range fn.AnonFuncs {
		if includable(anon) && matchesFilter(anon, funcFilter) {
			result = append(result, anon)
		}
		result = append(result, collectClosures(anon, funcFilter)...)
	}
	return result
}

// collectMethods returns methods declared on the named package type m.
func collectMethods(
	m *ssa.Type,
	prog *ssa.Program,
	funcFilter string,
) []*ssa.Function {
	named, ok := m.Type().(*types.Named)
	if !ok {
		return nil
	}

	var result []*ssa.Function
	for method := range named.Methods() {
		fn := prog.FuncValue(method)
		if !includable(fn) {
			continue
		}
		if matchesFilter(fn, funcFilter) {
			result = append(result, fn)
		}
		result = append(result, collectClosures(fn, funcFilter)...)
	}
	return result
}

// CollectFunctions returns the non-synthetic, non-init functions from pkg,
// including methods on named types and anonymous/closure functions,
// sorted by source position. If funcFilter is non-empty, only functions
// with that name are returned.
//
// For example, given:
//
//	type Counter struct{ value int }
//	func (c *Counter) Increment() { c.value++ }
//	func MakeAdder(n int) func(int) int {
//		return func(x int) int { return x + n }
//	}
//
// CollectFunctions returns [Increment, MakeAdder, MakeAdder$1].
func CollectFunctions(pkg *ssa.Package, funcFilter string) []*ssa.Function {
	var functions []*ssa.Function

	for _, member := range pkg.Members {
		switch m := member.(type) {
		case *ssa.Function:
			if !includable(m) {
				continue
			}
			if matchesFilter(m, funcFilter) {
				functions = append(functions, m)
			}
			functions = append(functions, collectClosures(m, funcFilter)...)

		case *ssa.Type:
			typeName, ok := m.Object().(*types.TypeName)
			if ok && typeName.IsAlias() {
				continue
			}
			functions = append(functions, collectMethods(m, pkg.Prog, funcFilter)...)
		}
	}
	// Sort by source position to produce deterministic output
	// matching the order functions appear in the source file.
	sort.Slice(functions, func(i, j int) bool {
		return functions[i].Pos() < functions[j].Pos()
	})
	return functions
}
