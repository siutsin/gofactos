// This file provides the main three-clause counted-loop test case.
package main

// forI tests for-loop Phi node support. The parameter n controls the
// loop count, so the user can adjust it in Factorio. The loop produces
// Phi nodes for result and i, exercising signal reuse (feedback values
// share the Phi's signal), feedback state, and stop-gate wiring.
//
// Generated blueprint (stops at n and settles on the returned value):
//
// Dataflow:
//
//	                         +-> [i phi] ------> [i + 1]
//	[START] -> [clock] -> [gate i < n] -+
//	                         +-> [result phi] -+-> [result + 2]
//	                                          +-> [display]
//
// Layout:
//
//	[clock] [START] [A = 1] [run while i1 < A]
//	                [c0 = 1] [i1 = φ(0, i1 + c0)]
//	                         [i2 = i1 + c0]
//	                [c1 = 2] [i5 = φ(0, i5 + c1)] -> display
//	                         [i6 = i5 + c1]
func forI(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result += 2
	}
	return result
}
