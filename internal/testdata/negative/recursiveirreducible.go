// This file rejects recursive control flow outside the supported SSA subset.
package main

// recursiveIrreducible has two entries into one goto cycle. Neither cycle
// entry dominates the other, so dominance back-edge detection cannot find it.
func recursiveIrreducible(n int) int {
	if n < 0 {
		goto left
	}
	goto right

left:
	n--
	if n > 0 {
		goto right
	}
	return recursiveIrreducible(n)

right:
	n--
	if n > 0 {
		goto left
	}
	return recursiveIrreducible(n)
}
