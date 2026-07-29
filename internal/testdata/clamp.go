// This file provides a two-bound branch test case with sequential merges.
package main

// clamp constrains x to [0, 100] with two sequential if/assign merges. Each is
// a real SSA phi node, and both compare against a constant bound, so it lowers
// with the existing compare module plus mid-function phi handling.
//
// Generated blueprint:
//
// Dataflow:
//
//	[x] -> [if x < 0: 0] -> [if x > 100: 100] -> [display]
//
// Layout:
//
//	[A = 1]    [t0 = A < 0]              [normalise c0] [if t0 { merge = c0 }] [t1 = merge] [t2 = t1 > 100]              [normalise c1] [if t2 { merge = c1 }]  [t3 = merge] -> display
//	[c0 = 0]   [if A ≥ 0]   [t0 = false] [normalise A]  [if !t0 { merge = A }]              [if t1 ≤ 100]   [t2 = false] [normalise t1] [if !t2 { merge = t1 }]
//	[c1 = 100]
func clamp(x int) int {
	if x < 0 {
		x = 0
	}
	if x > 100 {
		x = 100
	}
	return x
}
