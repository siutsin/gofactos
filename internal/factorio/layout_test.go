// This file checks that deferred recursive-machine nets wire within reach.
package factorio

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// addPositionedEntity places one entity at an explicit coordinate so a net's
// deferred wiring can be exercised against a known geometry.
func addPositionedEntity(e *emitter, name string, x, y float64) handle {
	return e.add(entity{Name: name, Position: position{X: x, Y: y}})
}

// TestRecursiveNetWiresWideActionWithinReach reproduces a branch action whose
// activity lamp sits one row (1.5 tiles) below a wide predicate row. A greedy
// nearest-neighbour chain walks the row first because the nearest predicate
// (1 tile) beats the lamp (1.5 tiles), then strands the lamp on a final
// sqrt(9^2+1.5^2) = 9.12-tile hop past the 9-tile reach. A minimum spanning
// tree connects the lamp straight to the near predicate instead, so every red
// link stays connectable.
func TestRecursiveNetWiresWideActionWithinReach(t *testing.T) {
	t.Parallel()
	b := &recursiveMachineBuilder{e: newEmitter()}
	net := b.net(red)

	decider := addPositionedEntity(b.e, deciderCombinatorName, 0.5, 8.0)
	lamp := addPositionedEntity(b.e, smallLampEntityName, 0.5, 9.5)
	net.add(decider, connectorRedOut)
	net.add(lamp, connectorRedIn)
	// Command endpoints span the action row; the farthest sits 9 tiles from the
	// lamp, so the lamp-to-corner span is sqrt(9^2+1.5^2) = 9.12, over reach.
	for col := 1; col <= 9; col++ {
		h := addPositionedEntity(b.e, arithCombinatorName, float64(col)+0.5, 8.0)
		net.add(h, connectorRedIn)
	}

	net.wire()

	require.Len(t, b.e.wires, len(net.endpoints)-1)
	for _, w := range b.e.wires {
		from := b.e.entities[w[0]-1].Position
		to := b.e.entities[w[2]-1].Position
		d := math.Hypot(from.X-to.X, from.Y-to.Y)
		require.LessOrEqualf(t, d, wireReach,
			"wire %v spans %.2f tiles, over reach %.0f", w, d, wireReach)
	}

	// Every endpoint must settle into one connected network.
	sim := newSim(b.e.wires)
	want := sim.network(int(decider), connectorRedOut)
	for _, ep := range net.endpoints {
		require.Equal(t, want, sim.network(int(ep.h), ep.conn),
			"endpoint %d/%d left off the network", ep.h, ep.conn)
	}
}

// TestRecursiveRedNetPanicsWhenUnroutable keeps the reach guard: when even the
// bottleneck-optimal tree cannot connect two red endpoints, the build fails
// loudly rather than emitting an unconnectable private network.
func TestRecursiveRedNetPanicsWhenUnroutable(t *testing.T) {
	t.Parallel()
	b := &recursiveMachineBuilder{e: newEmitter()}
	net := b.net(red)

	near := addPositionedEntity(b.e, deciderCombinatorName, 0.5, 0.5)
	far := addPositionedEntity(b.e, arithCombinatorName, wireReach*2+0.5, 0.5)
	net.add(near, connectorRedOut)
	net.add(far, connectorRedIn)

	require.Panics(t, func() { net.wire() })
}
