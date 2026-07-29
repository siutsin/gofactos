// This file defines the finite signal banks used to allocate readable nets.
package factorio

import "fmt"

// Allocate draws from two finite register banks so the readout is legible: a
// parameter lands on its letter, an intermediate value on a distinct item
// signal.

// inputSignals maps live parameter nets to signal-A through signal-Z by
// signature position. Allocate fails if a used parameter exceeds the bank.
var inputSignals = func() []signalID {
	signals := make([]signalID, 0, 26)
	for c := 'A'; c <= 'Z'; c++ {
		signals = append(signals, signalID{Type: "virtual", Name: fmt.Sprintf("signal-%c", c)})
	}
	return signals
}()

// intermediateSignals maps every non-parameter net (computed values, literal
// constants, the result readout, the clock pulse) to a Factorio intermediate-
// product item signal, kept distinct from the letter inputs so the two are
// legible at a glance. The "intermediate product" category is a deliberate pun
// on the intermediate values it carries. Fluids and barrels are excluded, so
// every entry is a plain item signal. The order is fixed, so the Nth
// intermediate is the same signal across every program. It is the intermediate
// register bank; Allocate hard-fails once a program needs more than this many.
var intermediateSignals = []signalID{
	item("iron-plate"),
	item("copper-plate"),
	item("steel-plate"),
	item("iron-gear-wheel"),
	item("copper-cable"),
	item("electronic-circuit"),
	item("advanced-circuit"),
	item("processing-unit"),
	item("iron-stick"),
	item("plastic-bar"),
	item("sulfur"),
	item("battery"),
	item("explosives"),
	item("engine-unit"),
	item("electric-engine-unit"),
	item("flying-robot-frame"),
	item("low-density-structure"),
	item("rocket-fuel"),
	item("uranium-235"),
	item("uranium-238"),
	item("uranium-fuel-cell"),
}

// item keeps the intermediate signal bank concise and consistently typed.
func item(name string) signalID { return signalID{Type: "item", Name: name} }

// intermediateAlias maps an intermediate item signal name to a short, stable
// label token (iron-plate -> i0), so panel labels stay scannable while the wire
// still carries the distinct item signal. The index is the signal's fixed
// position in intermediateSignals, so the same item is the same token across
// every program.
var intermediateAlias = func() map[string]string {
	m := make(map[string]string, len(intermediateSignals))
	for i, s := range intermediateSignals {
		m[s.Name] = fmt.Sprintf("i%d", i)
	}
	return m
}()
