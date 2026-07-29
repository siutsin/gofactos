// This file provides a boolean merge test case with two positive comparisons.
package main

// bothPos tests a short-circuit `&&`: Go lowers it to control flow with a phi
// that merges the constant false (when a <= 0) against the b > 0 comparison, so
// it exercises the boolean merge where one arm is a constant false.
//
// Generated blueprint:
//
// Dataflow:
//
//	[a] -> [a > 0] --+
//	                 +-> [merge] -> [display]
//	[b] -> [b > 0] --+
//
// Layout:
//
//	[A = 1]      [t0 = A > 0]              [normalise t1] [if t0 { merge = t1 }]  [t2 = merge] -> display
//	[B = 1]      [if A ≤ 0]   [t0 = false] [normalise c0] [if !t0 { merge = c0 }]
//	[c0 = false] [t1 = B > 0]
//	             [if B ≤ 0]   [t1 = false]
func bothPos(a, b int) bool {
	return a > 0 && b > 0
}
