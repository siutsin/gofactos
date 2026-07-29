// This file rejects method roots and closure factories found by the loader.
package main

// methods is a rejected test case for the function loader. CollectFunctions
// discovers a struct's methods and a closure here, but the backend rejects each
// at Select, so none produces a blueprint. It pins both the loader's method and
// closure discovery and Select's rejection of those shapes.

// Counter is a simple counter used to expose methods.
type Counter struct {
	value int
}

// Increment is a pointer-receiver method, rejected at Select.
func (c *Counter) Increment() {
	c.value++
}

// Value is a value-receiver method, rejected at Select.
func (c Counter) Value() int {
	return c.value
}

// MakeAdder returns a closure (discovered as MakeAdder$1), rejected at Select.
func MakeAdder(n int) func(int) int {
	return func(x int) int {
		return x + n
	}
}
