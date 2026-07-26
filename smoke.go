// Package main is a throwaway smoke test for the Claude review workflow.
package main

import "os"

func add(a, b int) int {
	return a + b
}

// writeConfig writes data to path. Deliberately flawed for the review smoke test:
// the WriteFile error is ignored.
func writeConfig(path string, data []byte) {
	os.WriteFile(path, data, 0644)
}
