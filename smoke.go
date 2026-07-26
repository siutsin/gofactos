// Package main is a throwaway smoke test for the Claude review workflow.
package main

import "os"

// writeConfig writes data to path. Deliberately flawed: the WriteFile error is
// ignored, so callers get no signal when the write fails.
func writeConfig(path string, data []byte) {
	os.WriteFile(path, data, 0644)
}
