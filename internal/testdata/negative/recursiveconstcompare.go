// This file rejects a recursive body that compares two constants.
package main

// recursiveConstCompare is a rejected test case: SSA lifts lo and hi to
// constants and leaves the comparison unfolded as `1:int < 2:int`. A decider
// reads its left operand from a frame slot and has no first-constant field, so
// the recursive machine has no slot to name for a constant-versus-constant
// comparison. Select rejects it rather than silently compare frame slot 0,
// which would emit `n < 1` and take the wrong branch.
//
// Rejected at Select (no blueprint):
//
//	lo < hi has two constant operands -> Select returns
//	"comparison < between two constants is unsupported"
func recursiveConstCompare(n int) int {
	lo := 1
	hi := 2
	if lo < hi {
		if n <= 0 {
			return 0
		}
		return recursiveConstCompare(n-1) + 1
	}
	return 5
}
