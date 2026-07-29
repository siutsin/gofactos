// This file materialises placed netlist modules as entities and wires.
package factorio

import "fmt"

// portEdge is one logical driver-to-reader connection that Emit materialises as
// a numbered wire.
type portEdge struct {
	from *port // driver output
	to   *port // reader input
}

// handle identifies an entity the emitter has appended. It equals the
// entity's final number, so it doubles as the wire endpoint.
type handle int

// connBinding records the connector that realises a bound port.
type connBinding struct {
	h    handle
	conn int
}

// emitter accumulates the entities and wires of the whole blueprint, plus the
// binding from each public port to the connector that realises it, so Emit can
// materialise a portEdge as a wire.
type emitter struct {
	entities   []entity
	wires      []wire
	bound      map[*port]connBinding
	owner      map[int]*instance // entity number -> the module that built it
	aliases    map[string]string // signal name -> SSA name, for panel labels
	portAlias  map[*port]string  // owner port -> contextual SSA name
	consts     map[string]string // signal name -> literal value, for panel labels
	constAlias map[string]string // signal name -> cN alias for a literal constant
}

// newEmitter creates the ownership and binding indexes used during emission.
func newEmitter() *emitter {
	return &emitter{
		bound: make(map[*port]connBinding),
		owner: map[int]*instance{},
	}
}

// add gives an emitted entity its stable blueprint number and handle.
func (e *emitter) add(ent entity) handle {
	ent.EntityNumber = len(e.entities) + 1
	e.entities = append(e.entities, ent)
	return handle(ent.EntityNumber)
}

// bind lets logical port edges find their physical entity connectors.
func (e *emitter) bind(p *port, h handle, conn int) {
	e.bound[p] = connBinding{h: h, conn: conn}
}

// link records a physical connection for private wiring or a public port edge.
func (e *emitter) link(from handle, fromConn int, to handle, toConn int) {
	e.wires = append(e.wires, wire{int(from), fromConn, int(to), toConn})
}

// connectorFor maps an IR port to the connector required by Factorio.
// Single-point entities (constant combinators, poles, panels) use connectors 1
// and 2 for both sides; two-sided combinators read on 1 and 2 and write on 3
// and 4.
func connectorFor(singlePoint bool, kind portKind, colour wireColour) int {
	if singlePoint || kind == portIn {
		if colour == green {
			return connectorGreenIn
		}
		return connectorRedIn
	}
	if colour == green {
		return connectorGreenOut
	}
	return connectorRedOut
}

// netEdges exposes every logical connection for physical wire emission.
func netEdges(nets []*netlistNet) []portEdge {
	var edges []portEdge
	for _, n := range nets {
		for _, r := range n.readers {
			edges = append(edges, portEdge{from: n.driver, to: r})
		}
	}
	return edges
}

// emitNetlist materialises placed modules and logical edges as numbered
// Factorio entities and circuit wires. Edges that materialise as the same wire
// are de-duplicated, which happens when one net feeds both operand ports of a
// combinator, such as a*a.
func emitNetlist(insts []*instance, edges []portEdge) *emitter {
	e := newEmitter()
	buildInstances(e, insts)
	wirePortEdges(e, edges)
	collectSignalLabels(e, insts)
	addLabelPanels(e)
	return e
}

// buildInstances emits every module and records which module owns each entity.
func buildInstances(e *emitter, insts []*instance) {
	for _, in := range insts {
		before := len(e.entities)
		in.comp.build(e, in)
		for i := before; i < len(e.entities); i++ {
			e.owner[e.entities[i].EntityNumber] = in
		}
	}
}

// portSignal gives component builders the signal allocated to a wired port.
func portSignal(p *port) signalID {
	if p.net == nil {
		panic(
			"factorio: unwired port " + p.spec.name +
				" on " + p.inst.comp.kind(),
		)
	}
	return p.net.signal
}

// wirePortEdges realises logical edges without duplicate physical wires.
func wirePortEdges(e *emitter, edges []portEdge) {
	seen := make(map[wire]bool)
	for _, ed := range edges {
		from, ok := e.bound[ed.from]
		if !ok {
			panic(
				"factorio: unbound driver port " + ed.from.spec.name +
					" on " + ed.from.inst.comp.kind(),
			)
		}
		to, ok := e.bound[ed.to]
		if !ok {
			panic(
				"factorio: unbound reader port " + ed.to.spec.name +
					" on " + ed.to.inst.comp.kind(),
			)
		}
		w := canonWire(int(from.h), from.conn, int(to.h), to.conn)
		if seen[w] {
			continue
		}
		seen[w] = true
		e.link(from.h, from.conn, to.h, to.conn)
	}
}

// collectSignalLabels prepares source-level names for the teaching panels.
func collectSignalLabels(e *emitter, insts []*instance) {
	// Map each intermediate signal to its SSA value name and each literal source
	// to a stable cN alias plus its displayed value. Formulas use c0 while the
	// source panel shows c0 = 7.
	e.aliases = map[string]string{}
	e.portAlias = map[*port]string{}
	e.consts = map[string]string{}
	e.constAlias = map[string]string{}
	for _, in := range insts {
		for _, p := range in.ports {
			if p.ssaName != "" {
				e.portAlias[p] = p.ssaName
			}
			collectNetLabel(e, p.net)
		}
	}
}

// collectNetLabel records the most useful source name for one allocated net.
func collectNetLabel(e *emitter, net *netlistNet) {
	if net == nil {
		return
	}
	if net.ssaName != "" {
		e.aliases[net.signal.Name] = net.ssaName
	}
	if net.litLabel == "" {
		return
	}
	if _, seen := e.consts[net.signal.Name]; seen {
		return
	}
	// Literal constants are c0, c1, ... in first-seen order.
	e.consts[net.signal.Name] = net.litLabel
	e.constAlias[net.signal.Name] = fmt.Sprintf(
		"c%d",
		len(e.constAlias),
	)
}

// canonWire gives equivalent undirected connections one comparable form.
func canonWire(a, ac, b, bc int) wire {
	if a > b || (a == b && ac > bc) {
		return wire{b, bc, a, ac}
	}
	return wire{a, ac, b, bc}
}
