// This file rejects counted loops with unsupported initial state.
package main

// forInit is a rejected test case: its loop accumulator starts at a non-zero
// value. The scalar-loop contract deliberately starts accumulators at zero.
// Initialised recurrence registers can represent non-zero values, but widening
// the scalar contract would change existing compatibility behaviour.
//
// Rejected at Select (no blueprint):
//
//	result starts at 10 -> Select returns "loop accumulator must start at 0"
func forInit(n int) int {
	result := 10
	for i := 0; i < n; i++ {
		result += 2
	}
	return result
}
