// This file provides Factorio-compatible division-by-zero coverage.
package main

// divZero exercises Factorio's division-by-zero behaviour. The divisor is
// computed at runtime so the Go type checker accepts the source.
//
// Generated blueprint:
//
// Dataflow:
//
//	[a] --+-> [a - a] --+
//	      |             +-> [a / (a - a)] -> [display]
//	      +-------------+
//
// Layout:
//
//	[A = 1] [t0 = A - A] [t1 = A / t0] -> display
func divZero(a int) int {
	return a / (a - a)
}
