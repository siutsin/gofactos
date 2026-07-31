//go:build ignore

// This file demonstrates direct recursion through Fibonacci.
package main

// fibonacci returns the nth Fibonacci number using direct recursion.
func fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	return fibonacci(n-1) + fibonacci(n-2)
}
