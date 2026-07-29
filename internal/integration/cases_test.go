// This file defines the integration blueprint case inventory.
package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
)

type parameter struct {
	name  string
	value int
}

// blueprintCase identifies one reusable blueprint test case.
type blueprintCase struct {
	name     string
	function string

	source          string
	expectedOutput  bool
	params          []parameter
	useFunctionFlag bool
}

var blueprintCases = []blueprintCase{
	{
		name:           "abs",
		function:       "abs",
		source:         "internal/testdata/abs.go",
		expectedOutput: true,
	},
	{
		name:     "abs-bound-2",
		function: "abs",
		source:   "internal/testdata/absbound2.go",
	},
	{
		name:            "absolute",
		function:        "absolute",
		source:          "internal/testdata/branchcall.go",
		useFunctionFlag: true,
	},
	{
		name:           "add",
		function:       "add",
		source:         "internal/testdata/add.go",
		expectedOutput: true,
	},
	{
		name:     "answer",
		function: "answer",
		source:   "internal/testdata/answer.go",
	},
	{
		name:     "bothpos",
		function: "bothPos",
		source:   "internal/testdata/bothpos.go",
	},
	{
		name:            "branchcall",
		function:        "branchCall",
		source:          "internal/testdata/branchcall.go",
		useFunctionFlag: true,
	},
	{
		name:     "clamp",
		function: "clamp",
		source:   "internal/testdata/clamp.go",
	},
	{
		name:     "constfirst",
		function: "constFirst",
		source:   "internal/testdata/constfirst.go",
	},
	{
		name:     "deadexpr",
		function: "deadExpr",
		source:   "internal/testdata/deadexpr.go",
	},
	{
		name:     "divzero",
		function: "divZero",
		source:   "internal/testdata/divzero.go",
	},
	{
		name:           "fib-n10",
		function:       "fib",
		source:         "internal/testdata/fib.go",
		expectedOutput: true,
		params: []parameter{
			{name: "n", value: 10},
		},
	},
	{
		name:     "forcounter",
		function: "forCounter",
		source:   "internal/testdata/forcounter.go",
	},
	{
		name:           "fori",
		function:       "forI",
		source:         "internal/testdata/fori.go",
		expectedOutput: true,
	},
	{
		name:     "forrange",
		function: "forRange",
		source:   "internal/testdata/forrange.go",
	},
	{
		name:     "greater",
		function: "greater",
		source:   "internal/testdata/greater.go",
	},
	{
		name:            "identity",
		function:        "identity",
		source:          "internal/testdata/calls.go",
		useFunctionFlag: true,
	},
	{
		name:           "iseven",
		function:       "isEven",
		source:         "internal/testdata/iseven.go",
		expectedOutput: true,
	},
	{
		name:            "loader-double",
		function:        "double",
		source:          "internal/testdata/loader.go",
		useFunctionFlag: true,
	},
	{
		name:            "loader-triple",
		function:        "triple",
		source:          "internal/testdata/loader.go",
		useFunctionFlag: true,
	},
	{
		name:     "max",
		function: "max",
		source:   "internal/testdata/max.go",
	},
	{
		name:     "pick",
		function: "pick",
		source:   "internal/testdata/pick.go",
	},
	{
		name:            "square",
		function:        "square",
		source:          "internal/testdata/calls.go",
		useFunctionFlag: true,
	},
	{
		name:            "sumidentities",
		function:        "sumIdentities",
		source:          "internal/testdata/calls.go",
		useFunctionFlag: true,
	},
	{
		name:           "sumsquares",
		function:       "sumSquares",
		source:         "internal/testdata/calls.go",
		expectedOutput: true,
		params: []parameter{
			{name: "a", value: 3},
			{name: "b", value: -4},
		},
		useFunctionFlag: true,
	},
	{
		name:     "unusedparam",
		function: "unusedParam",
		source:   "internal/testdata/unusedparam.go",
		params: []parameter{
			{name: "a", value: 99},
			{name: "b", value: 7},
		},
	},
	{
		name:           "wide",
		function:       "wide",
		source:         "internal/testdata/wide.go",
		expectedOutput: true,
	},
}

// expectedOutputCases returns cases with committed expected output.
func expectedOutputCases() []blueprintCase {
	var cases []blueprintCase
	for _, c := range blueprintCases {
		if c.expectedOutput {
			cases = append(cases, c)
		}
	}
	return cases
}

// findBlueprintCase returns the named blueprint case.
func findBlueprintCase(name string) (blueprintCase, bool) {
	for _, c := range blueprintCases {
		if c.name == name {
			return c, true
		}
	}
	return blueprintCase{}, false
}

// cliArgs builds the real blueprint command arguments for this case.
func (c blueprintCase) cliArgs(source string) []string {
	args := []string{"gofactos", "blueprint", "--json"}
	if c.useFunctionFlag {
		args = append(args, "--func", c.function)
	}
	params := slices.Clone(c.params)
	sort.Slice(params, func(i, j int) bool {
		return params[i].name < params[j].name
	})
	for _, param := range params {
		args = append(
			args,
			"--set",
			param.name+"="+strconv.Itoa(param.value),
		)
	}
	return append(args, source)
}

// sourcePath returns the source path for this case.
func (c blueprintCase) sourcePath(root string) (string, error) {
	path := filepath.Join(root, c.source)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat source %s: %w", c.name, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("source %s is a directory", c.name)
	}
	return path, nil
}

// readExpectedOutput reads this case's canonical CLI JSON stdout.
func (c blueprintCase) readExpectedOutput(root string) ([]byte, error) {
	if !c.expectedOutput {
		return nil, fmt.Errorf("case %s has no exact expected output", c.name)
	}
	data, err := os.ReadFile(filepath.Join(
		expectedOutputDir(root),
		c.expectedOutputFilename(),
	))
	if err != nil {
		return nil, fmt.Errorf("read expected output %s: %w", c.name, err)
	}
	return data, nil
}

// expectedOutputFilename derives the output filename from the case name.
func (c blueprintCase) expectedOutputFilename() string {
	return c.name + ".json"
}

// expectedOutputDir returns the committed blueprint output directory.
func expectedOutputDir(root string) string {
	return filepath.Join(root, "internal", "testdata", "blueprints")
}
