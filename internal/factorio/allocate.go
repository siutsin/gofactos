// This file assigns finite Factorio signals to abstract public nets.
package factorio

import "fmt"

// allocateSignals gives every public net a distinct, readable Factorio signal.
// It draws from two finite register
// banks: parameters take their letter from inputSignals by signature position,
// every other (intermediate) net takes the next item signal from
// intermediateSignals in encounter order. There is no reuse and no spill; each
// bank is a hard wall that fails when exhausted.
func allocateSignals(nets []*netlistNet) error {
	var intermediates []*netlistNet
	for _, n := range nets {
		if !n.isInput {
			intermediates = append(intermediates, n)
			continue
		}
		if n.inputIndex >= len(inputSignals) {
			return fmt.Errorf(
				"input signal bank exhausted: parameter %d, %d signals",
				n.inputIndex,
				len(inputSignals),
			)
		}
		n.signal = inputSignals[n.inputIndex]
	}
	if len(intermediates) > len(intermediateSignals) {
		return fmt.Errorf(
			"intermediate signal bank exhausted: %d nets, %d signals",
			len(intermediates),
			len(intermediateSignals),
		)
	}
	for i, n := range intermediates {
		n.signal = intermediateSignals[i]
	}
	return nil
}
