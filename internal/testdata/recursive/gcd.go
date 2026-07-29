// This file provides tail recursion with two arguments and remainder.
package main

// gcd exercises two recursive arguments, remainder, and a tail-position call.
func gcd(a, b int) int {
	if b == 0 {
		return a
	}
	return gcd(b, a%b)
}
