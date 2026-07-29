// This file protects complete and filtered SSA dump output.
package analyse

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siutsin/gofactos/internal/ssaloader"
)

// TestDump verifies that Dump writes both the package summary and function
// disassembly in the built-in ssadump format.
func TestDump(t *testing.T) {
	pkg, err := ssaloader.Load("../testdata/add.go")
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, Dump(&buf, pkg, ""))

	output := buf.String()
	assert.Contains(t, output, "func  add")
	assert.Contains(t, output, "# Name:")
	assert.Contains(t, output, "t0 = a + b")
	assert.Contains(t, output, "return t0")
}

// TestDumpFilterByFuncName verifies that Dump only outputs the
// named function when funcFilter is provided.
func TestDumpFilterByFuncName(t *testing.T) {
	pkg, err := ssaloader.Load("../testdata/abs.go", "../testdata/iseven.go")
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, Dump(&buf, pkg, "abs"))

	output := buf.String()
	assert.Contains(t, output, "# Name: command-line-arguments.abs")
	assert.NotContains(t, output, "# Name: command-line-arguments.isEven")
}
