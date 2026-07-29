// This file rejects a loop that returns a constant instead of carried state.
package main

// loopConst is a rejected test case: its loop result is a
// plain constant, not the loop-carried accumulator. The design seeds the result
// at 0, so a non-zero static return would be silently displayed as 0. Select
// rejects it rather than miscompile.
//
// Rejected at Select (no blueprint):
//
//	result is the constant 5, not the loop accumulator -> Select returns
//	"loop result must be a loop-carried accumulator or constant 0"
func loopConst(n int) int {
	result := 5
	for i := 0; i < n; i++ {
	}
	return result
}
