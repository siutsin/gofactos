// This file provides multiple roots for CLI function-selection tests.
package main

// loader is the test case for ssaloader.CollectFunctions. It
// holds two plain functions in one file so within-file discovery, the name
// filter, and source-order sorting can be tested. Both compile to blueprints
// (try `blueprint --func double`). The method and closure shapes the loader must
// also handle live in negative/methods.go, where the backend rejects them.

// double returns twice its input.
func double(n int) int {
	return n * 2
}

// triple returns three times its input.
func triple(n int) int {
	return n * 3
}
