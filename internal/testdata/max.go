// This file provides a branch merge that selects the greater input.
package main

// max tests the `>=` comparison and a phi that merges two parameters. The
// existing merges pick between a value and its negation (abs) or a value and a
// constant bound (clamp); none select between two distinct parameters.
//
// Generated blueprint:
//
// Dataflow:
//
//	      +-> [a >= b] --(cond)--+
//	[a] --+----- a (if a>=b) ----+-> [merge] -> [display]
//	[b] --+----- b (else) -------+
//
// Layout:
//
//	[A = 1] [t0 = A ≥ B]              [normalise A] [if t0 { merge = A }]  [i1 = merge] -> display
//	[B = 1] [if A < B]   [t0 = false] [normalise B] [if !t0 { merge = B }]
func max(a, b int) int {
	if a >= b {
		return a
	}
	return b
}
