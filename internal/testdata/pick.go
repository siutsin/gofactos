// This file provides a boolean-parameter test case with divergent values.
package main

// pick exercises a boolean parameter used directly as a branch condition. The
// bool param rides the 1/-1 encoding (false is -1), so --set b=0 selects the
// false branch; without the encoding the condition would be absent, not false.
//
// Generated blueprint:
//
// Dataflow:
//
//	      +-> 5 (if b) --+
//	[b] --+              +-> [merge] -> [display]
//	      +-> 3 (else) --+
//
// Layout:
//
//	[A = 1]  [normalise c0] [if A { merge = c0 }]  [i2 = merge] -> display
//	[c0 = 5] [normalise c1] [if !A { merge = c1 }]
//	[c1 = 3]
func pick(b bool) int {
	if b {
		return 5
	}
	return 3
}
