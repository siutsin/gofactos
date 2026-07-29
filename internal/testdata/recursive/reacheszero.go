// This file provides recursive branching with a boolean result.
package main

// reachesZero exercises multiple branches and a recursive boolean result.
func reachesZero(n int) bool {
	if n < 0 {
		return false
	}
	if n == 0 {
		return true
	}
	return reachesZero(n - 1)
}
