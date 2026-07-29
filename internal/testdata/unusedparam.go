// This file provides an ignored leading parameter for input pruning tests.
package main

// unusedParam exercises an unused leading parameter. a never feeds the result,
// so its constant source has no net and must be pruned in Select; left in, it
// would fail pre-emission verification because its output is unwired. b keeps
// its signature index and therefore maps to signal-B.
//
// Generated blueprint:
//
// Dataflow:
//
//	[b] -> [b + 1] -> [display]
//
// Layout:
//
//	[B = 7]  [t0 = B + c0] -> display
//	[c0 = 1]
func unusedParam(a, b int) int {
	return b + 1
}
