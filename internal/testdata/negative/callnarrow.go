// This file rejects ordinary calls whose scalar types are too narrow.
package main

// incrementInt8 overflows at 127 under Go's int8 semantics. Factorio's signed
// 32-bit arithmetic would instead produce 128, so its signature is rejected.
func incrementInt8(n int8) int8 {
	return n + 1
}

// callNarrow exposes the unsupported narrow signature at a call boundary.
func callNarrow() int {
	return int(incrementInt8(127))
}
