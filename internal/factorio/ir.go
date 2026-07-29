// This file defines the abstract netlist IR used to build blueprints.
package factorio

// wireColour records a port's wiring convention. Green is required at public
// module boundaries and may also carry isolated private buses inside a
// composite; red remains module-private.
type wireColour int

const (
	green wireColour = iota
	red
)

// Module-local private signals carry a composite module's isolated internal
// nets on either colour. They sit outside the two public signal banks, so
// Allocate never assigns them. Ownership isolation, rather than colour alone,
// lets the same name recur in every module without collision.
var (
	privateData = signalID{
		Type: "virtual",
		Name: "signal-dot",
	} // merged value / register cell / loop count
	privateTmp = signalID{
		Type: "virtual",
		Name: "signal-info",
	} // scratch marker / tap result
	privateInc = signalID{
		Type: "virtual",
		Name: "signal-check",
	} // loop next-index / register next value
)

// portKind marks a port as a module input or output.
type portKind int

const (
	portIn portKind = iota
	portOut
)

// portSpec is the frozen declaration of a module's port: its name, kind, and
// the colour it sits on. A component returns its full portSpec set so callers
// wire against names that exist.
type portSpec struct {
	name   string
	kind   portKind
	colour wireColour
}

// port is a materialised connection point on one instance. Obtain it via
// inst.port(name); it knows its owning net once connected. ssaName is the SSA
// value this port produces (t0, t1, ...) when the port is an SSA value's
// producer, so panel labels can show the dump's identifier; it is empty for
// parameters, literals, and synthesised modules. litLabel is the display form
// of a literal constant source (`7`, `true`), used on its cN source panel; it
// is
// empty for everything else.
type port struct {
	inst     *instance
	spec     portSpec
	net      *netlistNet
	ssaName  string
	litLabel string
}

// netlistNet is a green inter-module net: one signal, shared by the ports on
// it. A net has exactly one driver and one or more readers. (Internal red
// wiring inside a composite module is handled by the emitter, not by nets.)
//
// isInput marks a net that carries a function parameter; inputIndex is its
// position in the signature (signal-A is 0). Allocate maps inputs to the letter
// bank by index and every other (intermediate) net to the item bank, so a
// parameter's readout lines up with its place in the signature. The zero value
// is an intermediate net, so only parameter nets need tagging.
type netlistNet struct {
	driver     *port
	readers    []*port
	signal     signalID // assigned by allocateSignals
	isInput    bool
	inputIndex int
	ssaName    string // SSA value name (t0, t1, ...) driving this net, if any
	litLabel   string // display form of a literal source (`7`, `true`), if any
}

// connect gives one producer and its consumers a shared public net identity.
func connect(driver *port, readers ...*port) *netlistNet {
	n := &netlistNet{driver: driver, readers: readers}
	driver.net = n
	for _, r := range readers {
		r.net = n
	}
	return n
}

// instance is a placed module: its behaviour, orientation, position, and
// materialised ports.
type instance struct {
	comp  component
	dir   int
	pos   position
	ports []*port
}

// port gives component internals their declared connection point by name.
// It is a must-accessor:
// component internals: an unknown name means a build method no longer matches
// its own frozen portSpec table, not bad user input. Component build tests
// guard
// this invariant so the panic fails during development rather than in a user
// blueprint path.
func (in *instance) port(name string) *port {
	for _, p := range in.ports {
		if p.spec.name == name {
			return p
		}
	}
	panic("factorio: unknown port " + name + " on " + in.comp.kind())
}

// component is module behaviour: declare ports, report its placement bounds,
// and expand into entities. A leaf component is one combinator; a composite
// emits several private entities wired together directly.
type component interface {
	kind() string
	ports() []portSpec
	footprint(dir int) footprint
	build(e *emitter, self *instance)
}

// dirEast is the Factorio direction the placer orients combinators. Facing
// east, a combinator is two tiles wide and one tall. dirWest is its mirror, the
// other two-wide horizontal facing.
const (
	dirEast = 4
	dirWest = 12
)

const (
	// wireReach is the circuit-wire reach limit in tiles. A link longer than
	// this needs a relay pole. Red links inside a module must stay within it,
	// since the module owns its own private wiring.
	wireReach = 9.0

	relayPoleEntityName = "medium-electric-pole"
	powerPoleEntityName = "substation"
	powerPoleQuality    = "legendary"

	// Entity names drive string dispatch (needsPower, entityCells,
	// addLabelPanels, verify), so they are named constants to keep the build
	// sites and the dispatch sites in sync.
	arithCombinatorName   = "arithmetic-combinator"
	deciderCombinatorName = "decider-combinator"
	constCombinatorName   = "constant-combinator"
	displayPanelName      = "display-panel"
	smallLampEntityName   = "small-lamp"
)

// tile is one cell of the Factorio build grid, identified by integer column and
// row. Route, Power, and output verification derive occupied cells from emitted
// entities.
type tile struct{ X, Y int }

// tileMap is the set of grid cells a placement occupies.
type tileMap map[tile]bool

// footprint is the rectangular placement bound reserved for one module.
type footprint struct {
	width  int
	height int
}

// footprintPart is one combinator's contribution to a module's footprint: the
// cell of its top-left tile from the module anchor, and whether it is a
// single-tile constant combinator rather than a two-tile combinator. Its label
// panel sits one row above, at (dx, dy-1), so dy is at least 1.
type footprintPart struct {
	dx, dy   int
	constant bool
}

// combinatorSize supplies the oriented dimensions used by placement and
// overlap checks: two wide and one tall facing east or west, one wide and two
// tall facing north or south.
func combinatorSize(dir int) (w, h int) {
	if dir == dirEast || dir == dirWest {
		return 2, 1
	}
	return 1, 2 // north or south
}

// expandFootprint bounds combinators and their labels as one placement unit.
// Coordinates are relative to the anchor at (0, 0). Each part includes a label
// one row above its top-left tile, so dy is at least one and the bound starts at
// row zero.
func expandFootprint(dir int, parts ...footprintPart) footprint {
	w, h := combinatorSize(dir)
	var out footprint
	for _, p := range parts {
		pw, ph := w, h
		if p.constant {
			pw, ph = 1, 1
		}
		out.width = max(out.width, p.dx+pw)
		out.height = max(out.height, p.dy+ph)
	}
	return out
}

// newInstance materialises a component so the selector can wire its ports.
func newInstance(c component) *instance {
	in := &instance{comp: c}
	for _, spec := range c.ports() {
		in.ports = append(in.ports, &port{inst: in, spec: spec})
	}
	return in
}
