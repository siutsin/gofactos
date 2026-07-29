// This file tests the complete expected output update contract.
package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateExpectedOutputsRejectsUnexpectedOutput proves validation comes
// before writes.
func TestUpdateExpectedOutputsRejectsUnexpectedOutput(t *testing.T) {
	root := t.TempDir()
	outputs := testBlueprintOutputs(t)
	outputs["unexpected"] = []byte("{}\n")

	err := writeExpectedOutputs(root, outputs)

	require.ErrorContains(t, err, "unexpected output")
	_, statErr := os.Stat(expectedOutputDir(root))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// TestUpdateExpectedOutputsRejectsInvalidJSON keeps malformed bytes from
// replacing an existing contract.
func TestUpdateExpectedOutputsRejectsInvalidJSON(t *testing.T) {
	root := t.TempDir()
	dir := seedExpectedOutputDirectory(t, root)
	first := expectedOutputCases()[0]
	//nolint:gosec // The path is below a test-owned temporary directory.
	before, err := os.ReadFile(
		filepath.Join(dir, first.expectedOutputFilename()),
	)
	require.NoError(t, err)
	outputs := testBlueprintOutputs(t)
	outputs[first.name] = []byte("not JSON\n")

	err = writeExpectedOutputs(root, outputs)

	require.ErrorContains(t, err, "expected output contains invalid JSON")
	//nolint:gosec // The path is below a test-owned temporary directory.
	after, readErr := os.ReadFile(
		filepath.Join(dir, first.expectedOutputFilename()),
	)
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
}

// TestUpdateExpectedOutputsRejectsUnexpectedEntry rejects stale files.
func TestUpdateExpectedOutputsRejectsUnexpectedEntry(t *testing.T) {
	root := t.TempDir()
	dir := seedExpectedOutputDirectory(t, root)
	stale := filepath.Join(dir, "stale.json")
	require.NoError(t, os.WriteFile(stale, []byte("stale"), 0o600))
	outputs := testBlueprintOutputs(t)

	err := writeExpectedOutputs(root, outputs)

	require.ErrorContains(
		t,
		err,
		"unexpected file in expected output directory: stale.json",
	)
	//nolint:gosec // The path is below a test-owned temporary directory.
	data, readErr := os.ReadFile(stale)
	require.NoError(t, readErr)
	assert.Equal(t, []byte("stale"), data)
}

// TestUpdateExpectedOutputsRejectsNonRegularEntry rejects directories and
// symbolic links in place of expected files.
func TestUpdateExpectedOutputsRejectsNonRegularEntry(t *testing.T) {
	root := t.TempDir()
	dir := expectedOutputDir(root)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.Mkdir(
		filepath.Join(dir, expectedOutputCases()[0].expectedOutputFilename()),
		0o750,
	))
	outputs := testBlueprintOutputs(t)

	err := writeExpectedOutputs(root, outputs)

	require.ErrorContains(t, err, "is not a regular file")
}

// TestUpdateExpectedOutputsWritesCompleteSet checks the full JSON replacement.
func TestUpdateExpectedOutputsWritesCompleteSet(t *testing.T) {
	root := t.TempDir()
	dir := seedExpectedOutputDirectory(t, root)
	outputs := testBlueprintOutputs(t)

	require.NoError(t, writeExpectedOutputs(root, outputs))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	actualNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		actualNames = append(actualNames, entry.Name())
	}
	assert.ElementsMatch(t, expectedOutputFilenames(), actualNames)
	for _, c := range expectedOutputCases() {
		//nolint:gosec // The path is below a test-owned temporary directory.
		data, readErr := os.ReadFile(
			filepath.Join(dir, c.expectedOutputFilename()),
		)
		require.NoError(t, readErr)
		assert.Equal(t, outputs[c.name], data)
	}
}

// TestValidateExpectedOutput checks JSON and the trailing-newline contract.
func TestValidateExpectedOutput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    string
		wantErr string
	}{
		{name: "valid", data: "{}\n"},
		{name: "missing newline", data: "{}", wantErr: "one newline"},
		{name: "double newline", data: "{}\n\n", wantErr: "one newline"},
		{name: "invalid JSON", data: "{\n", wantErr: "invalid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExpectedOutput([]byte(tc.data))
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// seedExpectedOutputDirectory creates one old file and leaves the rest absent.
func seedExpectedOutputDirectory(t *testing.T, root string) string {
	t.Helper()
	dir := expectedOutputDir(root)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, expectedOutputCases()[0].expectedOutputFilename()),
		[]byte("{\"old\":true}\n"),
		0o600,
	))
	return dir
}

// testBlueprintOutputs returns valid JSON output for every selected case.
func testBlueprintOutputs(t *testing.T) map[string][]byte {
	t.Helper()
	cases := expectedOutputCases()
	jsonOutputs := make(map[string][]byte, len(cases))
	for _, c := range cases {
		jsonOutput, err := json.MarshalIndent(map[string]any{
			"blueprint": map[string]any{
				"item":    "blueprint",
				"label":   c.name,
				"version": 1,
			},
		}, "", "  ")
		require.NoError(t, err)
		jsonOutputs[c.name] = append(jsonOutput, '\n')
	}
	return jsonOutputs
}

// expectedOutputFilenames returns every expected output filename.
func expectedOutputFilenames() []string {
	cases := expectedOutputCases()
	names := make([]string, 0, len(cases))
	for _, c := range cases {
		names = append(names, c.expectedOutputFilename())
	}
	return names
}
