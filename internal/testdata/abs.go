// This file provides the absolute-value branch test case for compiler tests.
package main

// abs tests multiple return paths through the same-signal merge. Two arms
// return different non-constant values (-n and n), exercising the compare, neg,
// and phi modules that lower an if/else result branch.
//
// Generated blueprint:
//
// Dataflow:
//
//	      +-> [n < 0] --(cond)--+
//	[n] --+----- n (else) ------+-> [merge] -> [display]
//	      +-> [-n] --(if n<0)---+
//
// Layout:
//
//	[A = 1] [t0 = A < 0]              [normalise t1] [if t0 { merge = t1 }] [i2 = merge] -> display
//	        [if A ≥ 0]   [t0 = false] [normalise A]  [if !t0 { merge = A }]
//	        [t1 = -A]
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
