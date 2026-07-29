// This file provides a deterministic oracle for Factorio circuit timing.
package factorio

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// sim is a synchronous tick simulator over a blueprint's emitted entities and
// wires. It is the correctness oracle of the redesign: it ticks the combinator
// network to a settled state so a test can assert the blueprint computes the
// same value as the Go function, not merely that it imports.
//
// Model: each (entity, connector) is a node; wires union nodes into networks.
// A combinator reads the merged value of its red and green input networks and,
// after one tick of latency, writes its result to its output networks. A value
// of zero is the same as no signal, so a computed zero is not written, matching
// the engine's zero-is-absent rule.
type sim struct {
	state       simState               // network id -> signal -> value
	next        simState               // reusable next-tick state
	root        map[simNode]simNode    // connector union-find roots
	networks    []simNetwork           // connector -> dense network id
	networkIDs  map[simNode]simNetwork // root -> dense network id
	nextNetwork simNetwork
	freeRows    []map[string]int // cleared signal maps ready for reuse
	seeded      bool             // constants placed into the initial state
}

type simNode int
type simNetwork int
type simState []map[string]int

// newSim builds the simulator network from wires, with empty state, ready to
// advance one tick at a time. Tests that inspect intermediate circuit states
// drive advance directly and read the value after each tick.
func newSim(wires []wire) *sim {
	s := &sim{
		state:      make(simState, 1),
		next:       make(simState, 1),
		root:       map[simNode]simNode{},
		networkIDs: map[simNode]simNetwork{},
	}
	for _, w := range wires {
		s.union(nodeKey(w[0], w[1]), nodeKey(w[2], w[3]))
	}
	return s
}

// advance installs the next state computed from the current one in one tick.
// The first call seeds the always-on constant outputs into the initial state,
// so the first tick already sees them.
func (s *sim) advance(entities []entity) {
	if !s.seeded {
		s.state = s.constantState(entities)
		s.ensureNetwork(s.nextNetwork)
		s.seeded = true
	}
	s.step(entities)
	s.state, s.next = s.next, s.state
}

// constantState builds the initial state from the constant combinators. In
// Factorio a constant combinator's signal is present the instant it is powered,
// with no combinator latency, so tick zero must already see it. A combinator,
// by contrast, has one tick of latency. Seeding constants removes the one-tick
// startup skew that a pulse-counting loop would otherwise turn into an
// off-by-one (the clock's startup pulse would land before the gate's condition
// is ready).
func (s *sim) constantState(entities []entity) simState {
	state := make(simState, len(s.state))
	add := func(netID simNetwork, signal string, v int) {
		addSignalInt32(&state, netID, signal, v)
	}
	for _, ent := range entities {
		if ent.Name == constCombinatorName {
			s.stepConstant(ent, ent.EntityNumber, add)
		}
	}
	return state
}

// simulate ticks the entities and wires until the state stops changing. It
// fails rather than returning an unsettled circuit at the tick budget.
func simulate(
	t *testing.T,
	entities []entity,
	wires []wire,
	maxTicks int,
) *sim {
	t.Helper()
	s := newSim(wires)
	for range maxTicks {
		prev := s.state
		s.advance(entities)
		if equalState(prev, s.state) {
			return s
		}
	}
	t.Fatalf("circuit did not settle within %d ticks", maxTicks)
	return s
}

// value returns the value of a signal on the network at one entity connector.
func (s *sim) value(entityNumber, conn int, signal string) int {
	return s.state[s.network(entityNumber, conn)][signal]
}

// step computes the reusable next state from the current synchronous tick.
func (s *sim) step(entities []entity) {
	s.resetNext()
	for _, ent := range entities {
		num := ent.EntityNumber
		switch ent.Name {
		case constCombinatorName:
			s.stepConstant(ent, num, s.addNextSignal)
		case arithCombinatorName:
			s.stepArith(ent, num, s.addNextSignal)
		case deciderCombinatorName:
			s.stepDecider(ent, num, s.addNextSignal)
		}
	}
}

// resetNext clears the older tick while retaining its allocated maps.
func (s *sim) resetNext() {
	for network, row := range s.next {
		if row == nil {
			continue
		}
		clear(row)
		s.freeRows = append(s.freeRows, row)
		s.next[network] = nil
	}
}

// addNextSignal merges one output into the reusable next-tick state.
func (s *sim) addNextSignal(network simNetwork, signal string, value int) {
	if value == 0 {
		return
	}
	row := s.next[network]
	sum := addInt32(row[signal], value)
	if sum == 0 {
		delete(row, signal)
		if len(row) == 0 && row != nil {
			s.next[network] = nil
			s.freeRows = append(s.freeRows, row)
		}
		return
	}
	if row == nil {
		row = s.takeRow()
		s.next[network] = row
	}
	row[signal] = sum
}

// takeRow returns a cleared signal map or allocates one during warm-up.
func (s *sim) takeRow() map[string]int {
	last := len(s.freeRows) - 1
	if last < 0 {
		return map[string]int{}
	}
	row := s.freeRows[last]
	s.freeRows[last] = nil
	s.freeRows = s.freeRows[:last]
	return row
}

// stepConstant preserves Factorio's immediate, dual-colour source behaviour in
// the timing oracle.
func (s *sim) stepConstant(
	ent entity,
	num int,
	add func(simNetwork, string, int),
) {
	if ent.ControlBehavior == nil || ent.ControlBehavior.Sections == nil {
		return
	}
	if ent.ControlBehavior.IsOn != nil && !*ent.ControlBehavior.IsOn {
		return
	}
	redNet := s.network(num, connectorRedIn)
	greenNet := s.network(num, connectorGreenIn)
	for _, sec := range ent.ControlBehavior.Sections.Sections {
		for _, f := range sec.Filters {
			add(redNet, f.Name, f.Count)
			add(greenNet, f.Name, f.Count)
		}
	}
}

// stepArith keeps arithmetic latency and output fan-out faithful to the game.
func (s *sim) stepArith(
	ent entity,
	num int,
	add func(simNetwork, string, int),
) {
	ac := ent.ControlBehavior.ArithmeticConditions
	if ac == nil {
		return
	}
	first := s.operand(num, ac.FirstSignal, ac.FirstConstant)
	second := s.operand(num, ac.SecondSignal, ac.SecondConstant)
	res := applyArith(ac.Operation, first, second)
	if ac.OutputSignal == nil {
		return
	}
	add(s.network(num, connectorRedOut), ac.OutputSignal.Name, res)
	add(s.network(num, connectorGreenOut), ac.OutputSignal.Name, res)
}

// stepDecider models conditional presence and copy-count semantics needed by
// gates and state machines.
func (s *sim) stepDecider(
	ent entity,
	num int,
	add func(simNetwork, string, int),
) {
	dc := ent.ControlBehavior.DeciderConditions
	if dc == nil || !s.evalDeciderConditions(num, dc.Conditions) {
		return
	}
	for _, out := range dc.Outputs {
		if out.Signal == nil {
			continue
		}
		v := 1
		if out.CopyCountFromInput {
			v = s.inputSignal(num, out.Signal.Name)
		}
		add(s.network(num, connectorRedOut), out.Signal.Name, v)
		add(s.network(num, connectorGreenOut), out.Signal.Name, v)
	}
}

// evalDeciderConditions evaluates Factorio's OR groups after reducing every
// AND group. A missing compare_type joins a row with OR.
func (s *sim) evalDeciderConditions(
	num int,
	conditions []deciderCondition,
) bool {
	if len(conditions) == 0 {
		return false
	}

	result := false
	group := s.evalDeciderCondition(num, conditions[0])
	for _, cond := range conditions[1:] {
		value := s.evalDeciderCondition(num, cond)
		if cond.CompareType == "and" {
			group = group && value
			continue
		}
		result = result || group
		group = value
	}
	return result || group
}

// evalDeciderCondition gives each AND/OR row the same signal and constant
// comparison rules.
func (s *sim) evalDeciderCondition(num int, cond deciderCondition) bool {
	left := s.inputSignal(num, signalName(cond.FirstSignal))
	var right int
	if cond.SecondSignal != nil {
		right = s.inputSignal(num, cond.SecondSignal.Name)
	} else if cond.Constant != nil {
		right = factorioInt32(*cond.Constant)
	}
	return evalCompare(cond.Comparator, left, right)
}

// operand resolves an arithmetic operand: a signal read from the merged input,
// or a constant.
func (s *sim) operand(num int, sig *signalID, constant *int) int {
	if sig != nil {
		return s.inputSignal(num, sig.Name)
	}
	if constant != nil {
		return factorioInt32(*constant)
	}
	return 0
}

// inputSignal returns a signal's value merged across the entity's red and
// green input networks, the way a combinator reads its input.
func (s *sim) inputSignal(num int, signal string) int {
	if signal == "" {
		return 0
	}
	redNet := s.network(num, connectorRedIn)
	greenNet := s.network(num, connectorGreenIn)
	return addInt32(
		s.state[redNet][signal],
		s.state[greenNet][signal],
	)
}

// network resolves one connector once and reuses a dense network id on later
// ticks.
func (s *sim) network(entityNumber, connector int) simNetwork {
	key := nodeKey(entityNumber, connector)
	index := int(key)
	if index < len(s.networks) && s.networks[index] != 0 {
		return s.networks[index]
	}
	root := s.find(key)
	network, ok := s.networkIDs[root]
	if !ok {
		s.nextNetwork++
		network = s.nextNetwork
		s.networkIDs[root] = network
	}
	if index >= len(s.networks) {
		s.networks = append(s.networks, make([]simNetwork, index-len(s.networks)+1)...)
	}
	s.networks[index] = network
	s.ensureNetwork(network)
	return network
}

// ensureNetwork grows both tick buffers to hold one dense network id.
func (s *sim) ensureNetwork(network simNetwork) {
	length := int(network) + 1
	if len(s.state) < length {
		s.state = append(s.state, make(simState, length-len(s.state))...)
	}
	if len(s.next) < length {
		s.next = append(s.next, make(simState, length-len(s.next))...)
	}
}

// find canonicalises wired connectors so all members observe one network state.
func (s *sim) find(x simNode) simNode {
	p, ok := s.root[x]
	if !ok || p == x {
		s.root[x] = x
		return x
	}
	r := s.find(p)
	s.root[x] = r
	return r
}

// union makes each emitted wire merge its connector networks.
func (s *sim) union(a, b simNode) {
	s.root[s.find(a)] = s.find(b)
}

// nodeKey packs an entity connector into one stable union-find identity.
func nodeKey(entityNumber, conn int) simNode {
	return simNode(entityNumber<<3 | conn)
}

// TestNodeKeyKeepsConnectorsDistinct protects the three-bit packing boundary
// shared with entity connector validation.
func TestNodeKeyKeepsConnectorsDistinct(t *testing.T) {
	t.Parallel()
	seen := map[simNode]bool{}
	for entityNumber := 1; entityNumber <= 2; entityNumber++ {
		for connector := 1; connector < 8; connector++ {
			key := nodeKey(entityNumber, connector)
			require.False(t, seen[key])
			seen[key] = true
		}
	}
}

// signalName treats a missing optional signal as Factorio's absent input.
func signalName(s *signalID) string {
	if s == nil {
		return ""
	}
	return s.Name
}

// applyArith applies a Factorio signed-int32 arithmetic operation. Division and
// modulo truncate toward zero.
func applyArith(op string, a, b int) int {
	//nolint:gosec // Inputs intentionally wrap to Factorio signed 32 bits.
	a32, b32 := int32(a), int32(b)
	switch op {
	case "+":
		//nolint:gosec // Unsigned arithmetic models Factorio wraparound.
		return int(int32(uint32(a32) + uint32(b32)))
	case "-":
		//nolint:gosec // Unsigned arithmetic models Factorio wraparound.
		return int(int32(uint32(a32) - uint32(b32)))
	case "*":
		//nolint:gosec // Unsigned arithmetic models Factorio wraparound.
		return int(int32(uint32(a32) * uint32(b32)))
	case "/":
		if b32 == 0 {
			return 0
		}
		return int(a32 / b32)
	case "%":
		if b32 == 0 {
			return 0
		}
		return int(a32 % b32)
	default:
		panic("unsupported arithmetic operation " + strconv.Quote(op))
	}
}

// addInt32 preserves Factorio's signed network-sum wraparound.
func addInt32(a, b int) int {
	//nolint:gosec // Unsigned addition models Factorio network wraparound.
	return int(int32(uint32(a) + uint32(b)))
}

// addSignalInt32 keeps zero equivalent to absence while merging network values.
func addSignalInt32(
	state *simState,
	network simNetwork,
	signal string,
	value int,
) {
	if value == 0 {
		return
	}
	length := int(network) + 1
	if len(*state) < length {
		*state = append(*state, make(simState, length-len(*state))...)
	}
	row := (*state)[network]
	sum := addInt32(row[signal], value)
	if sum == 0 {
		delete(row, signal)
		if len(row) == 0 {
			(*state)[network] = nil
		}
		return
	}
	if row == nil {
		row = map[string]int{}
		(*state)[network] = row
	}
	row[signal] = sum
}

// stateSignal returns zero for an absent network or signal.
func stateSignal(state simState, network simNetwork, signal string) int {
	if int(network) >= len(state) {
		return 0
	}
	return state[network][signal]
}

// factorioInt32 constrains host integers to Factorio's signed signal width.
func factorioInt32(value int) int {
	//nolint:gosec // Constants intentionally wrap to Factorio signed 32 bits.
	return int(int32(value))
}

// evalCompare evaluates a Factorio decider comparator. Both ASCII and the
// Unicode forms the generator emits are accepted.
func evalCompare(op string, a, b int) bool {
	switch op {
	case "=":
		return a == b
	case "≠", "!=":
		return a != b
	case "<":
		return a < b
	case "≤", "<=":
		return a <= b
	case ">":
		return a > b
	case "≥", ">=":
		return a >= b
	default:
		panic("unsupported comparator " + strconv.Quote(op))
	}
}

// TestSimulatorRejectsUnknownOperators ensures malformed test cases fail
// instead of silently producing a plausible zero or false result.
func TestSimulatorRejectsUnknownOperators(t *testing.T) {
	t.Parallel()
	require.PanicsWithValue(
		t,
		`unsupported arithmetic operation "?"`,
		func() { applyArith("?", 1, 2) },
	)
	require.PanicsWithValue(
		t,
		`unsupported comparator "?"`,
		func() { evalCompare("?", 1, 2) },
	)
}

// equalState detects a true circuit fixed point without depending on map order.
func equalState(a, b simState) bool {
	return slices.EqualFunc(a, b, maps.Equal)
}

// TestSimulatorWrapsSigned32ArithmeticAndNetworks pins Factorio's int32
// overflow at arithmetic outputs and when multiple wire values are summed.
func TestSimulatorWrapsSigned32ArithmeticAndNetworks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		op   string
		a, b int
		want int
	}{
		{name: "add", op: "+", a: math.MaxInt32, b: 1, want: math.MinInt32},
		{name: "subtract", op: "-", a: math.MinInt32, b: 1, want: math.MaxInt32},
		{name: "multiply", op: "*", a: math.MaxInt32, b: 2, want: -2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, applyArith(tc.op, tc.a, tc.b))
		})
	}

	s := newSim(nil)
	red := s.network(1, connectorRedIn)
	green := s.network(1, connectorGreenIn)
	s.state[red] = map[string]int{"signal-A": math.MaxInt32}
	s.state[green] = map[string]int{"signal-A": 1}
	require.Equal(t, math.MinInt32, s.inputSignal(1, "signal-A"))

	state := simState(nil)
	addSignalInt32(&state, 1, "signal-A", math.MinInt32)
	addSignalInt32(&state, 1, "signal-A", math.MinInt32)
	require.Nil(t, state[1])
}

// TestSimulatorEvaluatesCompoundDeciderConditions verifies explicit AND,
// default OR, and Factorio's AND-before-OR precedence.
func TestSimulatorEvaluatesCompoundDeciderConditions(t *testing.T) {
	t.Parallel()
	one := 1
	condition := func(signal, compareType string) deciderCondition {
		return deciderCondition{
			FirstSignal: &signalID{Type: "virtual", Name: signal},
			Comparator:  "=",
			Constant:    &one,
			CompareType: compareType,
		}
	}

	for _, tc := range []struct {
		name       string
		conditions []deciderCondition
		inputs     map[string]int
		want       int
	}{
		{
			name: "all AND conditions pass",
			conditions: []deciderCondition{
				condition("signal-A", ""),
				condition("signal-B", "and"),
			},
			inputs: map[string]int{"signal-A": 1, "signal-B": 1},
			want:   1,
		},
		{
			name: "one AND condition fails",
			conditions: []deciderCondition{
				condition("signal-A", ""),
				condition("signal-B", "and"),
			},
			inputs: map[string]int{"signal-A": 1},
			want:   0,
		},
		{
			name: "missing compare type defaults to OR",
			conditions: []deciderCondition{
				condition("signal-A", ""),
				condition("signal-B", ""),
			},
			inputs: map[string]int{"signal-B": 1},
			want:   1,
		},
		{
			name: "AND takes precedence over OR",
			conditions: []deciderCondition{
				condition("signal-A", ""),
				condition("signal-B", "or"),
				condition("signal-C", "and"),
			},
			inputs: map[string]int{"signal-A": 1},
			want:   1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := signalID{Type: "virtual", Name: "signal-check"}
			ent := entity{
				EntityNumber: 1,
				Name:         deciderCombinatorName,
				ControlBehavior: &controlBehavior{
					DeciderConditions: &deciderConditions{
						Conditions: tc.conditions,
						Outputs: []deciderOutput{{
							Signal: &output,
						}},
					},
				},
			}
			s := newSim(nil)
			inputNet := s.network(1, connectorRedIn)
			s.state[inputNet] = tc.inputs
			next := simState(nil)
			s.stepDecider(ent, 1, func(net simNetwork, signal string, value int) {
				addSignalInt32(&next, net, signal, value)
			})

			outputNet := s.network(1, connectorRedOut)
			require.Equal(t, tc.want, stateSignal(next, outputNet, output.Name))
		})
	}
}
