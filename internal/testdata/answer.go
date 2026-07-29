// This file provides a constant-result test case for source-free data flow.
package main

// answer tests a constant-only function with no parameters: a single constSrc
// feeds the numeric readout directly, the simplest possible blueprint.
//
// Generated blueprint:
//
// Dataflow:
//
//	[42] -> [display]
//
// Layout:
//
//	[c0 = 42] -> display
func answer() int {
	return 42
}
