// This file builds and verifies the electric network for active entities.
package factorio

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
)

const (
	substationSupplyReach = 14.0
	substationWireReach   = 28.0
	powerWireMargin       = 2.0
)

type poweredEntity struct {
	num   int
	pos   position
	cells []tile
}

type substationCandidate struct {
	cell     tile
	covered  int
	distance float64
}

// addPower is the Power phase that makes every active entity operable. It adds
// substations after Route, when every
// circuit relay and exact entity position is known, so the power grid can avoid
// overlaps and stay separate from red/green circuit routing.
func addPower(e *emitter) error {
	occupied := tileMap{}
	pos := make(map[int]position, len(e.entities))
	var targets []poweredEntity
	for _, ent := range e.entities {
		if _, ok := entityCapabilities[ent.Name]; !ok {
			return fmt.Errorf("unknown emitted entity %q", ent.Name)
		}
		pos[ent.EntityNumber] = ent.Position
		cells := entityCells(ent)
		for _, c := range cells {
			occupied[c] = true
		}
		if needsPower(ent) {
			targets = append(targets, poweredEntity{
				num:   ent.EntityNumber,
				pos:   ent.Position,
				cells: cells,
			})
		}
	}
	if len(targets) == 0 {
		return nil
	}

	substations, err := placePowerSubstations(e, occupied, pos, targets)
	if err != nil {
		return err
	}
	return wirePowerNetwork(e, occupied, pos, substations)
}

// placePowerSubstations greedily covers every entity that consumes electricity.
func placePowerSubstations(
	e *emitter,
	occupied tileMap,
	pos map[int]position,
	targets []poweredEntity,
) ([]int, error) {
	uncovered := sortedPoweredEntities(targets)

	var substations []int
	for len(uncovered) > 0 {
		cell, ok := bestSubstationCell(occupied, uncovered)
		if !ok {
			return nil, fmt.Errorf("no free substation position covers %d powered entities", len(uncovered))
		}
		num := addSubstation(e, occupied, pos, cell.X, cell.Y)
		substations = append(substations, num)
		uncovered = removeCoveredEntities(uncovered, substationPosition(cell.X, cell.Y))
	}
	return substations, nil
}

// sortedPoweredEntities returns a deterministic copy ordered by entity number.
func sortedPoweredEntities(targets []poweredEntity) []poweredEntity {
	ordered := slices.Clone(targets)
	slices.SortFunc(ordered, func(a, b poweredEntity) int {
		return cmp.Compare(a.num, b.num)
	})
	return ordered
}

// bestSubstationCell chooses the deterministic position with greatest coverage.
func bestSubstationCell(
	occupied tileMap,
	uncovered []poweredEntity,
) (tile, bool) {
	minBoundX, minBoundY, maxBoundX, maxBoundY := powerSearchBounds(uncovered)
	minBound := tile{minBoundX, minBoundY}
	maxBound := tile{maxBoundX, maxBoundY}
	width := maxBound.X - minBound.X + 1
	height := maxBound.Y - minBound.Y + 1
	covered := make([]int, width*height)
	distance := make([]float64, width*height)
	scoreSubstationCandidates(
		uncovered,
		minBound,
		maxBound,
		width,
		covered,
		distance,
	)

	best := substationCandidate{distance: math.Inf(1)}
	for y := minBound.Y; y <= maxBound.Y; y++ {
		for x := minBound.X; x <= maxBound.X; x++ {
			index := (y-minBound.Y)*width + x - minBound.X
			if covered[index] == 0 || !free2x2(occupied, x, y) {
				continue
			}
			candidate := substationCandidate{
				cell:     tile{x, y},
				covered:  covered[index],
				distance: distance[index],
			}
			if betterSubstationCandidate(candidate, best) {
				best = candidate
			}
		}
	}
	return best.cell, best.covered > 0
}

// scoreSubstationCandidates accumulates each target over only the cells whose
// supply square covers that target completely.
func scoreSubstationCandidates(
	uncovered []poweredEntity,
	minBound, maxBound tile,
	width int,
	covered []int,
	distance []float64,
) {
	for _, target := range uncovered {
		minX, minY := math.MaxInt, math.MaxInt
		maxX, maxY := math.MinInt, math.MinInt
		for _, cell := range target.cells {
			minX = min(minX, cell.X)
			minY = min(minY, cell.Y)
			maxX = max(maxX, cell.X)
			maxY = max(maxY, cell.Y)
		}
		firstX := int(math.Ceil(
			float64(maxX) - substationSupplyReach - 0.5,
		))
		lastX := int(math.Floor(
			float64(minX) + substationSupplyReach - 0.5,
		))
		firstY := int(math.Ceil(
			float64(maxY) - substationSupplyReach - 0.5,
		))
		lastY := int(math.Floor(
			float64(minY) + substationSupplyReach - 0.5,
		))
		for y := max(firstY, minBound.Y); y <= min(lastY, maxBound.Y); y++ {
			for x := max(firstX, minBound.X); x <= min(lastX, maxBound.X); x++ {
				index := (y-minBound.Y)*width + x - minBound.X
				covered[index]++
				distance[index] += pointDistance(
					substationPosition(x, y),
					target.pos,
				)
			}
		}
	}
}

// substationCandidateAt scores one collision-free supply position.
func substationCandidateAt(
	occupied tileMap,
	uncovered []poweredEntity,
	cell tile,
) (substationCandidate, bool) {
	if !free2x2(occupied, cell.X, cell.Y) {
		return substationCandidate{}, false
	}
	p := substationPosition(cell.X, cell.Y)
	candidate := substationCandidate{cell: cell}
	for _, target := range uncovered {
		if !substationCovers(p, target.cells) {
			continue
		}
		candidate.covered++
		candidate.distance += pointDistance(p, target.pos)
	}
	return candidate, candidate.covered > 0
}

// betterSubstationCandidate applies stable coverage, distance, and grid ties.
func betterSubstationCandidate(
	candidate substationCandidate,
	best substationCandidate,
) bool {
	if candidate.covered != best.covered {
		return candidate.covered > best.covered
	}
	if candidate.distance != best.distance {
		return candidate.distance < best.distance
	}
	if candidate.cell.Y != best.cell.Y {
		return candidate.cell.Y < best.cell.Y
	}
	return candidate.cell.X < best.cell.X
}

// removeCoveredEntities filters covered entities while preserving sort order.
func removeCoveredEntities(
	uncovered []poweredEntity,
	pos position,
) []poweredEntity {
	return slices.DeleteFunc(uncovered, func(target poweredEntity) bool {
		return substationCovers(pos, target.cells)
	})
}

// powerSearchBounds limits candidate search to cells that can cover a target.
func powerSearchBounds(uncovered []poweredEntity) (int, int, int, int) {
	minX, minY := math.MaxInt, math.MaxInt
	maxX, maxY := math.MinInt, math.MinInt
	for _, target := range uncovered {
		for _, c := range target.cells {
			minX = min(minX, c.X)
			minY = min(minY, c.Y)
			maxX = max(maxX, c.X)
			maxY = max(maxY, c.Y)
		}
	}
	reach := int(math.Ceil(substationSupplyReach))
	return minX - reach, minY - reach, maxX + reach, maxY + reach
}

// wirePowerNetwork joins placed substations into one electric network.
func wirePowerNetwork(
	e *emitter,
	occupied tileMap,
	pos map[int]position,
	substations []int,
) error {
	if len(substations) <= 1 {
		return nil
	}
	connected := []int{substations[0]}
	remaining := make(map[int]bool, len(substations)-1)
	for _, num := range substations[1:] {
		remaining[num] = true
	}
	for len(remaining) > 0 {
		from, to := nearestPowerPair(pos, connected, remaining)
		if err := connectPowerSpan(e, occupied, pos, from, to); err != nil {
			return err
		}
		connected = append(connected, to)
		delete(remaining, to)
	}
	return nil
}

// nearestPowerPair grows the power network by its closest deterministic edge.
func nearestPowerPair(
	pos map[int]position,
	connected []int,
	remaining map[int]bool,
) (int, int) {
	connected = slices.Sorted(slices.Values(connected))
	nums := slices.Sorted(maps.Keys(remaining))
	bestFrom, bestTo := 0, 0
	bestDist := math.Inf(1)
	for _, from := range connected {
		for _, to := range nums {
			d := pointDistance(pos[from], pos[to])
			sameDistance := d == bestDist
			smallerPair := from < bestFrom ||
				from == bestFrom && to < bestTo
			if d < bestDist || sameDistance && smallerPair {
				bestFrom, bestTo = from, to
				bestDist = d
			}
		}
	}
	return bestFrom, bestTo
}

// connectPowerSpan bridges one electric span within copper-wire reach.
func connectPowerSpan(
	e *emitter,
	occupied tileMap,
	pos map[int]position,
	from, to int,
) error {
	chain, err := relayChain(
		pos,
		from,
		to,
		connectorPoleCopper,
		connectorPoleCopper,
		connectorPoleCopper,
		substationWireReach,
		powerWireMargin,
		func(target, prev, next position) (int, error) {
			return placePowerRelay(
				e,
				occupied,
				pos,
				target,
				prev,
				next,
			)
		},
	)
	if err != nil {
		return err
	}
	e.wires = append(e.wires, chain...)
	return nil
}

// placePowerRelay adds a reachable intermediate substation without overlap.
func placePowerRelay(
	e *emitter,
	occupied tileMap,
	pos map[int]position,
	target, prev, next position,
) (int, error) {
	x, y, err := freeSubstationCell(
		occupied,
		int(math.Round(target.X-1)),
		int(math.Round(target.Y-1)),
		prev,
		next,
	)
	if err != nil {
		return 0, err
	}
	return addSubstation(e, occupied, pos, x, y), nil
}

// freeSubstationCell finds a relay position that preserves both adjacent spans.
func freeSubstationCell(
	occupied tileMap,
	x, y int,
	prev, next position,
) (int, int, error) {
	cell, ok := findRelayCell(x, y, func(c tile) bool {
		p := substationPosition(c.X, c.Y)
		return free2x2(occupied, c.X, c.Y) &&
			pointDistance(p, prev) <= substationWireReach &&
			pointDistance(p, next) <= substationWireReach
	})
	if ok {
		return cell.X, cell.Y, nil
	}
	return 0, 0, fmt.Errorf("no free substation tile near (%d,%d)", x, y)
}

// addSubstation emits and reserves one legendary power pole.
func addSubstation(
	e *emitter,
	occupied tileMap,
	pos map[int]position,
	x, y int,
) int {
	p := substationPosition(x, y)
	num := int(e.add(entity{
		Name:     powerPoleEntityName,
		Quality:  powerPoleQuality,
		Position: p,
	}))
	pos[num] = p
	for _, c := range entityCells(e.entities[num-1]) {
		occupied[c] = true
	}
	return num
}

// substationPosition converts a 2x2 top-left cell to its entity centre.
func substationPosition(x, y int) position {
	return position{X: float64(x) + 1, Y: float64(y) + 1}
}

// needsPower identifies generated entities that require electric coverage.
func needsPower(ent entity) bool {
	capability, ok := entityCapabilities[ent.Name]
	return ok && capability.powered
}

// substationCovers checks that a whole entity lies inside one supply square.
func substationCovers(p position, cells []tile) bool {
	for _, c := range cells {
		cell := position{X: float64(c.X) + 0.5, Y: float64(c.Y) + 0.5}
		if math.Abs(p.X-cell.X) > substationSupplyReach ||
			math.Abs(p.Y-cell.Y) > substationSupplyReach {
			return false
		}
	}
	return true
}

// free2x2 protects the full substation footprint from placement collisions.
func free2x2(occupied tileMap, x, y int) bool {
	for i := range 2 {
		for j := range 2 {
			if occupied[tile{x + i, y + j}] {
				return false
			}
		}
	}
	return true
}

// verifyPowered rejects blueprints with unpowered consumers or broken wiring.
func verifyPowered(entities []entity, wires []wire) error {
	pos := make(map[int]position, len(entities))
	names := make(map[int]string, len(entities))
	var substations []entity
	for _, ent := range entities {
		if ent.Name == "big-electric-pole" {
			return fmt.Errorf("big electric pole %d emitted", ent.EntityNumber)
		}
		if _, ok := entityCapabilities[ent.Name]; !ok {
			return fmt.Errorf("unknown emitted entity %q", ent.Name)
		}
		pos[ent.EntityNumber] = ent.Position
		names[ent.EntityNumber] = ent.Name
		if ent.Name == powerPoleEntityName {
			substations = append(substations, ent)
		}
	}
	for _, ent := range entities {
		if !needsPower(ent) || poweredBySubstation(ent, substations) {
			continue
		}
		return fmt.Errorf("%s %d is not powered", ent.Name, ent.EntityNumber)
	}
	return verifyPowerWires(pos, names, substations, wires)
}

// verifyPowerQuality enforces the reach assumptions of legendary substations.
func verifyPowerQuality(entities []entity) error {
	for _, ent := range entities {
		capability, ok := entityCapabilities[ent.Name]
		if !ok {
			return fmt.Errorf("unknown emitted entity %q", ent.Name)
		}
		want := capability.quality
		if ent.Quality != want {
			return fmt.Errorf("%s %d has quality %q, want %q",
				ent.Name, ent.EntityNumber, ent.Quality, want)
		}
	}
	return nil
}

// poweredBySubstation confirms that at least one supply area covers an entity.
func poweredBySubstation(ent entity, substations []entity) bool {
	cells := entityCells(ent)
	for _, substation := range substations {
		if substationCovers(substation.Position, cells) {
			return true
		}
	}
	return false
}

// verifyPowerWires ensures all substations form one valid copper network.
func verifyPowerWires(
	pos map[int]position,
	names map[int]string,
	substations []entity,
	wires []wire,
) error {
	adj, err := powerWireAdjacency(pos, names, substations, wires)
	if err != nil {
		return err
	}
	if len(substations) <= 1 {
		return nil
	}
	seen := reachablePowerSubstations(
		adj,
		substations[0].EntityNumber,
	)
	if len(seen) != len(substations) {
		return fmt.Errorf("power substations are not connected")
	}
	return nil
}

// powerWireAdjacency validates copper spans while building their graph.
func powerWireAdjacency(
	pos map[int]position,
	names map[int]string,
	substations []entity,
	wires []wire,
) (map[int][]int, error) {
	adj := make(map[int][]int, len(substations))
	for _, w := range wires {
		if !isCopperWire(w) {
			continue
		}
		if err := validatePowerWire(pos, names, w); err != nil {
			return nil, err
		}
		adj[w[0]] = append(adj[w[0]], w[2])
		adj[w[2]] = append(adj[w[2]], w[0])
	}
	return adj, nil
}

// validatePowerWire rejects wrong connectors, endpoints, and overlong spans.
func validatePowerWire(
	pos map[int]position,
	names map[int]string,
	w wire,
) error {
	if w[1] != connectorPoleCopper || w[3] != connectorPoleCopper {
		return fmt.Errorf(
			"copper wire %d-%d uses non-copper connector",
			w[0],
			w[2],
		)
	}
	if names[w[0]] != powerPoleEntityName ||
		names[w[2]] != powerPoleEntityName {
		return fmt.Errorf(
			"copper wire %d-%d touches non-substation",
			w[0],
			w[2],
		)
	}
	if pointDistance(pos[w[0]], pos[w[2]]) > substationWireReach {
		return fmt.Errorf(
			"copper wire %d-%d exceeds reach",
			w[0],
			w[2],
		)
	}
	return nil
}

// reachablePowerSubstations finds the connected component used by verification.
func reachablePowerSubstations(adj map[int][]int, start int) map[int]bool {
	seen := map[int]bool{start: true}
	stack := []int{start}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, next := range adj[n] {
			if seen[next] {
				continue
			}
			seen[next] = true
			stack = append(stack, next)
		}
	}
	return seen
}

// isCopperWire separates electric links from red and green circuit wiring.
func isCopperWire(w wire) bool {
	return w[1] == connectorPoleCopper || w[3] == connectorPoleCopper
}
