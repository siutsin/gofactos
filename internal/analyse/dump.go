// This file renders loaded SSA in the form used for inspection and teaching.
package analyse

import (
	"fmt"
	"io"

	"golang.org/x/tools/go/ssa"

	"github.com/siutsin/gofactos/internal/ssaloader"
)

// Dump writes the raw SSA representation of pkg using the built-in ssadump
// format from golang.org/x/tools/go/ssa. It prints the package summary
// followed by the full disassembly of each function. If funcFilter is
// non-empty, only the named function is dumped.
//
//	package command-line-arguments:
//	  func  fibonacci  func(n int) int
//
//	# Name: command-line-arguments.fibonacci
//	# Package: command-line-arguments
//	func fibonacci(n int) int:
//	0:                                                entry P:0 S:2
//	    t0 = n <= 1:int                                    bool
//	    if t0 goto 1 else 2
//	1:                                       if.then P:1 S:0 idom:0
//	    return n
//	2:                                       if.done P:1 S:0 idom:0
//	    t1 = n - 1:int                                      int
//	    t2 = fibonacci(t1)                                  int
//	    t3 = n - 2:int                                      int
//	    t4 = fibonacci(t3)                                  int
//	    t5 = t2 + t4                                        int
//	    return t5
func Dump(w io.Writer, pkg *ssa.Package, funcFilter string) error {
	fns := ssaloader.CollectFunctions(pkg, funcFilter)
	if funcFilter != "" && len(fns) == 0 {
		return fmt.Errorf("function %q not found", funcFilter)
	}

	if _, err := pkg.WriteTo(w); err != nil {
		return fmt.Errorf("analyse: write package summary: %w", err)
	}

	for _, fn := range fns {
		// Blank line to visually separate each function's disassembly.
		if _, err := fmt.Fprintln(w); err != nil {
			return fmt.Errorf("analyse: write separator: %w", err)
		}
		// Writes the function disassembly, e.g. "# Name: ...\nfunc fibonacci(n int) int:\n0: ...".
		if _, err := fn.WriteTo(w); err != nil {
			return fmt.Errorf("analyse: write function dump: %w", err)
		}
	}
	return nil
}
