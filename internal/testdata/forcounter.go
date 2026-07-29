// This file provides a counted loop whose result is the counter itself.
package main

// forCounter returns the loop counter after a canonical counted loop.
func forCounter(n int) int {
	i := 0
	for ; i < n; i++ {
	}
	return i
}
