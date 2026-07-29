// This file rejects unsigned recurrence state and results.
package main

// recurrenceUnsigned cannot preserve uint overflow in Factorio's int32 network.
func recurrenceUnsigned(n int) uint {
	previous, current := uint(0), uint(1)
	for i := 0; i < n; i++ {
		previous, current = current, previous+current
	}
	return previous
}
