// This file provides a direct boolean comparison and display test case.
package main

// greater tests a boolean returned straight from a comparison, with no if/else
// merge. The result net is the compare's 1/-1 condition, which the boolDisplay
// shows as a check or deny icon. isEven returns constant booleans through a
// branch. This returns the comparison itself.
//
// Generated blueprint:
//
// Dataflow:
//
//	[a] -+
//	     +-> [a > b] -> [display]
//	[b] -+
//
// Layout:
//
//	[A = 1] [t0 = A > B] -> display
//	[B = 1] [if A ≤ B]   [t0 = false]
func greater(a, b int) bool {
	return a > b
}
