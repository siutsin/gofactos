// This file rejects helper calls inside a direct-recursive root.
package main

// decrement creates the unsupported helper edge inside the recursive body.
func decrement(n int) int {
	return n - 1
}

// recursiveHelper is rejected because its recursive cycle reaches an ordinary
// helper.
func recursiveHelper(n int) int {
	if n <= 0 {
		return n
	}
	return recursiveHelper(decrement(n))
}
