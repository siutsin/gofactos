// This file provides modulo, comparison, and boolean-display coverage.
package main

// isEven tests a boolean returned through a branch. Modulo feeds a comparison
// encoded as 1/-1, the phi merges constant true and false arms, and boolDisplay
// reads the result directly to show a check or deny icon.
//
// Generated blueprint:
//
// Dataflow:
//
//	                      +-> true (if even) --+
//	[n] -> [n % 2 == 0] --+                    +-> [merge] -> [check/deny]
//	                      +-> false (else) ----+
//
// Layout:
//
//	[A = 1]      [t0 = A % c0] [t1 = t0 = 0]              [normalise c1] [if t1 { merge = c1 }]  [i5 = merge] -> check/deny
//	[c0 = 2]                   [if t0 ≠ 0]   [t1 = false] [normalise c2] [if !t1 { merge = c2 }]
//	[c1 = true]
//	[c2 = false]
func isEven(n int) bool {
	if n%2 == 0 {
		return true
	}
	return false
}
