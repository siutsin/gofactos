// This file generates E2E blueprints through the CLI boundary.
package e2e

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type blueprintCase struct {
	encoded string
}

// generateCase ensures E2E inputs come from the built CLI under test.
func generateCase(
	t *testing.T,
	paths e2ePaths,
	relativePath string,
	function string,
	n int,
	fast bool,
) blueprintCase {
	t.Helper()
	return generateCaseWithSets(
		t,
		paths,
		relativePath,
		function,
		fast,
		"n="+strconv.Itoa(n),
	)
}

// generateCaseWithSets generates one blueprint with deterministic CLI inputs.
func generateCaseWithSets(
	t *testing.T,
	paths e2ePaths,
	relativePath string,
	function string,
	fast bool,
	sets ...string,
) blueprintCase {
	t.Helper()
	source := filepath.Join(
		paths.root,
		"internal/testdata",
		relativePath,
	)
	args := []string{
		"blueprint",
		"--func", function,
	}
	for _, set := range sets {
		args = append(args, "--set", set)
	}
	if fast {
		args = append(args, "--fast")
	}
	args = append(args, source)
	encoded := runGofactos(t, paths.gofactos, paths.root, args)
	return blueprintCase{
		encoded: strings.TrimSpace(encoded),
	}
}

// runGofactos keeps test case generation at the executable boundary.
func runGofactos(
	t *testing.T,
	binary, root string,
	args []string,
) string {
	t.Helper()
	command := exec.CommandContext( //nolint:gosec // Explicit local E2E binary.
		t.Context(),
		binary,
		args...,
	)
	command.Dir = root
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	require.NoError(t, err, stderr.String())
	return stdout.String()
}
