// This file rejects mutually recursive functions as one unsupported cycle.
package main

// a and b form an unsupported mutual-recursion cycle. The bounded runtime
// accepts direct self calls only.
func a(n int) int {
	if n <= 0 {
		return n
	}
	return b(n - 1)
}

// b closes the mutual-recursion cycle required by the rejected test case.
func b(n int) int {
	if n <= 0 {
		return n
	}
	return a(n - 1)
}
