// This file rejects recurrence state whose integer type is too narrow.
package main

// recurrenceNarrow cannot preserve int8 overflow in Factorio's int32 network.
func recurrenceNarrow(n int) int8 {
	previous, current := int8(0), int8(1)
	for i := 0; i < n; i++ {
		previous, current = current, previous+current
	}
	return previous
}
