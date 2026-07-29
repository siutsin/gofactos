// This file places netlist modules in a readable left-to-right dataflow.
package factorio

// Place is the placement phase. It lays modules in dependency layers over the
// public-net graph: a module sits one column right of every module it reads, so
// dataflow runs left to right and rotated combinators land on the grid. See
// docs/backend.md ("Backend Phases") for the design rationale.

// placeGapX and placeGapY are the extra empty tiles the placer leaves between
// dependency columns and stacked modules. One keeps a visible gap while leaving
// the blueprint compact.
const (
	placeGapX = 1
	placeGapY = 1
)

// layerize assigns dependency depths so dataflow can read from left to right.
// A module's layer is its longest-path depth over the public module graph: a
// source with no producer is layer 0, and every other module is one past the
// deepest module driving its inputs. Short sequential feedback and remaining
// DFS back edges are excluded so cyclic state lays out deterministically.
func layerize(insts []*instance, nets []*netlistNet) []int {
	index := make(map[*instance]int, len(insts))
	for i, in := range insts {
		index[in] = i
	}
	adj := moduleGraph(nets, index, len(insts))
	return longestPathLayers(forwardPreds(adj))
}

// moduleGraph exposes producer-to-consumer dependencies for placement.
func moduleGraph(
	nets []*netlistNet,
	index map[*instance]int,
	count int,
) [][]int {
	// The module dataflow graph: an edge from each net's driver module to every
	// reader module.
	adj := make([][]int, count)
	for _, net := range nets {
		if net.driver == nil {
			continue
		}
		from, ok := index[net.driver.inst]
		if !ok {
			continue
		}
		for _, r := range net.readers {
			if isRecurrenceFeedback(net.driver, r) {
				continue
			}
			to, ok := index[r.inst]
			if !ok || to == from {
				continue
			}
			adj[from] = append(adj[from], to)
		}
	}
	return adj
}

// isRecurrenceFeedback identifies short sequential feedback excluded from
// dependency layering: register next-state edges and register indexes feeding
// stop gates.
func isRecurrenceFeedback(driver, reader *port) bool {
	if _, ok := reader.inst.comp.(*register); ok {
		return reader.spec.name == "next"
	}
	if reader.spec.name != "index" {
		return false
	}
	if _, ok := reader.inst.comp.(*stopGate); !ok {
		return false
	}
	_, ok := driver.inst.comp.(*register)
	return ok
}

// longestPathLayers assigns the earliest column that honours all predecessors.
func longestPathLayers(preds [][]int) []int {
	// Longest path from the sources: layer(u) = 1 + max(layer(pred)), and 0 with
	// no forward predecessor. Memoised; the forward graph is acyclic, so it
	// terminates.
	layer := make([]int, len(preds))
	done := make([]bool, len(preds))
	for u := range preds {
		layerDepth(u, preds, layer, done)
	}
	return layer
}

// layerDepth memoises one module's longest predecessor path.
func layerDepth(
	u int,
	preds [][]int,
	layer []int,
	done []bool,
) int {
	if done[u] {
		return layer[u]
	}
	// The forward graph is a DAG, so this guard is never hit mid-cycle.
	done[u] = true
	best := 0
	for _, p := range preds[u] {
		best = max(best, layerDepth(p, preds, layer, done)+1)
	}
	layer[u] = best
	return best
}

// forwardPreds removes DFS back edges so cyclic state can still be layered.
// It returns the predecessor lists of the acyclic graph obtained by
// dropping every depth-first back edge from adj, so longest-path layering over
// a recurrence terminates. An edge into a node still on the recursion stack
// closes a cycle and is excluded.
func forwardPreds(adj [][]int) [][]int {
	const (
		white = iota
		grey
		black
	)
	n := len(adj)
	colour := make([]int, n)
	type edge struct{ from, to int }
	back := map[edge]bool{}
	var mark func(u int)
	mark = func(u int) {
		colour[u] = grey
		for _, v := range adj[u] {
			switch colour[v] {
			case grey:
				back[edge{u, v}] = true
			case white:
				mark(v)
			}
		}
		colour[u] = black
	}
	for u := range n {
		if colour[u] == white {
			mark(u)
		}
	}

	preds := make([][]int, n)
	for u := range n {
		for _, v := range adj[u] {
			if back[edge{u, v}] {
				continue
			}
			preds[v] = append(preds[v], u)
		}
	}
	return preds
}

// place gives every module non-overlapping bounding coordinates for emission.
// Shared timing infrastructure is pinned left of programme dataflow;
// unclocked programmes keep column 0.
func place(insts []*instance, nets []*netlistNet) {
	logic := insts
	startX := 0
	if clock := clockInstance(insts); clock != nil {
		startX = placeClock(clock)
		logic = excluding(insts, clock)
	}
	placeLayers(logic, nets, startX)
}

// placeClock keeps shared timing infrastructure outside the program dataflow.
func placeClock(clock *instance) int {
	clock.dir = dirEast
	clock.pos = anchorPos(clock, 0, 0, clock.dir)
	fp := clock.comp.footprint(dirEast)
	return fp.width + placeGapX
}

// placeLayers turns dependency depths into non-overlapping module coordinates.
// Each layer is a
// column on X with its modules stacked down Y; the column step is the widest
// footprint in the layer just placed plus the gap.
func placeLayers(insts []*instance, nets []*netlistNet, startX int) {
	layers := layerize(insts, nets)
	maxLayer := 0
	for _, l := range layers {
		if l > maxLayer {
			maxLayer = l
		}
	}

	colX := startX
	for layer := 0; layer <= maxLayer; layer++ {
		rowY := 0
		layerWidth := 0
		for i, in := range insts {
			if layers[i] != layer {
				continue
			}
			fp := in.comp.footprint(dirEast)
			in.dir = dirEast
			in.pos = anchorPos(in, colX, rowY, in.dir)
			rowY += fp.height + placeGapY
			layerWidth = max(layerWidth, fp.width)
		}
		colX += layerWidth + placeGapX
	}
}

// clockInstance finds timing infrastructure that placement treats specially.
func clockInstance(insts []*instance) *instance {
	for _, in := range insts {
		if _, ok := in.comp.(*clockDiv); ok {
			return in
		}
	}
	return nil
}

// excluding removes infrastructure from the logic placement input.
func excluding(insts []*instance, drop *instance) []*instance {
	out := make([]*instance, 0, len(insts))
	for _, in := range insts {
		if in != drop {
			out = append(out, in)
		}
	}
	return out
}

// anchorPos converts a footprint's top-left cell into the anchor passed to
// build.
func anchorPos(in *instance, cellX, cellY, dir int) position {
	w, h := combinatorSize(dir)
	if _, ok := in.comp.(*constSrc); ok {
		w, h = 1, 1
	}
	return position{
		X: float64(cellX) + float64(w)/2,
		Y: float64(cellY+1) + float64(h)/2,
	}
}
