// This file protects test case coverage and expected output mapping.
package integration

import (
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManifestExpectedOutputsOneToOne rejects missing or extra output files.
func TestManifestExpectedOutputsOneToOne(t *testing.T) {
	entries, err := os.ReadDir(expectedOutputDir(projectRoot(t)))
	require.NoError(t, err)

	actual := make([]string, 0, len(entries))
	for _, entry := range entries {
		require.False(t, entry.IsDir())
		actual = append(actual, entry.Name())
	}
	sort.Strings(actual)

	want := make([]string, 0, len(expectedOutputCases()))
	for _, c := range expectedOutputCases() {
		want = append(want, c.expectedOutputFilename())
	}
	sort.Strings(want)
	assert.Equal(t, want, actual)
}

// TestUpdateExpectedOutputsRejectsIncompleteSet validates all output before
// creating the directory or writing any file.
func TestUpdateExpectedOutputsRejectsIncompleteSet(t *testing.T) {
	root := t.TempDir()
	err := writeExpectedOutputs(root, nil)
	require.ErrorContains(t, err, "missing output")
	_, statErr := os.Stat(expectedOutputDir(root))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
