// This file orchestrates compilation from SSA to an importable blueprint.
package factorio

import (
	"fmt"

	"golang.org/x/tools/go/ssa"
)

// compilation bundles the emitted blueprint with the result net the simulator
// reads. It is the internal result simulator-based tests use, distinct from
// Compile's blueprint-only result.
type compilation struct {
	e         *emitter
	resultNet *netlistNet
}

// compileFunction runs every backend phase needed by Compile and simulator
// tests. It returns the emitter plus the result net. The phases run in order:
// Select (SSA to netlist), Clock (insert the shared clock), Allocate (one
// signal per public net), Verify (structural netlist checks), Place
// (coordinates), Emit (number entities and materialise edges), Route (insert
// relay poles past reach), Power (add substations), then a final Verify of
// overlap, reach, powered entities, and colour.
func compileFunction(fn *ssa.Function, opts ...Option) (*compilation, error) {
	sel, err := selectFunc(fn, opts...)
	if err != nil {
		return nil, err
	}
	clockPhase(sel)
	if err := allocateSignals(sel.nets); err != nil {
		return nil, fmt.Errorf("allocate %s: %w", fn.Name(), err)
	}
	if err := verifyNetlist(sel.insts, sel.nets); err != nil {
		return nil, fmt.Errorf("verify %s: %w", fn.Name(), err)
	}
	place(sel.insts, sel.nets)
	e := emitNetlist(sel.insts, netEdges(sel.nets))
	if err := insertRelays(e); err != nil {
		return nil, fmt.Errorf("route %s: %w", fn.Name(), err)
	}
	if err := addPower(e); err != nil {
		return nil, fmt.Errorf("power %s: %w", fn.Name(), err)
	}
	if err := verifyOutput(e); err != nil {
		return nil, fmt.Errorf("verify %s: %w", fn.Name(), err)
	}
	return &compilation{e: e, resultNet: sel.resultNet}, nil
}

// Compile exposes SSA-to-blueprint compilation to the CLI.
// The blueprint label is the function name; the player edits parameter
// constant combinators to feed inputs.
func Compile(fn *ssa.Function, opts ...Option) (*BlueprintWrapper, error) {
	compiled, err := compileFunction(fn, opts...)
	if err != nil {
		return nil, err
	}
	return &BlueprintWrapper{
		Blueprint: Blueprint{
			Item:     "blueprint",
			Label:    fn.Name(),
			Version:  blueprintVersion,
			Entities: compiled.e.entities,
			Wires:    compiled.e.wires,
		},
	}, nil
}
