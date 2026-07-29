// This file proves long circuit nets gain relays without changing connectivity.
package factorio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInsertRelaysUsesDedicatedMediumPoles proves that two long wires with the
// same route get separate relay entities rather than sharing an existing pole.
func TestInsertRelaysUsesDedicatedMediumPoles(t *testing.T) {
	t.Parallel()
	e := newEmitter()
	a := e.add(entity{
		Name:     "constant-combinator",
		Position: position{X: 0.5, Y: 0.5},
	})
	b := e.add(entity{
		Name:     "constant-combinator",
		Position: position{X: 20.5, Y: 0.5},
	})
	e.wires = []wire{
		{int(a), connectorGreenIn, int(b), connectorGreenIn},
		{int(a), connectorGreenIn, int(b), connectorGreenIn},
	}

	require.NoError(t, insertRelays(e))
	require.NoError(t, verifyEmitted(e.entities, e.wires))

	relayPositions := map[position]bool{}
	for _, ent := range e.entities {
		require.NotEqual(t, "big-electric-pole", ent.Name)
		if ent.Name != relayPoleEntityName {
			continue
		}
		require.False(t, relayPositions[ent.Position])
		relayPositions[ent.Position] = true
	}
	require.Len(t, relayPositions, 4)
}

// TestFindRelayCellPreservesSearchOrder pins deterministic row-major perimeter
// traversal while the search avoids allocating candidate slices.
func TestFindRelayCellPreservesSearchOrder(t *testing.T) {
	t.Parallel()
	target := tile{X: 1, Y: 1}
	var visited []tile

	cell, ok := findRelayCell(0, 0, func(candidate tile) bool {
		visited = append(visited, candidate)
		return candidate == target
	})

	require.True(t, ok)
	require.Equal(t, target, cell)
	require.Equal(t, []tile{
		{X: 0, Y: 0},
		{X: -1, Y: -1},
		{X: 0, Y: -1},
		{X: 1, Y: -1},
		{X: -1, Y: 0},
		{X: 1, Y: 0},
		{X: -1, Y: 1},
		{X: 0, Y: 1},
		{X: 1, Y: 1},
	}, visited)
}

// TestFreeRelayCellRejectsReuse proves route placement fails when every
// reachable tile is already occupied instead of returning a used cell.
func TestFreeRelayCellRejectsReuse(t *testing.T) {
	t.Parallel()
	occupied := tileMap{}
	for x := -int(wireReach); x <= int(wireReach); x++ {
		for y := -int(wireReach); y <= int(wireReach); y++ {
			occupied[tile{x, y}] = true
		}
	}

	_, _, err := freeRelayCell(
		occupied,
		0,
		0,
		position{X: 0.5, Y: 0.5},
		position{X: 0.5, Y: 0.5},
	)
	require.ErrorContains(t, err, "no dedicated")
}

// TestFreeRelayCellPreservesBothSpans proves a blocked ideal cell skips
// candidates that can reach only one neighbouring endpoint.
func TestFreeRelayCellPreservesBothSpans(t *testing.T) {
	t.Parallel()
	occupied := tileMap{{X: 0, Y: 0}: true}
	previous := position{X: -8.5, Y: 0.5}
	next := position{X: 8.5, Y: 0.5}

	x, y, err := freeRelayCell(occupied, 0, 0, previous, next)

	require.NoError(t, err)
	require.Equal(t, -1, x)
	require.Zero(t, y)
	require.True(t, relayReachable(
		position{X: float64(x) + 0.5, Y: float64(y) + 0.5},
		previous,
		next,
	))
}
