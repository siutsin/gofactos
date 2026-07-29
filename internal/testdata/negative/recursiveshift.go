// This file rejects bit shifting in the recursive instruction subset.
package main

// recursiveShift uses an operator outside the scalar arithmetic subset.
func recursiveShift(n int) int {
	if n <= 0 {
		return 1
	}
	return recursiveShift(n-1) << 1
}
