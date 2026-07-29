//go:build ignore

package main

func loop(n int) int {
	result := 0
	for i := 0; i < n; i++ {
		result += 2
	}
	return result
}
