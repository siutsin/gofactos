// This file provides generic direct-recursion coverage through Fibonacci.
package main

// fibonacci exercises two recursive calls in one expression.
func fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	return fibonacci(n-1) + fibonacci(n-2)
}
