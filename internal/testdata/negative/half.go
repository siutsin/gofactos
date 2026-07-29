// This file rejects unsupported floating-point arithmetic and results.
package main

// half is a rejected test case for non-integer types. gofactos lowers only
// integer and boolean values. A float64 signature returns a clear "unsupported
// parameter type" error rather than panicking in constant handling.
//
// Rejected at Select (no blueprint):
//
//	float64 param -> Select returns "unsupported parameter type float64"
func half(x float64) float64 {
	return x / 2
}
