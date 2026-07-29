// This file rejects malformed netlists and unusable emitted blueprints.
package factorio

import (
	"fmt"
	"slices"
)

// verifyOutput checks every invariant that depends on concrete emitted output.
func verifyOutput(e *emitter) error {
	if err := verifyEmitted(e.entities, e.wires); err != nil {
		return err
	}
	if err := verifyPowered(e.entities, e.wires); err != nil {
		return err
	}
	if err := verifyPowerQuality(e.entities); err != nil {
		return err
	}
	return verifyColours(e)
}

// verifyNetlist stops malformed allocated IR from reaching physical emission.
// It runs structural checks before placement and emission: every module port
// reads or drives a listed net, every net has a driver, and every net carries
// one unique allocated signal. It rejects malformed netlists with a Go error
// before a Select wiring bug can reach a broken blueprint.
//
// Emission-dependent colour, overlap, and reach checks run after Emit, once
// entity ownership and positions are available.
func verifyNetlist(insts []*instance, nets []*netlistNet) error {
	ports, err := verifyInstances(insts)
	if err != nil {
		return err
	}
	netSet, err := verifyNets(nets, ports)
	if err != nil {
		return err
	}
	return verifyPortBacklinks(ports, netSet)
}

// verifyInstances indexes unique, owned, and wired instance ports.
func verifyInstances(insts []*instance) (map[*port]*instance, error) {
	instances := make(map[*instance]bool, len(insts))
	ports := make(map[*port]*instance)
	for i, in := range insts {
		if in == nil {
			return nil, fmt.Errorf("instance %d is nil", i)
		}
		if instances[in] {
			return nil, fmt.Errorf("instance %d is listed more than once", i)
		}
		instances[in] = true
		if err := verifyInstancePorts(in, ports); err != nil {
			return nil, err
		}
	}
	return ports, nil
}

// verifyInstancePorts checks one instance's ownership and wiring.
func verifyInstancePorts(in *instance, ports map[*port]*instance) error {
	for _, p := range in.ports {
		if p == nil {
			return fmt.Errorf("%s has a nil port", in.comp.kind())
		}
		if owner, exists := ports[p]; exists {
			return fmt.Errorf(
				"port %q belongs to both %s and %s",
				p.spec.name,
				owner.comp.kind(),
				in.comp.kind(),
			)
		}
		if p.inst != in {
			return fmt.Errorf(
				"%s port %q has the wrong owner",
				in.comp.kind(),
				p.spec.name,
			)
		}
		ports[p] = in
		if p.net == nil {
			return fmt.Errorf(
				"%s %s %q is unwired",
				in.comp.kind(),
				portKindName(p.spec.kind),
				p.spec.name,
			)
		}
	}
	return nil
}

// verifyNets indexes unique nets, signals, drivers, and readers.
func verifyNets(
	nets []*netlistNet,
	ports map[*port]*instance,
) (map[*netlistNet]int, error) {
	netSet := make(map[*netlistNet]int, len(nets))
	signals := make(map[signalID]int, len(nets))
	for i, n := range nets {
		if n == nil {
			return nil, fmt.Errorf("net %d is nil", i)
		}
		if first, exists := netSet[n]; exists {
			return nil, fmt.Errorf("nets %d and %d are the same net", first, i)
		}
		netSet[n] = i
		if n.driver == nil {
			return nil, fmt.Errorf("net %d has no driver", i)
		}
		if n.signal == (signalID{}) {
			return nil, fmt.Errorf("net %d has no allocated signal", i)
		}
		if first, exists := signals[n.signal]; exists {
			return nil, fmt.Errorf(
				"nets %d and %d share allocated signal %s",
				first,
				i,
				n.signal.Name,
			)
		}
		signals[n.signal] = i
		if err := verifyNetPort(n.driver, n, portOut, ports); err != nil {
			return nil, fmt.Errorf("net %d driver: %w", i, err)
		}
		if err := verifyNetReaders(i, n, ports); err != nil {
			return nil, err
		}
	}
	return netSet, nil
}

// verifyNetReaders checks unique input endpoints for one net.
func verifyNetReaders(
	index int,
	n *netlistNet,
	ports map[*port]*instance,
) error {
	seen := make(map[*port]bool, len(n.readers))
	for _, reader := range n.readers {
		if seen[reader] {
			return fmt.Errorf("net %d lists a reader more than once", index)
		}
		seen[reader] = true
		if err := verifyNetPort(reader, n, portIn, ports); err != nil {
			return fmt.Errorf("net %d reader: %w", index, err)
		}
	}
	return nil
}

// verifyPortBacklinks ensures every materialised port names a listed net.
func verifyPortBacklinks(
	ports map[*port]*instance,
	netSet map[*netlistNet]int,
) error {
	for p, in := range ports {
		index, exists := netSet[p.net]
		if !exists {
			return fmt.Errorf(
				"%s port %q references an unlisted net",
				in.comp.kind(),
				p.spec.name,
			)
		}
		if p.spec.kind == portOut && p.net.driver != p {
			return fmt.Errorf("net %d does not name its output driver", index)
		}
		if p.spec.kind == portIn && !slices.Contains(p.net.readers, p) {
			return fmt.Errorf("net %d omits input reader %q", index, p.spec.name)
		}
	}
	return nil
}

// portKindName gives malformed-netlist errors a stable human-readable side.
func portKindName(kind portKind) string {
	if kind == portOut {
		return "output"
	}
	return "input"
}

// verifyNetPort checks one net endpoint's direction, ownership, and backlink.
func verifyNetPort(
	p *port,
	n *netlistNet,
	want portKind,
	ports map[*port]*instance,
) error {
	if p == nil {
		return fmt.Errorf("port is nil")
	}
	if _, ok := ports[p]; !ok {
		return fmt.Errorf("port belongs to an unlisted instance")
	}
	if p.spec.kind != want {
		return fmt.Errorf("port %q has the wrong direction", p.spec.name)
	}
	if p.net != n {
		return fmt.Errorf("port %q points to another net", p.spec.name)
	}
	return nil
}

// verifyEmitted prevents placement and reach defects from reaching Factorio.
// It checks that no two entities
// occupy the same tile, and no wire exceeds the circuit reach (Route inserts
// relays to keep spans connectable). It is the post-Emit half of Verify,
// complementing the netlist-level verifyNetlist. Wire colour by module
// ownership
// is checked separately by verifyColours, which needs the emitter's owner map.
// Decider copy-count is deliberately not checked: a phi gate legitimately
// copies
// its input count to pass the selected value, so there is no fixed invariant.
func verifyEmitted(entities []entity, wires []wire) error {
	pos, capabilities, err := indexEmittedEntities(entities)
	if err != nil {
		return err
	}
	for _, w := range wires {
		if err := verifyEmittedWire(w, pos, capabilities); err != nil {
			return err
		}
	}
	return nil
}

// indexEmittedEntities validates identities, capabilities, and occupancy.
func indexEmittedEntities(
	entities []entity,
) (map[int]position, map[int]entityCapability, error) {
	occupied := map[tile]int{}
	pos := make(map[int]position, len(entities))
	capabilities := make(map[int]entityCapability, len(entities))
	for _, ent := range entities {
		if ent.EntityNumber <= 0 {
			return nil, nil, fmt.Errorf(
				"entity number %d must be positive",
				ent.EntityNumber,
			)
		}
		if _, exists := pos[ent.EntityNumber]; exists {
			return nil, nil, fmt.Errorf(
				"duplicate entity number %d",
				ent.EntityNumber,
			)
		}
		capability, ok := entityCapabilities[ent.Name]
		if !ok {
			return nil, nil, fmt.Errorf("unknown emitted entity %q", ent.Name)
		}
		pos[ent.EntityNumber] = ent.Position
		capabilities[ent.EntityNumber] = capability
		for _, c := range entityCells(ent) {
			if other, ok := occupied[c]; ok {
				return nil, nil, fmt.Errorf(
					"entities %d and %d overlap at %v",
					other,
					ent.EntityNumber,
					c,
				)
			}
			occupied[c] = ent.EntityNumber
		}
	}
	return pos, capabilities, nil
}

// verifyEmittedWire checks endpoints, connector classes, colour, and reach.
func verifyEmittedWire(
	w wire,
	pos map[int]position,
	capabilities map[int]entityCapability,
) error {
	from, fromExists := pos[w[0]]
	to, toExists := pos[w[2]]
	if !fromExists || !toExists {
		return fmt.Errorf(
			"wire %d-%d references a missing entity",
			w[0],
			w[2],
		)
	}
	if !capabilities[w[0]].supportsConnector(w[1]) {
		return fmt.Errorf(
			"entity %d does not support connector %d",
			w[0],
			w[1],
		)
	}
	if !capabilities[w[2]].supportsConnector(w[3]) {
		return fmt.Errorf(
			"entity %d does not support connector %d",
			w[2],
			w[3],
		)
	}
	// Copper reach belongs to verifyPowerWires; this verifier owns class only.
	fromCopper := w[1] == connectorPoleCopper
	toCopper := w[3] == connectorPoleCopper
	if fromCopper != toCopper {
		return fmt.Errorf(
			"wire %d-%d mixes copper and circuit connectors",
			w[0],
			w[2],
		)
	}
	if fromCopper {
		return nil
	}
	if isRedConnector(w[1]) != isRedConnector(w[3]) {
		return fmt.Errorf(
			"wire %d-%d mixes red and green connectors",
			w[0],
			w[2],
		)
	}
	if distance := pointDistance(from, to); distance > wireReach {
		return fmt.Errorf(
			"wire %d-%d exceeds reach: %.1f > %.0f",
			w[0],
			w[2],
			distance,
			wireReach,
		)
	}
	return nil
}

// verifyColours protects module isolation by rejecting cross-module red wires.
// A red wire
// never crosses a module boundary, since red is a module's private internal
// network. It needs the emitter's owner map, so it runs on the emitter. Green
// may cross modules (the inter-module bus) or stay inside one (a module joining
// its own green output, like compare's cond). Both red endpoints must name the
// same non-nil module owner.
func verifyColours(e *emitter) error {
	for _, w := range e.wires {
		// A wire is red if either endpoint is a red connector, so a wire red on
		// one side and green on the other cannot evade the check.
		if !isRedConnector(w[1]) && !isRedConnector(w[3]) {
			continue
		}
		from, ok1 := e.owner[w[0]]
		to, ok2 := e.owner[w[2]]
		if !ok1 || from == nil || !ok2 || to == nil {
			return fmt.Errorf(
				"red wire %d-%d requires module owners",
				w[0],
				w[2],
			)
		}
		if from != to {
			return fmt.Errorf(
				"red wire crosses modules %s and %s",
				from.comp.kind(),
				to.comp.kind(),
			)
		}
	}
	return nil
}
