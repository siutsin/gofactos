// This file validates and writes complete expected output updates.
package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// writeExpectedOutputs rewrites all validated expected output.
func writeExpectedOutputs(
	root string,
	jsonOutputs map[string][]byte,
) error {
	if err := validateExpectedOutputs(jsonOutputs); err != nil {
		return err
	}

	dir := expectedOutputDir(root)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create expected output directory: %w", err)
	}
	if err := validateExpectedOutputDirectory(dir); err != nil {
		return err
	}
	for _, c := range expectedOutputCases() {
		path := filepath.Join(dir, c.expectedOutputFilename())
		if err := os.WriteFile(path, jsonOutputs[c.name], 0o600); err != nil {
			return fmt.Errorf("write expected output %s: %w", c.name, err)
		}
	}
	return nil
}

// validateExpectedOutputDirectory rejects stale or non-regular entries.
func validateExpectedOutputDirectory(dir string) error {
	expected := expectedOutputFiles()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read expected output directory: %w", err)
	}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			return fmt.Errorf(
				"unexpected file in expected output directory: %s",
				entry.Name(),
			)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf(
				"stat expected output %s: %w",
				entry.Name(),
				err,
			)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf(
				"expected output %s is not a regular file",
				entry.Name(),
			)
		}
	}
	return nil
}

// expectedOutputFiles returns the only filenames the updater may replace.
func expectedOutputFiles() map[string]bool {
	cases := expectedOutputCases()
	expected := make(map[string]bool, len(cases))
	for _, c := range cases {
		expected[c.expectedOutputFilename()] = true
	}
	return expected
}

// validateExpectedOutputs blocks incomplete test case sets before any write.
func validateExpectedOutputs(jsonOutputs map[string][]byte) error {
	cases := expectedOutputCases()
	expected := make(map[string]bool, len(cases))
	for _, c := range cases {
		expected[c.name] = true
		data, ok := jsonOutputs[c.name]
		if !ok {
			return fmt.Errorf("missing output for %s", c.name)
		}
		if err := validateExpectedOutput(data); err != nil {
			return fmt.Errorf("validate output %s: %w", c.name, err)
		}
	}
	for name := range jsonOutputs {
		if !expected[name] {
			return fmt.Errorf("unexpected output %s", name)
		}
	}
	return nil
}

// validateExpectedOutput requires valid JSON and one trailing newline.
func validateExpectedOutput(data []byte) error {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return fmt.Errorf("expected output must end with one newline")
	}
	if len(data) > 1 && data[len(data)-2] == '\n' {
		return fmt.Errorf("expected output must end with one newline")
	}
	if !json.Valid(data[:len(data)-1]) {
		return fmt.Errorf("expected output contains invalid JSON")
	}
	return nil
}
