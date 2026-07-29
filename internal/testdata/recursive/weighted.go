// This file provides a phi merge and an unused root argument for recursion.
package main

// weighted exercises a phi merge while retaining its ignored root argument.
func weighted(n int, double bool, ignored int) int {
	if n <= 0 {
		return 0
	}
	amount := 1
	if double {
		amount = 2
	}
	return amount + weighted(n-1, double, 0)
}
