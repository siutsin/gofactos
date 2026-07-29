// This file provides arithmetic whose constant appears before its variable.
package main

// constFirst tests constant-as-first-operand in BinOp. The expression
// 7*b produces SSA with Const(7) as the first operand, exercising
// FirstConstant on the arithmetic combinator.
//
// Generated blueprint:
//
// Dataflow:
//
//	[a] -> [a * a] --+
//	                 +-> [a*a + 7*b] -> [display]
//	[b] -> [7 * b] --+
//
// Layout:
//
//	[A = 1]  [t0 = A * A]  [t2 = t0 + t1] -> display
//	[B = 1]  [t1 = c0 * B]
//	[c0 = 7]
func constFirst(a, b int) int {
	return a*a + 7*b
}
