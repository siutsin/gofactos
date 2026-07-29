// This file provides unused arithmetic for dead-instance pruning tests.
package main

// deadExpr exercises dead intermediate computations. a*b is discarded and does
// not touch the result; r*r is discarded but reads the returned value r. Both
// arithmetic combinators have unwired outputs and must be pruned in Select.
// Pruning seeds liveness from the result's display, not from every reader of
// the result net, so the result-reading dead r*r is removed too rather than
// reaching Emit as a floating combinator. a and b still feed the result.
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
func deadExpr(a, b int) int {
	r := a + b
	_ = a * b
	_ = r * r
	return r
}
