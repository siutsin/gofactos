// This file provides enough inputs to exercise wide blueprint placement.
package main

// wide tests relay pole insertion. Four parameters stacked vertically
// produce wires exceeding the 9-tile circuit reach limit, triggering
// automatic medium-pole relay placement.
//
// Generated blueprint:
//
// Dataflow:
//
//	[a] -+
//	[b] -+
//	     +-> [a + b + c + d] -> [display]
//	[c] -+
//	[d] -+
//
// Layout:
//
//	[A = 1] [t0 = A + B] [t1 = t0 + C] [t2 = t1 + D] -> display
//	[B = 1]
//	[C = 1]
//	[D = 1]
func wide(a, b, c, d int) int {
	return a + b + c + d
}
