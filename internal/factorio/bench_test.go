// This file benchmarks representative end-to-end backend compilation.
package factorio

import (
	"testing"

	"github.com/siutsin/gofactos/internal/ssaloader"
)

// BenchmarkCompileRecursive measures full compilation of recursive factorial.
func BenchmarkCompileRecursive(b *testing.B) {
	pkg, err := ssaloader.Load("../testdata/recursive/factorial.go")
	if err != nil {
		b.Fatal(err)
	}
	functions := ssaloader.CollectFunctions(pkg, "factorial")
	if len(functions) != 1 {
		b.Fatalf("got %d factorial functions, want 1", len(functions))
	}
	options := []Option{WithParams(map[string]int{"n": 5})}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := Compile(functions[0], options...); err != nil {
			b.Fatal(err)
		}
	}
}
