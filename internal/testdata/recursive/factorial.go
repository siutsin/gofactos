// This file provides multiplication across a suspended recursive call.
package main

// factorial exercises one recursive call and a caller value live across it.
func factorial(n int) int {
	if n <= 1 {
		return 1
	}
	return n * factorial(n-1)
}
