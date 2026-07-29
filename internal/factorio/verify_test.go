// This file protects netlist validation from accepting malformed circuits.
package factorio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVerifyNetlistPasses accepts a well-formed netlist: every input is wired
// and every net has a driver and an allocated signal.
func TestVerifyNetlistPasses(t *testing.T) {
	t.Parallel()
	cn := newInstance(newConstSrc(3))
	cb := newInstance(newConstSrc(5))
	ad := newInstance(newArith("+"))
	insts := []*instance{cn, cb, ad}
	nets := []*netlistNet{
		connect(cn.port("out"), ad.port("a")),
		connect(cb.port("out"), ad.port("b")),
		connect(ad.port("out")),
	}
	require.NoError(t, allocateSignals(nets))
	require.NoError(t, verifyNetlist(insts, nets))
}

// TestVerifyNetlistUnwiredInput rejects a module whose input reads no net, the
// signature of a Select wiring bug.
func TestVerifyNetlistUnwiredInput(t *testing.T) {
	t.Parallel()
	ad := newInstance(newArith("+")) // neither operand wired
	err := verifyNetlist([]*instance{ad}, nil)
	require.ErrorContains(t, err, "unwired")
}

// TestVerifyNetlistUnallocatedSignal rejects a net that skipped Allocate.
func TestVerifyNetlistUnallocatedSignal(t *testing.T) {
	t.Parallel()
	cn := newInstance(newConstSrc(1))
	cb := newInstance(newConstSrc(2))
	ad := newInstance(newArith("+"))
	insts := []*instance{cn, cb, ad}
	nets := []*netlistNet{
		connect(cn.port("out"), ad.port("a")),
		connect(cb.port("out"), ad.port("b")),
		connect(ad.port("out")),
	}
	err := verifyNetlist(insts, nets) // allocateSignals deliberately not run
	require.ErrorContains(t, err, "no allocated signal")
}

// TestVerifyNetlistNoDriver rejects a net with no driver.
func TestVerifyNetlistNoDriver(t *testing.T) {
	t.Parallel()
	n := &netlistNet{signal: inputSignals[0]}
	err := verifyNetlist(nil, []*netlistNet{n})
	require.ErrorContains(t, err, "driver")
}

// TestVerifyNetlistRejectsBrokenGraphClosure proves every listed port and net
// has reciprocal ownership and a unique allocated signal.
func TestVerifyNetlistRejectsBrokenGraphClosure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		mutate  func([]*instance, []*netlistNet) []*netlistNet
		wantErr string
	}{
		{
			name: "unwired output",
			mutate: func(instances []*instance, nets []*netlistNet) []*netlistNet {
				instances[2].port("out").net = nil
				return nets
			},
			wantErr: "output \"out\" is unwired",
		},
		{
			name: "unlisted net",
			mutate: func(_ []*instance, nets []*netlistNet) []*netlistNet {
				return nets[:len(nets)-1]
			},
			wantErr: "references an unlisted net",
		},
		{
			name: "wrong driver direction",
			mutate: func(instances []*instance, nets []*netlistNet) []*netlistNet {
				nets[0].driver = instances[2].port("a")
				return nets
			},
			wantErr: "wrong direction",
		},
		{
			name: "duplicate signal",
			mutate: func(_ []*instance, nets []*netlistNet) []*netlistNet {
				nets[1].signal = nets[0].signal
				return nets
			},
			wantErr: "share allocated signal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newInstance(newConstSrc(1))
			b := newInstance(newConstSrc(2))
			add := newInstance(newArith("+"))
			instances := []*instance{a, b, add}
			nets := []*netlistNet{
				connect(a.port("out"), add.port("a")),
				connect(b.port("out"), add.port("b")),
				connect(add.port("out")),
			}
			require.NoError(t, allocateSignals(nets))

			err := verifyNetlist(instances, tc.mutate(instances, nets))
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// TestVerifyEmittedNoOverlap accepts an emitted module whose entities tile
// without collision.
func TestVerifyEmittedNoOverlap(t *testing.T) {
	t.Parallel()
	require.NoError(t, verifyEmitted(emitModule(newArith("+")).entities, nil))
}

// TestVerifyEmittedOverlap rejects two entities sharing a tile.
func TestVerifyEmittedOverlap(t *testing.T) {
	t.Parallel()
	a := entity{EntityNumber: 1, Name: "constant-combinator", Position: position{X: 0, Y: 0}}
	b := entity{EntityNumber: 2, Name: "constant-combinator", Position: position{X: 0, Y: 0}}
	require.ErrorContains(t, verifyEmitted([]entity{a, b}, nil), "overlap")
}

// TestVerifyEmittedReach rejects a wire whose endpoints sit beyond the reach.
func TestVerifyEmittedReach(t *testing.T) {
	t.Parallel()
	a := entity{EntityNumber: 1, Name: "constant-combinator", Position: position{X: 0, Y: 0}}
	b := entity{EntityNumber: 2, Name: "constant-combinator", Position: position{X: 20, Y: 0}}
	w := wire{1, connectorGreenIn, 2, connectorGreenIn}
	require.ErrorContains(t, verifyEmitted([]entity{a, b}, []wire{w}), "reach")
}

// TestVerifyEmittedRejectsMalformedEndpoints proves emitted validation fails
// closed on entity identity, capability, and wire-class errors.
func TestVerifyEmittedRejectsMalformedEndpoints(t *testing.T) {
	t.Parallel()
	constant := func(number int, x float64) entity {
		return entity{
			EntityNumber: number,
			Name:         constCombinatorName,
			Position:     position{X: x, Y: 0.5},
		}
	}
	for _, tc := range []struct {
		name     string
		entities []entity
		wires    []wire
		wantErr  string
	}{
		{
			name: "non-positive ID", entities: []entity{constant(0, 0.5)},
			wantErr: "must be positive",
		},
		{
			name:     "duplicate ID",
			entities: []entity{constant(1, 0.5), constant(1, 2.5)},
			wantErr:  "duplicate entity number",
		},
		{
			name: "unknown entity",
			entities: []entity{{
				EntityNumber: 1, Name: "unknown", Position: position{X: 0.5, Y: 0.5},
			}},
			wantErr: "unknown emitted entity",
		},
		{
			name: "missing endpoint", entities: []entity{constant(1, 0.5)},
			wires:   []wire{{1, connectorRedIn, 2, connectorRedIn}},
			wantErr: "missing entity",
		},
		{
			name:     "unsupported connector",
			entities: []entity{constant(1, 0.5), constant(2, 2.5)},
			wires:    []wire{{1, connectorRedOut, 2, connectorRedIn}},
			wantErr:  "does not support connector",
		},
		{
			name:     "mixed colours",
			entities: []entity{constant(1, 0.5), constant(2, 2.5)},
			wires:    []wire{{1, connectorRedIn, 2, connectorGreenIn}},
			wantErr:  "mixes red and green",
		},
		{
			name: "mixed wire classes",
			entities: []entity{
				{
					EntityNumber: 1, Name: powerPoleEntityName,
					Position: position{X: 1, Y: 1},
				},
				constant(2, 3.5),
			},
			wires:   []wire{{1, connectorPoleCopper, 2, connectorRedIn}},
			wantErr: "mixes copper and circuit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyEmitted(tc.entities, tc.wires)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// TestVerifyColoursRejectsRedAcrossModules rejects a red wire that crosses a
// module boundary, since red is reserved for private module internals.
func TestVerifyColoursRejectsRedAcrossModules(t *testing.T) {
	t.Parallel()
	e := newEmitter()
	a := newInstance(newArith("+"))
	b := newInstance(newArith("+"))
	h1 := e.add(entity{Name: "arithmetic-combinator"})
	h2 := e.add(entity{Name: "arithmetic-combinator"})
	e.owner[int(h1)] = a
	e.owner[int(h2)] = b
	e.wires = []wire{{int(h1), connectorRedOut, int(h2), connectorRedIn}}
	require.ErrorContains(t, verifyColours(e), "red wire crosses")
}

// TestVerifyColoursRejectsUnownedRedEndpoint requires every private wire to
// remain attributable to one non-nil module owner.
func TestVerifyColoursRejectsUnownedRedEndpoint(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		setOwner bool
		owner    *instance
	}{
		{name: "missing owner"},
		{name: "nil owner", setOwner: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEmitter()
			owner := newInstance(newArith("+"))
			h1 := e.add(entity{Name: "arithmetic-combinator"})
			h2 := e.add(entity{Name: "arithmetic-combinator"})
			e.owner[int(h1)] = owner
			if tc.setOwner {
				e.owner[int(h2)] = tc.owner
			}
			e.wires = []wire{
				{int(h1), connectorRedOut, int(h2), connectorRedIn},
			}

			require.ErrorContains(
				t,
				verifyColours(e),
				"requires module owners",
			)
		})
	}
}
