// This file provides the main iterative Fibonacci recurrence test case.
package main

// fib exercises parallel additive recurrence lowering.
//
// Dataflow:
//
//	[START] -> [clock] -> [i < n] -> [i + 1]
//	                                [previous <- current]
//	                                [current <- previous + current]
//	                                [display previous]
//
// Layout (`blueprint --json --set n=10`, label-panel coordinates):
//
//	y=0.5: x=1.5 [clock], x=3.5 [START], x=5.5 [A=10]
//	       x=11 [run while t2 < A], x=15.5 [t0=φ(0,t1)]
//	       x=19.5 [t4=t0+t1]
//	y=3.5: x=5.5 [c0=1], x=19.5 [t5=t2+c0]
//	y=4.5: x=15.5 [t1=φ(1,t4)]
//	y=8.5: x=15.5 [t2=φ(0,t5)]
//	y=10.5: x=19.5..26.5 [eight digit panels] -> display previous
func fib(n int) int {
	previous, current := 0, 1
	for i := 0; i < n; i++ {
		previous, current = current, previous+current
	}
	return previous
}
