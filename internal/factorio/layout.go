// This file places and wires recursive machine entities.
package factorio

type recursiveMachineCell struct {
	hold, write, output handle
}

// cellAt builds one resettable memory cell behind the shared write protocol.
func (b *recursiveMachineBuilder) cellAt(
	p recursiveMachinePoint,
	data *recursiveMachineNet,
	value signalID,
) recursiveMachineCell {
	zero := 0
	one := 1
	dataSignal := privateData
	hold := b.decider(p, []deciderCondition{{
		FirstSignal: &b.s.write, Comparator: "=", Constant: &zero,
	}, {
		FirstSignal: &b.s.arm, Comparator: "=", Constant: &one,
		CompareType: "and",
	}}, []deciderOutput{{
		Signal: &dataSignal, CopyCountFromInput: true,
	}})
	b.stateBus.add(hold, connectorGreenIn)
	write := b.multiplySignals(
		recursiveMachinePoint{p.x + 1, p.y},
		b.s.data,
		b.s.write,
		dataSignal,
	)
	output := b.arithConstant(
		recursiveMachinePoint{p.x + 2, p.y},
		dataSignal,
		"*",
		one,
		value,
	)
	b.e.link(hold, connectorRedOut, hold, connectorRedIn)
	b.e.link(write, connectorRedOut, hold, connectorRedIn)
	b.e.link(output, connectorRedIn, hold, connectorRedIn)
	data.add(write, connectorGreenIn)
	return recursiveMachineCell{
		hold: hold, write: write, output: output,
	}
}

// connectWrite lets a field selector control both hold and incoming data paths.
func (b *recursiveMachineBuilder) connectWrite(
	cell recursiveMachineCell,
	selector handle,
) {
	b.e.link(selector, connectorRedOut, cell.hold, connectorRedIn)
	b.e.link(selector, connectorRedOut, cell.write, connectorRedIn)
}

// arithOperands creates arithmetic with either stored or immediate operands.
func (b *recursiveMachineBuilder) arithOperands(
	p recursiveMachinePoint,
	x recursiveOperand,
	op string,
	y recursiveOperand,
	output signalID,
) handle {
	conditions := &arithmeticConditions{
		Operation: op, OutputSignal: &output,
	}
	if x.isConstant {
		conditions.FirstConstant = &x.constant
	} else {
		conditions.FirstSignal = &b.s.slots[x.slot]
	}
	if y.isConstant {
		conditions.SecondConstant = &y.constant
	} else {
		conditions.SecondSignal = &b.s.slots[y.slot]
	}
	return b.e.add(entity{
		Name:      arithCombinatorName,
		Position:  b.logicPosition(p),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{
			ArithmeticConditions: conditions,
		},
	})
}

// multiplySignals provides the common two-signal gate used throughout actions.
func (b *recursiveMachineBuilder) multiplySignals(
	p recursiveMachinePoint,
	first signalID,
	second signalID,
	output signalID,
) handle {
	return b.e.add(entity{
		Name:      arithCombinatorName,
		Position:  b.logicPosition(p),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{
			ArithmeticConditions: &arithmeticConditions{
				FirstSignal:  &first,
				Operation:    "*",
				SecondSignal: &second,
				OutputSignal: &output,
			},
		},
	})
}

// arithConstant provides compact signal-to-constant controller arithmetic.
func (b *recursiveMachineBuilder) arithConstant(
	p recursiveMachinePoint,
	first signalID,
	op string,
	constant int,
	output signalID,
) handle {
	return b.e.add(entity{
		Name:      arithCombinatorName,
		Position:  b.logicPosition(p),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{
			ArithmeticConditions: &arithmeticConditions{
				FirstSignal:    &first,
				Operation:      op,
				SecondConstant: &constant,
				OutputSignal:   &output,
			},
		},
	})
}

// decider canonicalises comparisons before emitting conditional controller
// logic.
func (b *recursiveMachineBuilder) decider(
	p recursiveMachinePoint,
	conditions []deciderCondition,
	outputs []deciderOutput,
) handle {
	for index := range conditions {
		conditions[index].Comparator = canonicalComparator(
			conditions[index].Comparator,
		)
	}
	return b.e.add(entity{
		Name:      deciderCombinatorName,
		Position:  b.logicPosition(p),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{
			DeciderConditions: &deciderConditions{
				Conditions: conditions,
				Outputs:    outputs,
			},
		},
	})
}

// rmCondition constructs the repeated signal-to-constant action predicate.
func rmCondition(
	signal signalID,
	comparator string,
	constant int,
	compareType string,
) deciderCondition {
	return deciderCondition{
		FirstSignal: &signal,
		Comparator:  comparator,
		Constant:    &constant,
		CompareType: compareType,
	}
}

// staticPanel labels machine regions without requiring circuit input.
func (b *recursiveMachineBuilder) staticPanel(
	p recursiveMachinePoint,
	text string,
) {
	b.e.add(entity{
		Name:       displayPanelName,
		Position:   b.panelPosition(p),
		Text:       text,
		AlwaysShow: true,
	})
}

// statusPanel displays a message only while its status network is active.
func (b *recursiveMachineBuilder) statusPanel(
	p recursiveMachinePoint,
	text string,
	net *recursiveMachineNet,
) {
	h := b.e.add(entity{
		Name:       displayPanelName,
		Position:   b.panelPosition(p),
		AlwaysShow: true,
		ControlBehavior: &controlBehavior{Parameters: []displayPanelMessage{{
			Text: text,
			Condition: &displayPanelCondition{
				FirstSignal: &b.s.action,
				Comparator:  "=",
				Constant:    1,
			},
		}}},
	})
	net.add(h, connectorGreenIn)
}

// activityLamp makes action and frame selection visible during execution.
func (b *recursiveMachineBuilder) activityLamp(
	p recursiveMachinePoint,
	activeCount int,
) handle {
	return b.e.add(entity{
		Name:     smallLampEntityName,
		Position: b.panelPosition(p),
		Colour:   greenLampColour(),
		ControlBehavior: &controlBehavior{
			CircuitEnabled: true,
			CircuitCondition: &circuitCondition{
				FirstSignal: &b.s.action,
				Comparator:  "=",
				Constant:    activeCount,
			},
		},
	})
}

// logicPosition maps machine-grid points to combinator coordinates.
func (b *recursiveMachineBuilder) logicPosition(
	p recursiveMachinePoint,
) position {
	return position{
		X: b.self.pos.X + float64(p.x) - 0.5,
		Y: b.self.pos.Y + float64(p.y) - 0.5,
	}
}

// panelPosition aligns one-tile displays and lamps with machine-grid rows.
func (b *recursiveMachineBuilder) panelPosition(
	p recursiveMachinePoint,
) position {
	return position{
		X: b.self.pos.X + float64(p.x) - 0.5,
		Y: b.self.pos.Y + float64(p.y) - 1,
	}
}

type recursiveMachinePoint struct{ x, y int }

type recursiveMachineLayout struct {
	x, y, maxX int
}

// newRecursiveMachineLayout starts controller placement below public status.
func newRecursiveMachineLayout() recursiveMachineLayout {
	return recursiveMachineLayout{y: 4}
}

// group keeps a related action together on one controller row when possible.
func (l *recursiveMachineLayout) group(size int) {
	if size > recursiveMachineControllerWrapWidth {
		panic("factorio: recursive machine action exceeds layout width")
	}
	if l.x+size > recursiveMachineControllerWrapWidth {
		l.x = 0
		l.y += recursiveMachineControllerRowHeight
	}
}

// take allocates the next controller point while tracking required width.
func (l *recursiveMachineLayout) take() recursiveMachinePoint {
	if l.x == recursiveMachineControllerWrapWidth {
		l.x = 0
		l.y += recursiveMachineControllerRowHeight
	}
	p := recursiveMachinePoint{l.x, l.y}
	l.x++
	l.maxX = max(l.maxX, l.x)
	return p
}

// width reports the horizontal clearance required before frame placement.
func (l *recursiveMachineLayout) width() int { return l.maxX }

// padFrom preserves label clearance and stable action spacing.
func (l *recursiveMachineLayout) padFrom(
	start recursiveMachinePoint,
	width int,
) {
	if l.y != start.y {
		panic("factorio: recursive machine action crossed layout rows")
	}
	for l.x < start.x+width {
		l.take()
	}
}

type recursiveMachineEndpoint struct {
	h    handle
	conn int
}

type recursiveMachineNet struct {
	b         *recursiveMachineBuilder
	colour    wireColour
	endpoints []recursiveMachineEndpoint
	seen      map[recursiveMachineEndpoint]bool
}

// net owns one coloured logical network until all endpoints are known.
func (b *recursiveMachineBuilder) net(
	colour wireColour,
) *recursiveMachineNet {
	net := &recursiveMachineNet{
		b: b, colour: colour,
		seen: make(map[recursiveMachineEndpoint]bool),
	}
	b.nets = append(b.nets, net)
	return net
}

// add records a unique, colour-compatible endpoint for deferred wiring.
func (n *recursiveMachineNet) add(h handle, connector int) {
	if isRedConnector(connector) != (n.colour == red) {
		panic("factorio: recursive machine net colour mismatch")
	}
	endpoint := recursiveMachineEndpoint{h: h, conn: connector}
	if n.seen[endpoint] {
		return
	}
	n.seen[endpoint] = true
	n.endpoints = append(n.endpoints, endpoint)
}

// wireAll finalises deferred networks after every entity has been positioned.
func (b *recursiveMachineBuilder) wireAll() {
	for _, net := range b.nets {
		net.wire()
	}
}

// wire connects a net's endpoints with a minimum spanning tree, which minimises
// the longest single link. A greedy nearest-neighbour chain threads the
// endpoints into a path and can strand a distant one on a final over-reach hop
// even when a shorter connection exists, such as an activity lamp left to reach
// back across a wide action row. A tree is bottleneck-optimal, so it never
// wastes reach that way. Red links must physically reach, so a red tree edge
// that still exceeds reach means no spanning wiring is possible.
func (n *recursiveMachineNet) wire() {
	for _, edge := range n.spanningTree() {
		if n.colour == red && edge.lengthSquared > wireReach*wireReach {
			panic("factorio: recursive machine red wire exceeds reach")
		}
		from, to := n.endpoints[edge.from], n.endpoints[edge.to]
		n.b.e.link(from.h, from.conn, to.h, to.conn)
	}
}

// recursiveMachineTreeEdge is one spanning-tree link between endpoint indices,
// carrying its squared length so the reach guard avoids a redundant square root.
type recursiveMachineTreeEdge struct {
	from, to      int
	lengthSquared float64
}

// spanningTree grows a minimum spanning tree over the endpoints with Prim's
// algorithm, so the longest single link is as short as any spanning wiring
// allows. best[i] is the shortest known link from endpoint i to the tree and
// parent[i] the in-tree endpoint that realises it.
func (n *recursiveMachineNet) spanningTree() []recursiveMachineTreeEdge {
	if len(n.endpoints) < 2 {
		return nil
	}
	inTree := make([]bool, len(n.endpoints))
	best := make([]float64, len(n.endpoints))
	parent := make([]int, len(n.endpoints))
	inTree[0] = true
	for i := 1; i < len(n.endpoints); i++ {
		best[i] = n.distanceSquared(0, i)
	}
	edges := make([]recursiveMachineTreeEdge, 0, len(n.endpoints)-1)
	for len(edges) < cap(edges) {
		next := nearestOutOfTree(inTree, best)
		edges = append(edges, recursiveMachineTreeEdge{
			from: parent[next], to: next, lengthSquared: best[next],
		})
		inTree[next] = true
		n.relaxToTree(inTree, best, parent, next)
	}
	return edges
}

// distanceSquared reports the squared tile distance between two endpoints.
func (n *recursiveMachineNet) distanceSquared(a, b int) float64 {
	from := n.b.e.entities[int(n.endpoints[a].h)-1].Position
	to := n.b.e.entities[int(n.endpoints[b].h)-1].Position
	dx := from.X - to.X
	dy := from.Y - to.Y
	return dx*dx + dy*dy
}

// nearestOutOfTree returns the endpoint with the shortest pending link, choosing
// the lowest index on ties so wiring stays deterministic.
func nearestOutOfTree(inTree []bool, best []float64) int {
	next := -1
	for i := 1; i < len(inTree); i++ {
		if !inTree[i] && (next == -1 || best[i] < best[next]) {
			next = i
		}
	}
	return next
}

// relaxToTree shortens each pending link once a new endpoint joins the tree.
func (n *recursiveMachineNet) relaxToTree(
	inTree []bool,
	best []float64,
	parent []int,
	added int,
) {
	for i := 1; i < len(n.endpoints); i++ {
		if d := n.distanceSquared(added, i); !inTree[i] && d < best[i] {
			best[i] = d
			parent[i] = added
		}
	}
}
