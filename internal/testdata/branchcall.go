// This file provides ordinary-call test cases on both sides of a branch.
package main

// absolute gives branchCall a reusable callee with divergent returns.
func absolute(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// branchCall proves selected branches can invoke the same static callee.
func branchCall(n int) int {
	if n == 0 {
		return absolute(-1)
	}
	return absolute(n)
}
