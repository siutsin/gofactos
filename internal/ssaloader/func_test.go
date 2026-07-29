// This file protects function collection and filtering at the SSA boundary.
package ssaloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// parseSource writes source to a temporary file and builds its SSA package.
func parseSource(t *testing.T, source string) *ssa.Package {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.go")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	pkg, err := Load(path)
	require.NoError(t, err)
	return pkg
}

// TestCollectFunctionsAll verifies that CollectFunctions returns every
// non-synthetic, non-init function across the parsed files. Source-position
// order across separate files is not guaranteed, so this checks membership;
// the within-file ordering guarantee is covered by
// TestCollectFunctionsMethodsAndClosuresSorted.
func TestCollectFunctionsAll(t *testing.T) {
	t.Parallel()
	pkg, err := Load("../testdata/abs.go", "../testdata/iseven.go")
	require.NoError(t, err)

	functions := CollectFunctions(pkg, "")

	names := make([]string, len(functions))
	for i, fn := range functions {
		names[i] = fn.Name()
	}
	assert.ElementsMatch(t, []string{"isEven", "abs"}, names)
}

// TestCollectFunctionsFilter verifies that CollectFunctions returns
// only the function matching the given name.
func TestCollectFunctionsFilter(t *testing.T) {
	t.Parallel()
	pkg, err := Load("../testdata/abs.go", "../testdata/iseven.go")
	require.NoError(t, err)

	functions := CollectFunctions(pkg, "abs")

	require.Len(t, functions, 1)
	assert.Equal(t, "abs", functions[0].Name())
}

// TestCollectFunctionsFilterNoMatch verifies that CollectFunctions
// returns an empty slice when no function matches the given name.
func TestCollectFunctionsFilterNoMatch(t *testing.T) {
	t.Parallel()
	pkg, err := Load("../testdata/abs.go", "../testdata/iseven.go")
	require.NoError(t, err)

	functions := CollectFunctions(pkg, "nonexistent")

	assert.Empty(t, functions)
}

// TestCollectFunctionsSorted verifies that CollectFunctions returns the
// plain top-level functions in one file in source position order.
func TestCollectFunctionsSorted(t *testing.T) {
	t.Parallel()
	pkg, err := Load("../testdata/loader.go")
	require.NoError(t, err)

	functions := CollectFunctions(pkg, "")
	names := make([]string, len(functions))
	for i, fn := range functions {
		names[i] = fn.Name()
	}

	assert.Equal(t, []string{"double", "triple"}, names)
}

// TestCollectFunctionsMethodFilter verifies that the name filter
// works for method names.
func TestCollectFunctionsMethodFilter(t *testing.T) {
	t.Parallel()
	pkg, err := Load("../testdata/negative/methods.go")
	require.NoError(t, err)

	functions := CollectFunctions(pkg, "Increment")

	require.Len(t, functions, 1)
	assert.Equal(t, "Increment", functions[0].Name())
}

// TestCollectFunctionsMethodsAndClosuresSorted verifies that
// CollectFunctions returns methods, top-level functions, and closures
// in source position order.
func TestCollectFunctionsMethodsAndClosuresSorted(t *testing.T) {
	t.Parallel()
	pkg, err := Load("../testdata/negative/methods.go")
	require.NoError(t, err)

	functions := CollectFunctions(pkg, "")
	names := make([]string, len(functions))
	for i, fn := range functions {
		names[i] = fn.Name()
	}

	expected := []string{"Increment", "Value", "MakeAdder", "MakeAdder$1"}
	assert.Equal(t, expected, names)
}

// TestCollectFunctionsFilterFindsClosure verifies that filtering a parent
// function does not prevent discovery of a matching nested closure.
func TestCollectFunctionsFilterFindsClosure(t *testing.T) {
	t.Parallel()
	pkg, err := Load("../testdata/negative/methods.go")
	require.NoError(t, err)

	functions := CollectFunctions(pkg, "MakeAdder$1")

	require.Len(t, functions, 1)
	assert.Equal(t, "MakeAdder$1", functions[0].Name())
}

// TestCollectFunctionsFindsDescendantClosures verifies that closure discovery
// descends recursively and can filter a closure nested more than one level.
func TestCollectFunctionsFindsDescendantClosures(t *testing.T) {
	t.Parallel()
	pkg := parseSource(t, `package main

func outer() func() func() int {
	return func() func() int {
		return func() int { return 42 }
	}
}
`)

	functions := CollectFunctions(pkg, "")
	names := make([]string, len(functions))
	for i, fn := range functions {
		names[i] = fn.Name()
	}
	assert.Equal(t, []string{"outer", "outer$1", "outer$1$1"}, names)

	functions = CollectFunctions(pkg, "outer$1$1")
	require.Len(t, functions, 1)
	assert.Equal(t, "outer$1$1", functions[0].Name())
}

// TestCollectFunctionsDistinguishesInitialisers verifies that package
// initialisers are excluded without excluding a legal method named init.
func TestCollectFunctionsDistinguishesInitialisers(t *testing.T) {
	t.Parallel()
	pkg := parseSource(t, `package main

type counter int

func (counter) init() {}

func init() {
	hidden := func() {}
	hidden()
}

func keep() {}
`)

	functions := CollectFunctions(pkg, "")
	names := make([]string, len(functions))
	for i, fn := range functions {
		names[i] = fn.Name()
	}

	assert.ElementsMatch(t, []string{"init", "keep"}, names)
}

// TestCollectFunctionsDeduplicatesAliasMethods verifies that an alias does
// not cause its original type's methods to be returned twice.
func TestCollectFunctionsDeduplicatesAliasMethods(t *testing.T) {
	t.Parallel()
	pkg := parseSource(t, `package main

type original int

func (original) Value() int { return 1 }

type alias = original
`)

	functions := CollectFunctions(pkg, "Value")

	require.Len(t, functions, 1)
	assert.Equal(t, "Value", functions[0].Name())
}

// TestCollectFunctionsIncludesGenericReceiverMethods proves discovery uses
// method declarations even when SSA cannot build an uninstantiated selection.
func TestCollectFunctionsIncludesGenericReceiverMethods(t *testing.T) {
	t.Parallel()
	pkg := parseSource(t, `package main

type box[T any] struct {
	value T
}

func (b box[T]) Value() T { return b.value }

func (b *box[T]) Set(value T) { b.value = value }
`)

	functions := CollectFunctions(pkg, "")
	names := make([]string, len(functions))
	for i, fn := range functions {
		names[i] = fn.Name()
	}

	assert.Equal(t, []string{"Value", "Set"}, names)
}
