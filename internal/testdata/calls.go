// This file provides reusable ordinary-call expansion test cases.
package main

// square gives call expansion a non-trivial arithmetic callee.
func square(n int) int {
	return n * n
}

// sumSquares proves independent calls retain their arguments and results.
func sumSquares(a, b int) int {
	return square(a) + square(b)
}

// identity provides the smallest supported ordinary callee.
func identity(n int) int {
	return n
}

// sumIdentities proves repeated calls do not share invocation state.
func sumIdentities(n int) int {
	return identity(n) + identity(n)
}
