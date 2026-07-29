// This file protects source loading, package checks, and SSA construction.
package ssaloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoad verifies that Load loads a Go source file and produces an SSA
// package containing the expected function.
func TestLoad(t *testing.T) {
	t.Parallel()
	pkg, err := Load("../testdata/add.go")
	require.NoError(t, err)
	require.NotNil(t, pkg)

	assert.Equal(t, "main", pkg.Pkg.Name())

	member := pkg.Members["add"]
	require.NotNil(t, member)
	assert.Equal(t, "add", member.Name())
}

// TestLoadInvalidFile verifies that Load returns an error for a
// non-existent file path.
func TestLoadInvalidFile(t *testing.T) {
	t.Parallel()
	_, err := Load("/nonexistent/path/to/file.go")

	assert.ErrorContains(t, err, "packages contain errors")
}

// TestLoadSyntaxError verifies that Load returns an error when the
// source file contains syntax errors.
func TestLoadSyntaxError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "invalid.go")
	src := []byte("package main\n\nfunc broken( {\n")
	require.NoError(t, os.WriteFile(filePath, src, 0o600))

	_, err := Load(filePath)
	assert.ErrorContains(t, err, "packages contain errors")
}

// TestLoadRejectsMultiplePackages verifies that Load cannot silently choose
// one package when the caller supplies more than one.
func TestLoadRejectsMultiplePackages(t *testing.T) {
	t.Parallel()
	_, err := Load("../analyse", "../blueprint")

	assert.ErrorContains(t, err, "expected exactly one SSA package, got 2")
}
