// This file provides the basic arithmetic case for blueprint generation.
package main

// add is the simplest test case: a single basic block with one
// arithmetic operation. Tests parameter wiring, signal allocation,
// and the return value display chain.
//
// Generated blueprint:
//
// Dataflow:
//
//	[a] -+
//	     +-> [a + b] -> [display]
//	[b] -+
//
// Layout:
//
//	[A = 1] [t0 = A + B] -> display
//	[B = 1]
func add(a, b int) int {
	return a + b
}
