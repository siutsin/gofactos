// This file tests materialisation of netlists as working entities and wires.
package factorio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAddComputes builds the netlist for add(a, b) by hand, lowers it to
// entities and wires, and asserts the simulator settles the result net to
// a + b. This exercises the redesign's IR, emitter, components, and the
// simulator oracle end to end on the simplest function.
func TestAddComputes(t *testing.T) {
	t.Parallel()
	ca := newInstance(newConstSrc(3))
	cb := newInstance(newConstSrc(5))
	ad := newInstance(newArith("+"))
	insts := []*instance{ca, cb, ad}

	nets := []*netlistNet{
		connect(ca.port("out"), ad.port("a")),
		connect(cb.port("out"), ad.port("b")),
		connect(ad.port("out")), // result, read by the test
	}
	result := nets[2]
	require.NoError(t, allocateSignals(nets))

	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))

	s := simulate(t, e.entities, e.wires, 50)
	out := e.bound[ad.port("out")]
	require.Equal(t, 8, s.value(int(out.h), out.conn, result.signal.Name))
}

// TestChainedArith proves multi-stage inter-module dataflow: (a+b)*c flows
// through two arithmetic modules in series, so the simulator must settle the
// adder before the multiplier reads it. This de-risks the public wiring and the
// simulator's multi-tick settling ahead of the merge.
func TestChainedArith(t *testing.T) {
	t.Parallel()
	ca := newInstance(newConstSrc(3))
	cb := newInstance(newConstSrc(5))
	cc := newInstance(newConstSrc(2))
	sum := newInstance(newArith("+"))
	mul := newInstance(newArith("*"))
	insts := []*instance{ca, cb, cc, sum, mul}

	nets := []*netlistNet{
		connect(ca.port("out"), sum.port("a")),
		connect(cb.port("out"), sum.port("b")),
		connect(sum.port("out"), mul.port("a")),
		connect(cc.port("out"), mul.port("b")),
		connect(mul.port("out")), // result
	}
	result := nets[4]
	require.NoError(t, allocateSignals(nets))

	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))

	s := simulate(t, e.entities, e.wires, 50)
	out := e.bound[mul.port("out")]
	require.Equal(t, (3+5)*2, s.value(int(out.h), out.conn, result.signal.Name))
}

// absResult builds the netlist for abs(n) = n < 0 ? -n : n, lowers it, and
// returns the simulator's settled result. It wires the merge keystone:
// constSrc -> {neg, compare, phi}, with compare's 1/-1 selecting the branch.
func absResult(t *testing.T, n int) int {
	t.Helper()
	cn := newInstance(newConstSrc(n))
	ng := newInstance(&neg{})
	cmp := newInstance(newCompare("<", 0))
	ph := newInstance(&phi{})
	insts := []*instance{cn, ng, cmp, ph}

	nets := []*netlistNet{
		connect(cn.port("out"), ng.port("in"), cmp.port("a"), ph.port("b")),
		connect(ng.port("out"), ph.port("a")),
		connect(cmp.port("cond"), ph.port("cond")),
		connect(ph.port("out")), // result
	}
	result := nets[3]
	require.NoError(t, allocateSignals(nets))

	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))

	s := simulate(t, e.entities, e.wires, 50)
	out := e.bound[ph.port("out")]
	return s.value(int(out.h), out.conn, result.signal.Name)
}

// TestAbsComputes proves the merge end to end: the live branch wins, the dead
// branch stays silent, and the shared sum settles to the winner for both signs.
func TestAbsComputes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		n    int
		want int
	}{
		{name: "negative", n: -5, want: 5},
		{name: "positive", n: 7, want: 7},
		{name: "zero", n: 0, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, absResult(t, tc.n))
		})
	}
}
