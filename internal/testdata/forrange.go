// This file provides the range-over-integer form of a counted loop.
package main

// forRange exercises range-over-integer's self-loop SSA shape so back-edge
// recognition cannot assume a separate loop body block.
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
func forRange(n int) int {
	result := 0
	for range n {
		result += 2
	}
	return result
}
