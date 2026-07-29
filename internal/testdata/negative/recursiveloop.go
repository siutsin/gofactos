// This file rejects a loop embedded in a direct-recursive function.
package main

// recursiveLoop combines a control-flow loop with recursive calls.
func recursiveLoop(n int) int {
	total := 0
	for i := 0; i < n; i++ {
		total++
	}
	if n <= 0 {
		return total
	}
	return total + recursiveLoop(n-1)
}
