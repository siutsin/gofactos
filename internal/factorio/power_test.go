// This file protects complete power coverage without obscuring circuit layout.
package factorio

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAddPowerUsesOneSubstationForNearbyCombinators proves the power stage
// minimises the simple clustered case to one substation.
func TestAddPowerUsesOneSubstationForNearbyCombinators(t *testing.T) {
	t.Parallel()
	e := newEmitter()
	e.add(entity{
		Name:      "arithmetic-combinator",
		Position:  position{X: 1, Y: 1.5},
		Direction: dirEast,
	})
	e.add(entity{
		Name:      "decider-combinator",
		Position:  position{X: 4, Y: 1.5},
		Direction: dirEast,
	})

	require.NoError(t, addPower(e))
	require.NoError(t, verifyEmitted(e.entities, e.wires))
	require.NoError(t, verifyPowered(e.entities, e.wires))
	require.Equal(t, 1, countEntities(e.entities, powerPoleEntityName))
}

// TestAddPowerCoversActivityLamp proves lamps join the powered-entity set and
// receive substation coverage even when no combinator is present.
func TestAddPowerCoversActivityLamp(t *testing.T) {
	t.Parallel()
	e := newEmitter()
	e.add(entity{
		Name:     smallLampEntityName,
		Position: position{X: 0.5, Y: 0.5},
	})

	require.NoError(t, addPower(e))
	require.NoError(t, verifyEmitted(e.entities, e.wires))
	require.NoError(t, verifyPowered(e.entities, e.wires))
	require.Equal(t, 1, countEntities(e.entities, powerPoleEntityName))
}

// TestAddPowerUsesCopperOnlyBetweenSubstations proves electricity wiring stays
// separate from circuit relay poles and red/green circuit networks.
func TestAddPowerUsesCopperOnlyBetweenSubstations(t *testing.T) {
	t.Parallel()
	e := newEmitter()
	e.add(entity{
		Name:      "arithmetic-combinator",
		Position:  position{X: 1, Y: 1.5},
		Direction: dirEast,
	})
	e.add(entity{
		Name:      "decider-combinator",
		Position:  position{X: 40, Y: 1.5},
		Direction: dirEast,
	})
	relay := e.add(entity{
		Name:     relayPoleEntityName,
		Position: position{X: 20.5, Y: 1.5},
	})

	require.NoError(t, addPower(e))
	require.NoError(t, verifyEmitted(e.entities, e.wires))
	require.NoError(t, verifyPowered(e.entities, e.wires))
	require.GreaterOrEqual(t, countEntities(e.entities, powerPoleEntityName), 2)
	for _, w := range e.wires {
		require.Equal(t, connectorPoleCopper, w[1])
		require.Equal(t, connectorPoleCopper, w[3])
		require.NotEqual(t, int(relay), w[0])
		require.NotEqual(t, int(relay), w[2])
	}
}

// TestAddPowerSkipsBlueprintWithoutPoweredEntities proves the Power phase is
// a no-op for blueprints that contain only passive entities.
func TestAddPowerSkipsBlueprintWithoutPoweredEntities(t *testing.T) {
	t.Parallel()
	e := newEmitter()
	e.add(entity{Name: "constant-combinator", Position: position{X: 0, Y: 0}})
	e.add(entity{Name: "display-panel", Position: position{X: 2, Y: 0}})
	e.add(entity{Name: relayPoleEntityName, Position: position{X: 4, Y: 0}})

	require.NoError(t, addPower(e))
	require.NoError(t, verifyEmitted(e.entities, e.wires))
	require.NoError(t, verifyPowered(e.entities, e.wires))
	require.Zero(t, countEntities(e.entities, powerPoleEntityName))
	require.Empty(t, e.wires)
}

// TestVerifyPoweredRejectsInvalidNetworks proves final power verification
// fails closed for missing coverage and malformed copper networks.
func TestVerifyPoweredRejectsInvalidNetworks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(*emitter)
		want  string
	}{
		{
			name: "unpowered consumer",
			setup: func(e *emitter) {
				e.add(entity{
					Name:     arithCombinatorName,
					Position: position{X: 0.5, Y: 0.5},
				})
			},
			want: "arithmetic-combinator 1 is not powered",
		},
		{
			name: "disconnected substations",
			setup: func(e *emitter) {
				e.add(entity{
					Name:     powerPoleEntityName,
					Position: position{X: 0, Y: 0},
				})
				e.add(entity{
					Name:     powerPoleEntityName,
					Position: position{X: 10, Y: 0},
				})
			},
			want: "power substations are not connected",
		},
		{
			name: "copper span exceeds reach",
			setup: func(e *emitter) {
				left := e.add(entity{
					Name:     powerPoleEntityName,
					Position: position{X: 0, Y: 0},
				})
				right := e.add(entity{
					Name:     powerPoleEntityName,
					Position: position{X: 29, Y: 0},
				})
				e.wires = []wire{{
					int(left), connectorPoleCopper,
					int(right), connectorPoleCopper,
				}}
			},
			want: "copper wire 1-2 exceeds reach",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEmitter()
			tc.setup(e)

			err := verifyPowered(e.entities, e.wires)

			require.ErrorContains(t, err, tc.want)
		})
	}
}

// TestVerifyPowerQualityRejectsMismatches proves legendary quality stays
// confined to the substation type whose reach calculations require it.
func TestVerifyPowerQualityRejectsMismatches(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		entity entity
		want   string
	}{
		{
			name: "ordinary substation",
			entity: entity{
				EntityNumber: 1,
				Name:         powerPoleEntityName,
			},
			want: `substation 1 has quality "", want "legendary"`,
		},
		{
			name: "legendary combinator",
			entity: entity{
				EntityNumber: 1,
				Name:         arithCombinatorName,
				Quality:      powerPoleQuality,
			},
			want: `arithmetic-combinator 1 has quality "legendary", want ""`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyPowerQuality([]entity{tc.entity})

			require.ErrorContains(t, err, tc.want)
		})
	}
}

// TestPlacePowerSubstationsReportsExhaustedGrid proves placement fails with a
// clear error when no free substation tile can cover the remaining target.
func TestPlacePowerSubstationsReportsExhaustedGrid(t *testing.T) {
	t.Parallel()
	targetEntity := entity{
		EntityNumber: 1,
		Name:         "arithmetic-combinator",
		Position:     position{X: 1, Y: 1.5},
		Direction:    dirEast,
	}
	target := poweredEntity{
		num:   targetEntity.EntityNumber,
		pos:   targetEntity.Position,
		cells: entityCells(targetEntity),
	}
	uncovered := []poweredEntity{target}
	minX, minY, maxX, maxY := powerSearchBounds(uncovered)
	occupied := tileMap{}
	for x := minX; x <= maxX+1; x++ {
		for y := minY; y <= maxY+1; y++ {
			occupied[tile{X: x, Y: y}] = true
		}
	}

	_, err := placePowerSubstations(
		newEmitter(),
		occupied,
		map[int]position{target.num: target.pos},
		[]poweredEntity{target},
	)
	require.ErrorContains(t, err, "no free substation position")
}

// TestBestSubstationCellIgnoresInputOrder proves normalised target slices make
// symmetric placement ties independent of caller order.
func TestBestSubstationCellIgnoresInputOrder(t *testing.T) {
	t.Parallel()
	target := func(num, x, y int) poweredEntity {
		return poweredEntity{
			num: num,
			pos: position{
				X: float64(x) + 0.5,
				Y: float64(y) + 0.5,
			},
			cells: []tile{{X: x, Y: y}},
		}
	}
	targets := map[int]poweredEntity{
		1: target(1, 0, 0),
		2: target(2, 4, 0),
		3: target(3, 0, 4),
		4: target(4, 4, 4),
	}
	orders := [][]int{
		{1, 2, 3, 4},
		{4, 3, 2, 1},
		{2, 4, 1, 3},
		{3, 1, 4, 2},
	}

	var wantCell tile
	for i, order := range orders {
		unordered := make([]poweredEntity, 0, len(order))
		for _, num := range order {
			unordered = append(unordered, targets[num])
		}
		uncovered := sortedPoweredEntities(unordered)
		require.Equal(t, []poweredEntity{
			targets[1],
			targets[2],
			targets[3],
			targets[4],
		}, uncovered)
		cell, ok := bestSubstationCell(tileMap{}, uncovered)
		require.True(t, ok)
		if i == 0 {
			wantCell = cell
		}
		require.Equal(t, wantCell, cell)
	}
}

// TestBestSubstationCellBreaksDistanceTiesByCell proves an exact distance tie
// chooses the topmost, then leftmost, candidate.
func TestBestSubstationCellBreaksDistanceTiesByCell(t *testing.T) {
	t.Parallel()
	target := poweredEntity{
		num:   1,
		pos:   position{X: 2.5, Y: 2},
		cells: []tile{{2, 2}},
	}
	cell, ok := bestSubstationCell(
		tileMap{},
		[]poweredEntity{target},
	)
	require.True(t, ok)
	require.Equal(t, tile{X: 1, Y: 1}, cell)
}

// TestBestSubstationCellMatchesExhaustiveSearch proves spatial scoring retains
// the original coverage, distance, and grid tie-breaking behaviour.
func TestBestSubstationCellMatchesExhaustiveSearch(t *testing.T) {
	t.Parallel()
	targets := sortedPoweredEntities([]poweredEntity{
		{
			num:   1,
			pos:   position{X: 0.5, Y: 0.5},
			cells: []tile{{0, 0}},
		},
		{
			num:   2,
			pos:   position{X: 18, Y: 2.5},
			cells: []tile{{17, 2}, {18, 2}},
		},
		{
			num:   3,
			pos:   position{X: 45.5, Y: 17.5},
			cells: []tile{{45, 17}},
		},
	})
	blockers := []tile{{8, 0}, {9, 1}, {29, 8}, {43, 15}, {46, 18}}
	for mask := range 1 << len(blockers) {
		occupied := tileMap{}
		for _, target := range targets {
			for _, cell := range target.cells {
				occupied[cell] = true
			}
		}
		for i, cell := range blockers {
			if mask&(1<<i) != 0 {
				occupied[cell] = true
			}
		}

		wantCell, wantOK := exhaustiveBestSubstationCell(occupied, targets)
		gotCell, gotOK := bestSubstationCell(occupied, targets)

		require.Equalf(t, wantOK, gotOK, "mask %05b", mask)
		require.Equalf(t, wantCell, gotCell, "mask %05b", mask)
	}
}

// exhaustiveBestSubstationCell keeps the former full-grid search as a test
// oracle for the optimised production scorer.
func exhaustiveBestSubstationCell(
	occupied tileMap,
	uncovered []poweredEntity,
) (tile, bool) {
	minX, minY, maxX, maxY := powerSearchBounds(uncovered)
	best := substationCandidate{distance: math.Inf(1)}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			candidate, ok := substationCandidateAt(
				occupied,
				uncovered,
				tile{x, y},
			)
			if ok && betterSubstationCandidate(candidate, best) {
				best = candidate
			}
		}
	}
	return best.cell, best.covered > 0
}

// TestSubstationCandidateAtDoesNotAllocate protects the hot scoring loop from
// per-candidate heap allocation.
func TestSubstationCandidateAtDoesNotAllocate(t *testing.T) {
	targets := []poweredEntity{
		{
			num:   1,
			pos:   position{X: 4.5, Y: 4.5},
			cells: []tile{{X: 4, Y: 4}},
		},
		{
			num:   2,
			pos:   position{X: 10.5, Y: 10.5},
			cells: []tile{{X: 10, Y: 10}},
		},
	}
	occupied := tileMap{}
	var candidate substationCandidate
	var ok bool
	allocations := testing.AllocsPerRun(100, func() {
		candidate, ok = substationCandidateAt(
			occupied,
			targets,
			tile{X: 0, Y: 0},
		)
	})

	require.True(t, ok)
	require.Equal(t, 2, candidate.covered)
	require.Zero(t, allocations)
}

// TestNearestPowerPairIgnoresMapInsertionOrder proves an equal-distance choice
// uses entity numbers as a stable tie-breaker.
func TestNearestPowerPairIgnoresMapInsertionOrder(t *testing.T) {
	t.Parallel()
	pos := map[int]position{
		1: {X: 0, Y: 0},
		2: {X: -10, Y: 0},
		3: {X: 10, Y: 0},
	}
	for _, order := range [][]int{{2, 3}, {3, 2}} {
		remaining := make(map[int]bool, len(order))
		for _, num := range order {
			remaining[num] = true
		}
		from, to := nearestPowerPair(pos, []int{1}, remaining)
		require.Equal(t, 1, from)
		require.Equal(t, 2, to)
	}
}

// TestPowerPlacementBytesIgnoreInputOrder proves target slice order cannot
// change substation cells, entity order, wire pairs, or blueprint bytes.
func TestPowerPlacementBytesIgnoreInputOrder(t *testing.T) {
	t.Parallel()
	orders := [][]int{
		{1, 2, 3},
		{1, 3, 2},
		{2, 1, 3},
		{2, 3, 1},
		{3, 1, 2},
		{3, 2, 1},
	}

	wantEntities, wantWires, wantBytes := powerPlacementCase(t, orders[0])
	for _, order := range orders[1:] {
		entities, wires, data := powerPlacementCase(t, order)
		require.Equal(t, wantEntities, entities)
		require.Equal(t, wantWires, wires)
		require.Equal(t, wantBytes, data)
	}
}

// TestGenerateAddIsPowered proves generated blueprints now include the Power
// phase and leave every arithmetic or decider combinator inside substation
// coverage.
func TestGenerateAddIsPowered(t *testing.T) {
	t.Parallel()
	g, err := compileFunction(parseTestFile(t, "../testdata/add.go", "add"))
	require.NoError(t, err)
	require.NoError(t, verifyPowered(g.e.entities, g.e.wires))
	require.Positive(t, countEntities(g.e.entities, powerPoleEntityName))
	require.Zero(t, countEntities(g.e.entities, "big-electric-pole"))
}

// powerPlacementCase makes input-order determinism observable in both the
// entity graph and encoded blueprint.
func powerPlacementCase(
	t *testing.T,
	order []int,
) ([]entity, []wire, []byte) {
	t.Helper()
	e := newEmitter()
	for _, pos := range []position{
		{X: 1, Y: 1.5},
		{X: 41, Y: 1.5},
		{X: 1, Y: 41.5},
	} {
		e.add(entity{
			Name:      arithCombinatorName,
			Position:  pos,
			Direction: dirEast,
		})
	}

	occupied := tileMap{}
	positions := make(map[int]position, len(e.entities))
	targets := make(map[int]poweredEntity, len(e.entities))
	for _, ent := range e.entities {
		cells := entityCells(ent)
		for _, cell := range cells {
			occupied[cell] = true
		}
		positions[ent.EntityNumber] = ent.Position
		targets[ent.EntityNumber] = poweredEntity{
			num:   ent.EntityNumber,
			pos:   ent.Position,
			cells: cells,
		}
	}

	ordered := make([]poweredEntity, 0, len(order))
	for _, num := range order {
		ordered = append(ordered, targets[num])
	}
	substations, err := placePowerSubstations(
		e,
		occupied,
		positions,
		ordered,
	)
	require.NoError(t, err)
	require.NoError(
		t,
		wirePowerNetwork(e, occupied, positions, substations),
	)
	require.NoError(t, verifyEmitted(e.entities, e.wires))
	require.NoError(t, verifyPowered(e.entities, e.wires))

	bp := BlueprintWrapper{Blueprint: Blueprint{
		Item:     "blueprint",
		Version:  blueprintVersion,
		Entities: e.entities,
		Wires:    e.wires,
	}}
	data, err := json.Marshal(bp)
	require.NoError(t, err)
	return e.entities, e.wires, data
}

// countEntities keeps power assertions focused on required infrastructure.
func countEntities(entities []entity, name string) int {
	count := 0
	for _, ent := range entities {
		if ent.Name == name {
			count++
		}
	}
	return count
}
