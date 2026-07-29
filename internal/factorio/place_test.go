// This file proves placement is deterministic, compact, and collision-free.
package factorio

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// emitModule builds one module anchored at cell (0, 0), adds its label panels,
// and returns the emitter, so a test can compare the tiles its entities cover
// against the footprint and check its wiring.
func emitModule(c component) *emitter {
	in := newInstance(c)
	in.dir = dirEast
	in.pos = anchorPos(in, 0, 0, dirEast)
	for _, p := range in.ports {
		p.net = &netlistNet{signal: inputSignals[0]}
	}
	e := newEmitter()
	c.build(e, in)
	for _, ent := range e.entities {
		e.owner[ent.EntityNumber] = in
	}
	addLabelPanels(e)
	return e
}

// emittedBounds returns the smallest origin-based rectangle covering entities.
func emittedBounds(t *testing.T, e *emitter) footprint {
	t.Helper()
	var bounds footprint
	for _, ent := range e.entities {
		for _, cell := range entityCells(ent) {
			require.GreaterOrEqual(t, cell.X, 0)
			require.GreaterOrEqual(t, cell.Y, 0)
			bounds.width = max(bounds.width, cell.X+1)
			bounds.height = max(bounds.height, cell.Y+1)
		}
	}
	return bounds
}

// TestFootprintMatchesEmitted proves each module reports the tight rectangular
// bound its build emits, so the placer reserves enough space without tracking
// every occupied cell.
func TestFootprintMatchesEmitted(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		comp component
	}{
		{"constSrc", newConstSrc(1)},
		{"arith", newArith("+")},
		{"neg", &neg{}},
		{"compare", newCompare("<", 0)},
		{"phi", &phi{}},
		{"register", &register{}},
		{"boolDisplay", &boolDisplay{}},
		{"clockDiv", newClockDivWithSummary(clockPeriod, "")},
		{"stopGate", &stopGate{}},
		{"warmStopGate", newStopGateWithWarmup(2)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emitted := emittedBounds(t, emitModule(tc.comp))
			require.Equal(t, tc.comp.footprint(dirEast), emitted)
		})
	}
}

// TestLayerizeDependencyDepth proves a module's layer is its longest-path depth
// over the public-net graph: sources at 0, each reader strictly right of every
// input.
func TestLayerizeDependencyDepth(t *testing.T) {
	t.Parallel()
	t.Run("add", func(t *testing.T) {
		ca := newInstance(newConstSrc(1))
		cb := newInstance(newConstSrc(1))
		ad := newInstance(newArith("+"))
		insts := []*instance{ca, cb, ad}
		nets := []*netlistNet{
			connect(ca.port("out"), ad.port("a")),
			connect(cb.port("out"), ad.port("b")),
			connect(ad.port("out")),
		}
		require.Equal(t, []int{0, 0, 1}, layerize(insts, nets))
	})

	t.Run("abs", func(t *testing.T) {
		cn := newInstance(newConstSrc(-5))
		ng := newInstance(&neg{})
		cmp := newInstance(newCompare("<", 0))
		ph := newInstance(&phi{})
		insts := []*instance{cn, ng, cmp, ph}
		nets := []*netlistNet{
			connect(cn.port("out"), ng.port("in"), cmp.port("a"), ph.port("b")),
			connect(ng.port("out"), ph.port("a")),
			connect(cmp.port("cond"), ph.port("cond")),
			connect(ph.port("out")),
		}
		require.Equal(t, []int{0, 1, 1, 2}, layerize(insts, nets))
	})
}

// TestLayerizeBreaksBackEdge proves a cyclic recurrence still layers: a depth
// pass that did not exclude the back edge would recurse forever. The two adders
// feed each other; the back edge is dropped so the depth terminates.
func TestLayerizeBreaksBackEdge(t *testing.T) {
	t.Parallel()
	ca := newInstance(newConstSrc(1))
	cb := newInstance(newConstSrc(1))
	na := newInstance(newArith("+"))
	nb := newInstance(newArith("+"))
	insts := []*instance{ca, cb, na, nb}
	nets := []*netlistNet{
		connect(ca.port("out"), na.port("a")),
		connect(nb.port("out"), na.port("b")), // na reads nb
		connect(cb.port("out"), nb.port("a")),
		connect(na.port("out"), nb.port("b")), // nb reads na: cycle
	}
	require.Equal(t, []int{0, 0, 1, 2}, layerize(insts, nets))
}

// TestLayerizeTreatsRecurrenceNextInputsAsBackEdges proves parallel state
// registers stay together while their next-state arithmetic sits to the right.
func TestLayerizeTreatsRecurrenceNextInputsAsBackEdges(t *testing.T) {
	t.Parallel()
	a := newInstance(newRegisterWithInitial(0))
	b := newInstance(newRegisterWithInitial(1))
	add := newInstance(newArith("+"))
	insts := []*instance{a, b, add}
	nets := []*netlistNet{
		connect(a.port("value"), add.port("a")),
		connect(b.port("value"), add.port("b"), a.port("next")),
		connect(add.port("out"), b.port("next")),
	}

	require.Equal(t, []int{0, 0, 1}, layerize(insts, nets))
}

// TestLayerizeTreatsScalarRegisterNextAsFeedback proves placement does not
// depend on DFS order to break a scalar register's sequential cycle.
func TestLayerizeTreatsScalarRegisterNextAsFeedback(t *testing.T) {
	t.Parallel()
	add := newInstance(newArith("+"))
	register := newInstance(&register{})
	constant := newInstance(newConstSrc(1))
	insts := []*instance{add, register, constant}
	nets := []*netlistNet{
		connect(register.port("value"), add.port("a")),
		connect(constant.port("out"), add.port("b")),
		connect(add.port("out"), register.port("next")),
	}

	require.Equal(t, []int{1, 0, 0}, layerize(insts, nets))
}

// TestConstantBoundRecurrenceRegistersShareLayer proves a bound source created
// after the stop gate cannot split the counter from the parallel state.
func TestConstantBoundRecurrenceRegistersShareLayer(t *testing.T) {
	t.Parallel()
	fn := parseLoopTestFunction(t, "", `package main
func fib10() int {
	previous, current := 0, 1
	for i := 0; i < 10; i++ {
		previous, current = current, previous+current
	}
	return previous
}
`, "fib10")
	sel, err := selectFunc(fn)
	require.NoError(t, err)
	layers := layerize(sel.insts, sel.nets)
	var registerLayers []int
	for i, in := range sel.insts {
		reg, ok := in.comp.(*register)
		if ok && reg.initial != nil {
			registerLayers = append(registerLayers, layers[i])
		}
	}
	require.Equal(t, []int{2, 2, 2}, registerLayers)
}

// placeAdd and placeAbs build the two reference netlists the placement tests
// share, returning the instances, nets, and the occupied tile map.
func placeAdd() ([]*instance, []*netlistNet, tileMap) {
	ca := newInstance(newConstSrc(3))
	cb := newInstance(newConstSrc(5))
	ad := newInstance(newArith("+"))
	insts := []*instance{ca, cb, ad}
	nets := []*netlistNet{
		connect(ca.port("out"), ad.port("a")),
		connect(cb.port("out"), ad.port("b")),
		connect(ad.port("out")),
	}
	place(insts, nets)
	return insts, nets, placedTiles(insts)
}

// placeAbs supplies a branched reference graph so placement is tested beyond a
// straight arithmetic chain.
func placeAbs() ([]*instance, []*netlistNet, tileMap) {
	cn := newInstance(newConstSrc(-5))
	ng := newInstance(&neg{})
	cmp := newInstance(newCompare("<", 0))
	ph := newInstance(&phi{})
	insts := []*instance{cn, ng, cmp, ph}
	nets := []*netlistNet{
		connect(cn.port("out"), ng.port("in"), cmp.port("a"), ph.port("b")),
		connect(ng.port("out"), ph.port("a")),
		connect(cmp.port("cond"), ph.port("cond")),
		connect(ph.port("out")),
	}
	place(insts, nets)
	return insts, nets, placedTiles(insts)
}

// placedTiles reconstructs test-only placement rectangles from module bounds.
func placedTiles(insts []*instance) tileMap {
	occupied := tileMap{}
	for _, in := range insts {
		w, h := combinatorSize(in.dir)
		if _, ok := in.comp.(*constSrc); ok {
			w, h = 1, 1
		}
		left := int(math.Round(in.pos.X - float64(w)/2))
		top := int(math.Round(in.pos.Y-float64(h)/2)) - 1
		fp := in.comp.footprint(in.dir)
		for y := range fp.height {
			for x := range fp.width {
				occupied[tile{left + x, top + y}] = true
			}
		}
	}
	return occupied
}

// sumFootprints is the total rectangular area reserved for every module.
func sumFootprints(insts []*instance) int {
	n := 0
	for _, in := range insts {
		fp := in.comp.footprint(dirEast)
		n += fp.width * fp.height
	}
	return n
}

// TestPlaceNoOverlap proves placed modules occupy disjoint tiles: the occupied
// map holds exactly one cell per footprint tile, so nothing is double-booked.
func TestPlaceNoOverlap(t *testing.T) {
	t.Parallel()
	insts, _, occ := placeAdd()
	require.Len(t, occ, sumFootprints(insts))

	insts, _, occ = placeAbs()
	require.Len(t, occ, sumFootprints(insts))
}

// TestPlaceSpacing proves the placer keeps one empty tile between dependency
// columns and between vertically stacked modules.
func TestPlaceSpacing(t *testing.T) {
	t.Parallel()
	insts, _, _ := placeAdd()
	leftTop := insts[0]
	leftBottom := insts[1]
	right := insts[2]

	require.InDelta(t, leftTop.pos.Y+3, leftBottom.pos.Y, 0)
	require.InDelta(t, leftTop.pos.X+2.5, right.pos.X, 0)
}

// TestCompositeInternalSpacing proves multi-combinator modules also keep one
// empty tile between their visible label-panel units.
func TestCompositeInternalSpacing(t *testing.T) {
	t.Parallel()
	in := newInstance(newCompare("<", 0))
	in.dir = dirEast
	in.pos = anchorPos(in, 0, 0, dirEast)

	e := newEmitter()
	in.port("a").net = &netlistNet{signal: inputSignals[0]}
	in.port("cond").net = &netlistNet{signal: inputSignals[1]}
	in.comp.build(e, in)

	require.Len(t, e.entities, 3)
	require.InDelta(t, in.pos.Y, e.entities[0].Position.Y, 0)
	require.InDelta(t, in.pos.Y+3, e.entities[1].Position.Y, 0)
	require.InDelta(t, in.pos.X+3, e.entities[2].Position.X, 0)
	require.InDelta(t, in.pos.Y+3, e.entities[2].Position.Y, 0)
}

// TestPlaceColumnsByLayer proves placement is dependency-driven: every module
// sits strictly right of each module that drives it.
func TestPlaceColumnsByLayer(t *testing.T) {
	t.Parallel()
	_, nets, _ := placeAbs()
	for _, n := range nets {
		for _, r := range n.readers {
			require.Greater(t, r.inst.pos.X, n.driver.inst.pos.X,
				"reader %s must sit right of driver %s", r.inst.comp.kind(), n.driver.inst.comp.kind())
		}
	}
}

// isAligned reports whether a coordinate is grid-legal for an entity of the
// given tile extent: an even extent centres on an integer, an odd extent on a
// half tile.
func isAligned(c float64, size int) bool {
	if size%2 == 0 {
		return c == math.Floor(c)
	}
	return c-0.5 == math.Floor(c-0.5)
}

// TestPlaceGridAlignment proves every emitted entity lands on the grid: a
// two-wide east combinator centres on an integer X, a one-tall on a half Y, and
// a constant combinator on a half tile both ways. This is the bug the row
// placer left: a two-wide combinator stranded on a half-tile X.
func TestPlaceGridAlignment(t *testing.T) {
	t.Parallel()
	check := func(t *testing.T, insts []*instance, nets []*netlistNet) {
		t.Helper()
		e := emitNetlist(insts, netEdges(nets))
		for _, ent := range e.entities {
			var w, h int
			switch ent.Name {
			case "constant-combinator", "display-panel":
				w, h = 1, 1
			default:
				w, h = combinatorSize(ent.Direction)
			}
			assert.Truef(t, isAligned(ent.Position.X, w),
				"%s X=%v not aligned for width %d", ent.Name, ent.Position.X, w)
			assert.Truef(t, isAligned(ent.Position.Y, h),
				"%s Y=%v not aligned for height %d", ent.Name, ent.Position.Y, h)
		}
	}

	t.Run("add", func(t *testing.T) {
		insts, nets, _ := placeAdd()
		check(t, insts, nets)
	})
	t.Run("abs", func(t *testing.T) {
		insts, nets, _ := placeAbs()
		check(t, insts, nets)
	})
}

// TestInitialRegisterFourColumnGeometry proves the initial source, hold, write,
// and output cells always occupy the same compact four-column unit.
func TestInitialRegisterFourColumnGeometry(t *testing.T) {
	t.Parallel()
	reg := newRegisterWithInitial(1)
	in := newInstance(reg)
	in.dir = dirEast
	in.pos = anchorPos(in, 0, 0, dirEast)
	setPortSignal(in, "next", "signal-N")
	setPortSignal(in, "pulse", "signal-P")
	setPortSignal(in, "start", "signal-S")
	setPortSignal(in, "value", "signal-R")
	e := newEmitter()
	reg.build(e, in)

	require.Len(t, e.entities, 4)
	require.Equal(t, []string{
		constCombinatorName,
		deciderCombinatorName,
		arithCombinatorName,
		arithCombinatorName,
	}, []string{
		e.entities[0].Name,
		e.entities[1].Name,
		e.entities[2].Name,
		e.entities[3].Name,
	})
	assert.Equal(t, []position{
		{X: 0.5, Y: 1.5},
		{X: 1.5, Y: 2},
		{X: 2.5, Y: 2},
		{X: 3.5, Y: 2},
	}, []position{
		e.entities[0].Position,
		e.entities[1].Position,
		e.entities[2].Position,
		e.entities[3].Position,
	})
	wantTiles := map[tile]bool{
		{X: 1, Y: 0}: true,
		{X: 0, Y: 1}: true,
		{X: 1, Y: 1}: true,
		{X: 1, Y: 2}: true,
		{X: 2, Y: 1}: true,
		{X: 2, Y: 2}: true,
		{X: 3, Y: 1}: true,
		{X: 3, Y: 2}: true,
	}
	assert.Equal(t, footprint{width: 4, height: 3}, reg.footprint(dirEast))

	for _, ent := range e.entities {
		e.owner[ent.EntityNumber] = in
	}
	addLabelPanels(e)
	emitted := map[tile]bool{}
	for _, ent := range e.entities {
		for _, cell := range entityCells(ent) {
			emitted[cell] = true
		}
	}
	assert.Equal(t, wantTiles, emitted)
	require.NoError(t, verifyEmitted(e.entities, e.wires))
}

// TestCompositeWiresWithinReach proves a composite's own wiring stays within
// circuit reach after the label panels enlarge it, so no public or internal red
// link needs a relay pole yet.
func TestCompositeWiresWithinReach(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		comp component
	}{
		{name: "compare", comp: newCompare("<", 0)},
		{name: "phi", comp: &phi{}},
		{name: "legacy register", comp: &register{}},
		{
			name: "initial register",
			comp: newRegisterWithInitial(1),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := emitModule(tc.comp)
			pos := map[int]position{}
			for _, ent := range e.entities {
				pos[ent.EntityNumber] = ent.Position
			}
			for _, w := range e.wires {
				a, b := pos[w[0]], pos[w[2]]
				d := math.Hypot(a.X-b.X, a.Y-b.Y)
				require.LessOrEqualf(t, d, wireReach, "wire %v spans %.2f tiles, over reach %.0f", w, d, wireReach)
			}
		})
	}
}

// TestRecurrenceLayerizeTerminates proves Fibonacci's cyclic state graph gets
// finite layers and a complete placement.
func TestRecurrenceLayerizeTerminates(t *testing.T) {
	t.Parallel()
	sel := selectLoopForTest(t, "../testdata/fib.go", "fib", 10)
	layers := layerize(sel.insts, sel.nets)
	require.Len(t, layers, len(sel.insts))
	maxLayer := 0
	for _, layer := range layers {
		if layer > maxLayer {
			maxLayer = layer
		}
	}
	require.Positive(t, maxLayer)

	place(sel.insts, sel.nets)
	occupied := placedTiles(sel.insts)
	require.NotEmpty(t, occupied)
}
