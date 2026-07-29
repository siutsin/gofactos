// This file rejects a multi-way return merge outside the two-input phi subset.
package main

// sign is a rejected test case. Its early returns form a multi-way result
// merge.
// Select supports sequential two-input joins such as clamp, but this shape
// returns a clear "more than one branch is unsupported" error.
//
// Rejected at Select (no blueprint):
//
//	[n < 0] then [n > 0] -> multi-way return merge -> Select returns
//	"more than one branch is unsupported", no entities
func sign(n int) int {
	if n < 0 {
		return -1
	}
	if n > 0 {
		return 1
	}
	return 0
}
