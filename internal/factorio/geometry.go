// This file owns spatial calculations shared by physical backend phases.
package factorio

import "math"

// entityCells derives occupied tiles from position, name, and direction the
// same way Factorio snaps an entity. Physical phases and tests share this model
// so no two entities overlap.
func entityCells(ent entity) []tile {
	px, py := ent.Position.X, ent.Position.Y
	capability, ok := entityCapabilities[ent.Name]
	if !ok {
		return nil
	}
	switch capability.shape {
	case singleTileShape:
		return []tile{{int(math.Floor(px)), int(math.Floor(py))}}
	case substationShape:
		left, top := int(math.Round(px-1)), int(math.Round(py-1))
		return []tile{
			{left, top},
			{left + 1, top},
			{left, top + 1},
			{left + 1, top + 1},
		}
	case combinatorShape:
		// Continue below with the direction-dependent combinator dimensions.
	}
	w, h := combinatorSize(ent.Direction)
	left := int(math.Round(px - float64(w)/2))
	top := int(math.Round(py - float64(h)/2))
	var ts []tile
	for i := range w {
		for j := range h {
			ts = append(ts, tile{left + i, top + j})
		}
	}
	return ts
}

// interpolate targets evenly spaced relays along a long physical span.
func interpolate(a, b position, frac float64) position {
	return position{
		X: a.X + frac*(b.X-a.X),
		Y: a.Y + frac*(b.Y-a.Y),
	}
}

// pointDistance provides the shared Euclidean measure for physical reach.
func pointDistance(a, b position) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}
