// This file loads user-supplied Go files and builds the SSA consumed by
// commands.
package ssaloader

import (
	"fmt"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Load loads and type-checks the Go source files at the given paths, builds
// their SSA representation, and returns the sole requested package.
//
//	// Single file
//	pkg, err := Load("main.go")
//
//	// Multiple files in the same package
//	pkg, err := Load("add.go", "multiply.go")
func Load(patterns ...string) (*ssa.Package, error) {
	cfg := &packages.Config{
		Mode: packages.LoadSyntax,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}
	// Collect package diagnostics into the returned error rather than printing
	// them here: the loader is core code and must not write to stderr. The CLI
	// layer decides how to display the error.
	var diags []string
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, e := range pkg.Errors {
			diags = append(diags, e.Error())
		}
	})
	if len(diags) > 0 {
		return nil, fmt.Errorf("packages contain errors: %s", strings.Join(diags, "; "))
	}

	prog, ssaPkgs := ssautil.Packages(pkgs, ssa.SanityCheckFunctions)
	prog.Build()

	if len(ssaPkgs) != 1 {
		return nil, fmt.Errorf(
			"expected exactly one SSA package, got %d",
			len(ssaPkgs),
		)
	}
	if ssaPkgs[0] == nil {
		return nil, fmt.Errorf("expected a non-nil SSA package")
	}

	return ssaPkgs[0], nil
}
