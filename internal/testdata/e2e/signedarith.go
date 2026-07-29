// This file supplies signed arithmetic for real Factorio engine verification.
package main

// signedArithmetic combines signed division and remainder in one test case.
func signedArithmetic(n int) int {
	return n/3 + n%3
}
