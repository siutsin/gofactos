// This file preserves the n < 2 absolute-value test case.
package main

// abs returns -n below the n < 2 boundary and n otherwise.
func abs(n int) int {
	if n < 2 {
		return -n
	}
	return n
}
