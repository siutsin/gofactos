// This file keeps emitted circuit wires within Factorio's connection reach.
package factorio

import (
	"fmt"
	"math"
)

// relayMargin keeps each routed hop comfortably under the wire reach so a small
// nudge off an occupied tile cannot push a hop past the limit.
const (
	relayMargin       = 2.0
	relaySearchRadius = 200
)

// insertRelays makes every emitted over-reach green span connectable in
// Factorio. It bridges any wire longer than the circuit reach with dedicated
// medium-pole relays, rewiring through them so every hop stays connectable. It
// runs after Emit, because only then are the exact connector entity positions
// known; estimating spans from module anchors mislocates ports on tall
// composite modules. A pole is a passthrough junction that keeps both ends on
// one network, carrying no logic. Like label panels, relays are emitted as
// plain entities rather than netlist modules.
func insertRelays(e *emitter) error {
	occupied := tileMap{}
	pos := make(map[int]position, len(e.entities))
	for _, ent := range e.entities {
		pos[ent.EntityNumber] = ent.Position
		for _, c := range entityCells(ent) {
			occupied[c] = true
		}
	}
	kept := make([]wire, 0, len(e.wires))
	for _, w := range e.wires {
		a, b := pos[w[0]], pos[w[2]]
		// Relay only green spans. Bridging a private red network through
		// green-connector poles would split it; leave red spans for verifyEmitted
		// to reject if they exceed reach.
		if isRedConnector(w[1]) || isRedConnector(w[3]) || pointDistance(a, b) <= wireReach {
			kept = append(kept, w)
			continue
		}
		chain, err := bridge(e, occupied, pos, w)
		if err != nil {
			return fmt.Errorf("bridge wire %d-%d: %w", w[0], w[2], err)
		}
		kept = append(kept, chain...)
	}
	e.wires = kept
	return nil
}

// bridge replaces one over-reach wire with a connectable relay chain.
func bridge(e *emitter, occupied tileMap, pos map[int]position, w wire) ([]wire, error) {
	return relayChain(
		pos,
		w[0],
		w[2],
		w[1],
		w[3],
		connectorGreenIn,
		wireReach,
		relayMargin,
		func(target, prev, next position) (int, error) {
			return placePole(
				e,
				occupied,
				pos,
				target.X,
				target.Y,
				prev,
				next,
			)
		},
	)
}

// relayChain builds deterministic, reach-preserving links for Route and Power.
func relayChain(
	pos map[int]position,
	from, to int,
	fromConnector, toConnector, relayConnector int,
	reach, margin float64,
	placeRelay func(position, position, position) (int, error),
) ([]wire, error) {
	a, b := pos[from], pos[to]
	distance := pointDistance(a, b)
	if distance <= reach {
		return []wire{{from, fromConnector, to, toConnector}}, nil
	}
	hops := int(math.Ceil(distance / (reach - margin)))
	chain := make([]wire, 0, max(1, hops))
	previous, previousConnector := from, fromConnector
	for i := 1; i < hops; i++ {
		target := interpolate(a, b, float64(i)/float64(hops))
		next := b
		if i+1 < hops {
			next = interpolate(a, b, float64(i+1)/float64(hops))
		}
		number, err := placeRelay(target, pos[previous], next)
		if err != nil {
			return nil, err
		}
		chain = append(chain, wire{
			previous,
			previousConnector,
			number,
			relayConnector,
		})
		previous, previousConnector = number, relayConnector
	}
	return append(chain, wire{previous, previousConnector, to, toConnector}), nil
}

// placePole adds one relay without colliding with existing entities.
func placePole(
	e *emitter,
	occupied tileMap,
	pos map[int]position,
	px, py float64,
	prev, next position,
) (int, error) {
	bx, by, err := freeRelayCell(
		occupied,
		int(math.Round(px-0.5)),
		int(math.Round(py-0.5)),
		prev,
		next,
	)
	if err != nil {
		return 0, err
	}
	occupied[tile{bx, by}] = true
	p := position{X: float64(bx) + 0.5, Y: float64(by) + 0.5}
	num := int(e.add(entity{Name: relayPoleEntityName, Position: p}))
	pos[num] = p
	return num, nil
}

// freeRelayCell finds a collision-free relay position that preserves reach.
func freeRelayCell(
	occupied tileMap,
	x, y int,
	prev, next position,
) (int, int, error) {
	cell, ok := findRelayCell(x, y, func(c tile) bool {
		p := position{X: float64(c.X) + 0.5, Y: float64(c.Y) + 0.5}
		return !occupied[c] && relayReachable(p, prev, next)
	})
	if ok {
		return cell.X, cell.Y, nil
	}
	return 0, 0, fmt.Errorf("no dedicated reachable relay tile near (%d,%d)", x, y)
}

// findRelayCell applies one placement policy over a shared outward search.
// Each perimeter keeps row-major order: top edge, side pairs, then bottom edge.
func findRelayCell(x, y int, usable func(tile) bool) (tile, bool) {
	for radius := 0; radius <= relaySearchRadius; radius++ {
		if cell, ok := findRelayOnPerimeter(x, y, radius, usable); ok {
			return cell, true
		}
	}
	return tile{}, false
}

// findRelayOnPerimeter checks one square without visiting its interior.
func findRelayOnPerimeter(
	x, y, radius int,
	usable func(tile) bool,
) (tile, bool) {
	if radius == 0 {
		cell := tile{x, y}
		return cell, usable(cell)
	}
	if cell, ok := findRelayOnRow(
		x-radius,
		x+radius,
		y-radius,
		usable,
	); ok {
		return cell, true
	}
	for row := y - radius + 1; row < y+radius; row++ {
		left := tile{x - radius, row}
		if usable(left) {
			return left, true
		}
		right := tile{x + radius, row}
		if usable(right) {
			return right, true
		}
	}
	return findRelayOnRow(x-radius, x+radius, y+radius, usable)
}

// findRelayOnRow checks one horizontal perimeter edge from left to right.
func findRelayOnRow(
	minX, maxX, y int,
	usable func(tile) bool,
) (tile, bool) {
	for x := minX; x <= maxX; x++ {
		cell := tile{x, y}
		if usable(cell) {
			return cell, true
		}
	}
	return tile{}, false
}

// relayReachable protects both neighbouring hops when a relay is nudged.
func relayReachable(p, prev, next position) bool {
	return pointDistance(prev, p) <= wireReach &&
		pointDistance(next, p) <= wireReach
}
