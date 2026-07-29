// This file protects recursive execution, visibility, timing, and reset safety.
package factorio

import (
	"fmt"
	"go/token"
	"maps"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

const factorialRecursiveSource = `package p
func factorial(n int) int {
	if n <= 1 {
		return 1
	}
	return n * factorial(n-1)
}`

const repeatRecursiveSource = `package p
func repeat(n, result int) int {
	if n == 0 {
		return result
	}
	return repeat(n-1, result)
}`

// recursiveSemanticPeriod is the shortest settled controller action; visible
// digits use the separately tested fastClockPeriod.
const recursiveSemanticPeriod = 6

type recursiveMachineTestRig struct {
	t               *testing.T
	machine         *recursiveMachine
	entities        []entity
	sim             *sim
	pulseNet        simNetwork
	pulseSignal     string
	result          connBinding
	resultSig       string
	stateOutput     handle
	startControl    handle
	status          map[string]recursiveMachineStatusProbe
	digits          []recursiveMachineDigitProbe
	controllerLamps []recursiveMachineLampProbe
	frameLamps      []recursiveMachineLampProbe
}

type recursiveMachineDigitProbe struct {
	panel  handle
	signal string
	place  int
}

type recursiveMachineStatusProbe struct {
	panel     handle
	condition displayPanelCondition
}

type recursiveMachineLampProbe struct {
	lamp      handle
	signal    string
	threshold int
	y         float64
}

// newRecursiveMachineTestRig provides the smallest executable recursive
// circuit for controller and stack assertions.
func newRecursiveMachineTestRig(
	t *testing.T,
	program *recursiveProgram,
	arguments ...int,
) *recursiveMachineTestRig {
	t.Helper()
	return newRecursiveMachineTestRigWithDisplay(
		t,
		program,
		false,
		arguments...,
	)
}

// newRecursiveMachineDisplayTestRig includes the visible result path when
// presentation timing is part of the contract under test.
func newRecursiveMachineDisplayTestRig(
	t *testing.T,
	program *recursiveProgram,
	arguments ...int,
) *recursiveMachineTestRig {
	t.Helper()
	return newRecursiveMachineTestRigWithDisplay(
		t,
		program,
		true,
		arguments...,
	)
}

// newRecursiveMachineTestRigWithDisplay keeps both rig variants on the same
// production allocation, placement, emission, and routing path.
func newRecursiveMachineTestRigWithDisplay(
	t *testing.T,
	program *recursiveProgram,
	withDisplay bool,
	arguments ...int,
) *recursiveMachineTestRig {
	t.Helper()
	require.Len(t, arguments, len(program.params))

	machine := newRecursiveMachine(program)
	machineInstance := newInstance(machine)
	clock := newInstance(newClockDivWithSummary(clockPeriod, ""))
	pulse := newInstance(newConstSrc(0))
	instances := make([]*instance, 0, len(arguments)+4)
	instances = append(instances, clock, pulse)
	nets := make([]*netlistNet, 0, len(arguments)+4)
	for index, value := range arguments {
		argument := newInstance(newConstSrc(value))
		instances = append(instances, argument)
		nets = append(nets, connect(
			argument.port("out"),
			machineInstance.port(fmt.Sprintf("arg%d", index)),
		))
	}
	instances = append(instances, machineInstance)
	resultNet := connect(machineInstance.port("result"))
	if withDisplay {
		display := newInstance(&digitDisplay{})
		instances = append(instances, display)
		resultNet = connect(
			machineInstance.port("result"),
			display.port("in"),
		)
	}
	nets = append(nets,
		connect(clock.port("pulse")),
		connect(clock.port("start"), machineInstance.port("start")),
		connect(pulse.port("out"), machineInstance.port("pulse")),
		resultNet,
	)
	require.NoError(t, allocateSignals(nets))
	place(instances, nets)
	e := emitNetlist(instances, netEdges(nets))
	require.NoError(t, insertRelays(e))
	require.NoError(t, verifyEmitted(e.entities, e.wires))

	pulseBinding := e.bound[pulse.port("out")]
	result := e.bound[machineInstance.port("result")]
	rig := recursiveMachineRigFromEmission(
		t,
		machine,
		e,
		result,
		machineInstance.port("result").net.signal.Name,
		machineInstance.port("start").net.signal,
		withDisplay,
	)
	rig.pulseNet = rig.sim.network(
		int(pulseBinding.h),
		pulseBinding.conn,
	)
	rig.pulseSignal = pulse.port("out").net.signal.Name
	rig.setStarted(true)
	return rig
}

// recursiveMachineRigFromEmission discovers probes from emitted behaviour so
// tests do not depend on fixed entity numbers or coordinates.
func recursiveMachineRigFromEmission(
	t *testing.T,
	machine *recursiveMachine,
	e *emitter,
	result connBinding,
	resultSignal string,
	startSignal signalID,
	withDisplay bool,
) *recursiveMachineTestRig {
	t.Helper()
	simulation := newSim(e.wires)
	stateOutput := recursiveMachineStateOutput(t, machine, e.entities)
	status := make(map[string]recursiveMachineStatusProbe)
	var digits []recursiveMachineDigitProbe
	for _, ent := range e.entities {
		if ent.Name != displayPanelName || ent.ControlBehavior == nil {
			continue
		}
		parameters := ent.ControlBehavior.Parameters
		for _, parameter := range parameters {
			if parameter.Text != "" && parameter.Condition != nil &&
				parameter.Condition.FirstSignal != nil {
				status[parameter.Text] = recursiveMachineStatusProbe{
					panel:     handle(ent.EntityNumber),
					condition: *parameter.Condition,
				}
			}
		}
		if len(parameters) != 10 ||
			parameters[0].Condition == nil ||
			parameters[0].Condition.FirstSignal == nil {
			continue
		}
		signal := parameters[0].Condition.FirstSignal.Name
		var place int
		_, err := fmt.Sscanf(signal, "signal-%d", &place)
		require.NoError(t, err)
		digits = append(digits, recursiveMachineDigitProbe{
			panel: handle(ent.EntityNumber), signal: signal, place: place,
		})
	}
	if withDisplay {
		require.Len(t, digits, displayDigits)
	}
	startControl := recursiveMachineStartControl(
		t,
		e.entities,
		startSignal,
	)
	controllerLamps, frameLamps := recursiveMachineLampProbes(t, e.entities)
	return &recursiveMachineTestRig{
		t:               t,
		machine:         machine,
		entities:        e.entities,
		sim:             simulation,
		result:          result,
		resultSig:       resultSignal,
		stateOutput:     stateOutput,
		startControl:    startControl,
		status:          status,
		digits:          digits,
		controllerLamps: controllerLamps,
		frameLamps:      frameLamps,
	}
}

// recursiveMachineStateOutput finds the emitted cell that exposes state.
func recursiveMachineStateOutput(
	t *testing.T,
	machine *recursiveMachine,
	entities []entity,
) handle {
	t.Helper()
	want := newRecursiveMachineSignals(machine.program.slotCount).mode
	for _, ent := range entities {
		if ent.ControlBehavior == nil {
			continue
		}
		conditions := ent.ControlBehavior.ArithmeticConditions
		if conditions != nil && conditions.OutputSignal != nil &&
			*conditions.OutputSignal == want {
			return handle(ent.EntityNumber)
		}
	}
	require.FailNow(t, "missing recursive machine state output")
	return 0
}

// recursiveMachineStartControl identifies and validates the shared clock's
// sole user-facing start and reset control.
func recursiveMachineStartControl(
	t *testing.T,
	entities []entity,
	startSignal signalID,
) handle {
	t.Helper()
	var found handle
	for _, ent := range entities {
		if ent.Name != constCombinatorName ||
			ent.ControlBehavior == nil ||
			ent.ControlBehavior.IsOn == nil {
			continue
		}
		require.Zero(t, found)
		require.False(t, *ent.ControlBehavior.IsOn)
		require.Equal(t, &constantCombinatorSections{
			Sections: []logisticSection{{
				Index: 1,
				Filters: []constantFilter{{
					Index: 1, Type: startSignal.Type,
					Name: startSignal.Name, Quality: "normal",
					Comparator: "=", Count: 1,
				}},
			}},
		}, ent.ControlBehavior.Sections)
		found = handle(ent.EntityNumber)
	}
	require.NotZero(t, found)
	return found
}

// recursiveMachineLampProbes separates controller and frame indicators so
// execution visibility can be asserted independently.
func recursiveMachineLampProbes(
	t *testing.T,
	entities []entity,
) ([]recursiveMachineLampProbe, []recursiveMachineLampProbe) {
	t.Helper()
	var controllerLamps []recursiveMachineLampProbe
	var frameLamps []recursiveMachineLampProbe
	for _, ent := range entities {
		if ent.Name != smallLampEntityName {
			continue
		}
		require.Equal(t, greenLampColour(), ent.Colour)
		require.NotNil(t, ent.ControlBehavior)
		require.True(t, ent.ControlBehavior.CircuitEnabled)
		condition := ent.ControlBehavior.CircuitCondition
		require.NotNil(t, condition)
		require.NotNil(t, condition.FirstSignal)
		require.Equal(t, privateInc, *condition.FirstSignal)
		require.Equal(t, "=", condition.Comparator)
		probe := recursiveMachineLampProbe{
			lamp:      handle(ent.EntityNumber),
			signal:    condition.FirstSignal.Name,
			threshold: condition.Constant,
			y:         ent.Position.Y,
		}
		switch condition.Constant {
		case 1:
			controllerLamps = append(controllerLamps, probe)
		case 2:
			frameLamps = append(frameLamps, probe)
		default:
			require.Failf(
				t,
				"unexpected activity lamp threshold",
				"entity %d uses %d",
				ent.EntityNumber,
				condition.Constant,
			)
		}
	}
	sort.Slice(frameLamps, func(i, j int) bool {
		return frameLamps[i].y < frameLamps[j].y
	})
	return controllerLamps, frameLamps
}

// tick advances the simulator while allowing tests to inject one raw action
// pulse without rebuilding the circuit.
func (r *recursiveMachineTestRig) tick(pulse int) {
	if pulse != 0 {
		if r.sim.state[r.pulseNet] == nil {
			r.sim.state[r.pulseNet] = map[string]int{}
		}
		r.sim.state[r.pulseNet][r.pulseSignal] = pulse
	}
	r.sim.advance(r.entities)
}

// action advances one default-period controller step for semantic tests.
func (r *recursiveMachineTestRig) action() {
	r.actionAtPeriod(clockPeriod)
}

// actionAtPeriod exposes alternative clock periods to presentation-boundary
// tests without changing the generated machine.
func (r *recursiveMachineTestRig) actionAtPeriod(period int) {
	r.t.Helper()
	for tick := range period {
		if tick == 0 {
			r.tick(1)
			continue
		}
		r.tick(0)
	}
}

// TestRecursiveMachineRejectsUnknownOpcode makes instruction-set extensions
// fail loudly until their circuit lowering is implemented.
func TestRecursiveMachineRejectsUnknownOpcode(t *testing.T) {
	t.Parallel()
	builder := &recursiveMachineBuilder{program: &recursiveProgram{
		instructions: []recursiveInstruction{{op: recursiveOpcode(255)}},
	}}

	require.PanicsWithValue(
		t,
		"factorio: recursive machine unsupported opcode",
		builder.buildInstructions,
	)
}

// TestRecursiveMachineFastClockCoversFullDisplay proves the fast period is the
// first one that settles every supported decimal place during PRESENT.
func TestRecursiveMachineFastClockCoversFullDisplay(t *testing.T) {
	t.Parallel()
	program := planRecursiveSource(t, repeatRecursiveSource, "repeat")
	const want = 87_654_321

	for _, tc := range []struct {
		name        string
		period      int
		wantSettled bool
	}{
		{
			name:   "one tick too short",
			period: fastClockPeriod - 1,
		},
		{
			name:        "fast clock boundary",
			period:      fastClockPeriod,
			wantSettled: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRecursiveMachineDisplayTestRig(t, program, 1, want)
			for range 20 {
				rig.tick(0)
			}
			for range 100 {
				rig.actionAtPeriod(tc.period)
				if rig.mode() != recursiveModePresent {
					continue
				}
				require.Equal(t, want, rig.resultValue())
				require.Equal(
					t,
					tc.wantSettled,
					rig.displayMatches(want),
					"display readings: %v",
					rig.displayReadings(),
				)
				return
			}
			t.Fatal("recursive machine did not enter PRESENT")
		})
	}
}

// setStarted models the player's explicit shared-clock start and reset
// interaction.
func (r *recursiveMachineTestRig) setStarted(started bool) {
	r.t.Helper()
	control := &r.entities[int(r.startControl)-1]
	require.Equal(r.t, int(r.startControl), control.EntityNumber)
	require.NotNil(r.t, control.ControlBehavior)
	require.NotNil(r.t, control.ControlBehavior.IsOn)
	*control.ControlBehavior.IsOn = started
}

// mode exposes the controller phase that defines recursive runtime progress.
func (r *recursiveMachineTestRig) mode() int {
	return r.sim.value(
		int(r.stateOutput),
		connectorGreenOut,
		r.machineSignals().mode.Name,
	)
}

// statusVisible checks the same conditional panel state a player observes.
func (r *recursiveMachineTestRig) statusVisible(text string) bool {
	r.t.Helper()
	probe, ok := r.status[text]
	require.True(r.t, ok)
	condition := probe.condition
	value := r.sim.inputSignal(
		int(probe.panel),
		condition.FirstSignal.Name,
	)
	return evalCompare(condition.Comparator, value, condition.Constant)
}

// displayMatches verifies the complete visible result instead of trusting the
// upstream result signal alone.
func (r *recursiveMachineTestRig) displayMatches(want int) bool {
	readings := r.displayReadings()
	for _, digit := range r.digits {
		divisor := 1
		for range digit.place {
			divisor *= 10
		}
		if readings[digit.place] != want/divisor%10 {
			return false
		}
	}
	return len(r.digits) == displayDigits
}

// displayReadings reports per-place values when a timing assertion fails.
func (r *recursiveMachineTestRig) displayReadings() map[int]int {
	readings := make(map[int]int, len(r.digits))
	for _, digit := range r.digits {
		readings[digit.place] = r.sim.value(
			int(digit.panel),
			connectorRedIn,
			digit.signal,
		)
	}
	return readings
}

// resultValue exposes the public result port promised to surrounding circuits.
func (r *recursiveMachineTestRig) resultValue() int {
	return r.sim.value(
		int(r.result.h),
		r.result.conn,
		r.resultSig,
	)
}

// resultPresent distinguishes a meaningful zero from signal absence.
func (r *recursiveMachineTestRig) resultPresent() bool {
	net := r.sim.network(int(r.result.h), r.result.conn)
	_, ok := r.sim.state[net][r.resultSig]
	return ok
}

// internalResultValue lets terminal-state tests detect accidental state drift.
func (r *recursiveMachineTestRig) internalResultValue() int {
	return r.sim.value(
		int(r.stateOutput),
		connectorGreenOut,
		r.machineSignals().result.Name,
	)
}

// machineState snapshots private controller state for reset and terminal-hold
// assertions.
func (r *recursiveMachineTestRig) machineState() map[string]int {
	net := r.sim.network(
		int(r.stateOutput),
		connectorGreenOut,
	)
	return maps.Clone(r.sim.state[net])
}

// stageInternalResult perturbs private state to prove terminal modes reject
// further writes.
func (r *recursiveMachineTestRig) stageInternalResult(value int) {
	r.t.Helper()
	require.NotZero(r.t, value)
	net := r.sim.network(
		int(r.stateOutput),
		connectorGreenOut,
	)
	if r.sim.state[net] == nil {
		r.sim.state[net] = map[string]int{}
	}
	r.sim.state[net][r.machineSignals().result.Name] = value
}

// machineSignals keeps probe names aligned with the planned frame width.
func (r *recursiveMachineTestRig) machineSignals() recursiveMachineSignals {
	return newRecursiveMachineSignals(r.machine.program.slotCount)
}

// stackPointer identifies which frame lamp should be visible during execution.
func (r *recursiveMachineTestRig) stackPointer() int {
	return r.sim.value(
		int(r.stateOutput),
		connectorGreenOut,
		r.machineSignals().sp.Name,
	)
}

// lampLit evaluates the actual circuit condition rather than an inferred mode.
func (r *recursiveMachineTestRig) lampLit(
	probe recursiveMachineLampProbe,
) bool {
	return r.sim.inputSignal(int(probe.lamp), probe.signal) ==
		probe.threshold
}

// litControllerLampCount enforces a single visible action at a time.
func (r *recursiveMachineTestRig) litControllerLampCount() int {
	return r.litLampCount(r.controllerLamps)
}

// litFrameLampIndexes reports active rows so failures identify the wrong frame.
func (r *recursiveMachineTestRig) litFrameLampIndexes() []int {
	indexes := make([]int, 0, len(r.frameLamps))
	for index, probe := range r.frameLamps {
		if r.lampLit(probe) {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

// litLampCount shares exact condition evaluation across indicator groups.
func (r *recursiveMachineTestRig) litLampCount(
	probes []recursiveMachineLampProbe,
) int {
	count := 0
	for _, probe := range probes {
		if r.lampLit(probe) {
			count++
		}
	}
	return count
}

// requireLiveActivityLamps protects the player's one-action, one-frame view of
// active recursion.
func (r *recursiveMachineTestRig) requireLiveActivityLamps() {
	r.t.Helper()
	require.NotEmpty(r.t, r.controllerLamps)
	require.Len(r.t, r.frameLamps, recursiveMachineRows)
	require.Equal(r.t, 1, r.litControllerLampCount())
	require.Equal(
		r.t,
		[]int{r.stackPointer()},
		r.litFrameLampIndexes(),
	)
}

// requireActivityLampsDark keeps inactive and terminal machines visually quiet.
func (r *recursiveMachineTestRig) requireActivityLampsDark() {
	r.t.Helper()
	require.Zero(r.t, r.litControllerLampCount())
	require.Empty(r.t, r.litFrameLampIndexes())
}

// run drives semantic test cases to a bounded terminal result and fails loudly
// on overflow or non-termination. Timing-boundary tests use actionAtPeriod;
// semantic tests use the controller's measured settle period without waiting
// for the separate display pipeline.
func (r *recursiveMachineTestRig) run(maxActions int) int {
	r.t.Helper()
	for range 20 {
		r.tick(0)
	}
	for range maxActions {
		r.actionAtPeriod(recursiveSemanticPeriod)
		switch r.mode() {
		case recursiveModeDone:
			for range 4 {
				r.tick(0)
			}
			return r.resultValue()
		case recursiveModeOverflow:
			r.t.Fatal("recursive machine overflowed")
		}
	}
	r.t.Fatalf("recursive machine did not finish in %d actions", maxActions)
	return 0
}

// TestRecursiveMachineActivityLampsTrackExecution proves every live state has
// one selected controller action and highlights the current stack frame.
func TestRecursiveMachineActivityLampsTrackExecution(t *testing.T) {
	t.Parallel()
	program := planRecursiveSource(t, factorialRecursiveSource, "factorial")
	rig := newRecursiveMachineTestRig(t, program, 5)
	for range 20 {
		rig.tick(0)
	}

	seenFrames := make(map[int]bool)
	for range 200 {
		if rig.mode() == recursiveModeDone {
			rig.requireActivityLampsDark()
			require.True(t, seenFrames[0])
			require.True(t, seenFrames[4])
			return
		}
		require.NotEqual(t, recursiveModeOverflow, rig.mode())
		rig.requireLiveActivityLamps()
		seenFrames[rig.stackPointer()] = true
		rig.action()
	}
	t.Fatal("recursive machine did not finish in 200 actions")
}

// TestRecursiveMachineStartControlResetsState proves the shared default-off
// START blocks pulses and switching it off clears a completed run for reuse.
func TestRecursiveMachineStartControlResetsState(t *testing.T) {
	t.Parallel()
	program := planRecursiveSource(t, factorialRecursiveSource, "factorial")
	rig := newRecursiveMachineTestRig(t, program, 5)
	require.Equal(t, 120, rig.run(200))
	require.Equal(t, recursiveModeDone, rig.mode())

	rig.setStarted(false)
	for range 10 {
		rig.tick(0)
	}
	require.Equal(t, recursiveModeInit, rig.mode())
	require.Empty(t, rig.machineState())
	require.False(t, rig.resultPresent())
	require.False(t, rig.statusVisible("RUNNING"))
	require.False(t, rig.statusVisible("DONE"))
	rig.requireActivityLampsDark()

	rig.action()
	require.Equal(t, recursiveModeInit, rig.mode())
	require.Empty(t, rig.machineState())
	rig.setStarted(true)
	require.Equal(t, 120, rig.run(200))
}

// TestRecursiveMachineStartControlWaitsForLateInput proves pulses cannot sample
// an absent argument while the shared clock remains stopped.
func TestRecursiveMachineStartControlWaitsForLateInput(t *testing.T) {
	t.Parallel()
	program := planRecursiveSource(t, factorialRecursiveSource, "factorial")
	rig := newRecursiveMachineTestRig(t, program, 5)
	rig.setStarted(false)

	var input *controlBehavior
	for index := range rig.entities {
		ent := &rig.entities[index]
		if ent.Name != constCombinatorName ||
			ent.ControlBehavior == nil ||
			ent.ControlBehavior.IsOn != nil ||
			ent.ControlBehavior.Sections == nil {
			continue
		}
		for _, section := range ent.ControlBehavior.Sections.Sections {
			for _, filter := range section.Filters {
				if filter.Count != 5 {
					continue
				}
				off := false
				ent.ControlBehavior.IsOn = &off
				input = ent.ControlBehavior
			}
		}
	}
	require.NotNil(t, input)

	for range 3 {
		rig.action()
	}
	require.Equal(t, recursiveModeInit, rig.mode())
	require.Empty(t, rig.machineState())

	*input.IsOn = true
	rig.setStarted(true)
	require.Equal(t, 120, rig.run(200))
}

// TestRecursiveMachinePresentsResultBeforeDone proves DONE appears only after
// the public result and numeric display have settled, including for zero.
func TestRecursiveMachinePresentsResultBeforeDone(t *testing.T) {
	t.Parallel()
	program := planRecursiveSource(t, repeatRecursiveSource, "repeat")
	tests := []struct {
		name string
		want int
	}{
		{name: "non-zero", want: 120},
		{name: "zero", want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRecursiveMachineDisplayTestRig(
				t,
				program,
				2,
				tc.want,
			)
			for range 20 {
				rig.tick(0)
			}
			for range 100 {
				rig.action()
				if rig.mode() != recursiveModePresent {
					continue
				}
				require.True(t, rig.statusVisible("RUNNING"))
				require.False(t, rig.statusVisible("DONE"))
				require.Equal(t, tc.want, rig.resultValue())
				require.Truef(
					t,
					rig.displayMatches(tc.want),
					"display readings: %v",
					rig.displayReadings(),
				)

				rig.action()
				require.Equal(t, recursiveModeDone, rig.mode())
				require.False(t, rig.statusVisible("RUNNING"))
				require.True(t, rig.statusVisible("DONE"))
				require.Equal(t, tc.want, rig.resultValue())
				require.True(t, rig.displayMatches(tc.want))
				return
			}
			t.Fatal("recursive machine did not present DONE")
		})
	}
}

// TestRecursiveMachineRunsThroughGeneratedClock proves the full generated
// netlist advances through its clock, relays, presentation, and terminal
// status after the shared clock is manually started.
func TestRecursiveMachineRunsThroughGeneratedClock(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, repeatRecursiveSource)
	const want = 87_654_321
	for _, tc := range []struct {
		name, summary string
		period        int
		opts          []Option
	}{
		{
			name: "default", summary: "recursion clock (1 Hz)",
			period: clockPeriod,
		},
		{
			name: "fast", summary: "recursion clock (4 Hz)",
			period: fastClockPeriod, opts: []Option{WithFastClock()},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, path, "repeat")
			opts := append([]Option{WithParams(map[string]int{
				"n": 1, "result": want,
			})}, tc.opts...)
			compiled, err := compileFunction(fn, opts...)
			require.NoError(t, err)
			assertGeneratedRecursiveClock(
				t,
				compiled,
				tc.period,
				tc.summary,
				want,
			)
		})
	}
}

// assertGeneratedRecursiveClock proves a generated clock drives presentation
// and completion without test-injected pulses.
func assertGeneratedRecursiveClock(
	t *testing.T,
	compiled *compilation,
	period int,
	summary string,
	want int,
) {
	t.Helper()
	machine, clock := generatedRecursiveInstances(t, compiled.e)
	clockComponent, ok := clock.comp.(*clockDiv)
	require.True(t, ok)
	require.Equal(t, period, clockComponent.period)
	require.Equal(t, summary, clockComponent.summary)
	require.Positive(
		t,
		countEntities(compiled.e.entities, relayPoleEntityName),
	)
	result, ok := compiled.e.bound[compiled.resultNet.driver]
	require.True(t, ok)
	rig := recursiveMachineRigFromEmission(
		t,
		machine,
		compiled.e,
		result,
		compiled.resultNet.signal.Name,
		clock.port("start").net.signal,
		true,
	)
	clockBinding, ok := compiled.e.bound[clock.port("pulse")]
	require.True(t, ok)
	clockSignal := clock.port("pulse").net.signal.Name

	require.Equal(t, recursiveModeInit, rig.mode())
	require.Zero(t, rig.resultValue())
	require.False(t, rig.statusVisible("RUNNING"))
	require.False(t, rig.statusVisible("DONE"))
	rig.requireActivityLampsDark()
	for range 2 * period {
		rig.sim.advance(rig.entities)
	}
	require.Equal(t, recursiveModeInit, rig.mode())
	require.Empty(t, rig.machineState())
	rig.requireActivityLampsDark()
	rig.setStarted(true)

	clockPulses := 0
	presentPulses := -1
	presentSettled := false
	doneSettled := false
	for range 30 * period {
		rig.sim.advance(rig.entities)
		if rig.sim.value(
			int(clockBinding.h),
			clockBinding.conn,
			clockSignal,
		) == 1 {
			clockPulses++
		}
		switch rig.mode() {
		case recursiveModePresent:
			if presentPulses < 0 {
				presentPulses = clockPulses
			}
			presentSettled = presentSettled ||
				rig.statusVisible("RUNNING") &&
					!rig.statusVisible("DONE") &&
					rig.resultValue() == want &&
					rig.displayMatches(want)
		case recursiveModeDone:
			doneSettled = !rig.statusVisible("RUNNING") &&
				rig.statusVisible("DONE") &&
				rig.resultValue() == want &&
				rig.displayMatches(want)
		}
		if doneSettled {
			break
		}
	}
	require.Positive(t, clockPulses)
	require.NotEqual(t, -1, presentPulses)
	require.True(t, presentSettled)
	require.True(t, doneSettled)
	require.Greater(t, clockPulses, presentPulses)
}

// generatedRecursiveInstances locates unique runtime owners without relying on
// emission order.
func generatedRecursiveInstances(
	t *testing.T,
	e *emitter,
) (*recursiveMachine, *instance) {
	t.Helper()
	var machine *recursiveMachine
	var clock *instance
	for _, owner := range e.owner {
		if owner == nil {
			continue
		}
		switch component := owner.comp.(type) {
		case *recursiveMachine:
			machine = component
		case *clockDiv:
			clock = owner
		}
	}
	require.NotNil(t, machine)
	require.NotNil(t, clock)
	return machine, clock
}

// TestRecursiveMachineExecutesDistinctPlans proves the emitted circuit runs
// plans with different arities, operators, and recursive call structures.
func TestRecursiveMachineExecutesDistinctPlans(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		source    string
		function  string
		arguments []int
		want      int
	}{
		{
			name:      "factorial",
			source:    factorialRecursiveSource,
			function:  "factorial",
			arguments: []int{5},
			want:      120,
		},
		{
			name: "greatest common divisor",
			source: `package p
func gcd(a, b int) int {
	if b == 0 {
		return a
	}
	return gcd(b, a%b)
}`,
			function:  "gcd",
			arguments: []int{48, 18},
			want:      6,
		},
		{
			name: "two call sites and unary minus",
			source: `package p
func distance(n int) int {
	if n == 0 {
		return 0
	}
	if n < 0 {
		return 1 + distance(-n-1)
	}
	return 1 + distance(n-1)
}`,
			function:  "distance",
			arguments: []int{-4},
			want:      4,
		},
		{
			name: "Fibonacci",
			source: `package p
func fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	return fibonacci(n-1) + fibonacci(n-2)
}`,
			function:  "fibonacci",
			arguments: []int{6},
			want:      8,
		},
		{
			name: "boolean parameter and result",
			source: `package p
func carry(n int, value bool) bool {
	if n == 0 {
		return value
	}
	return carry(n-1, value)
}`,
			function:  "carry",
			arguments: []int{4, -1},
			want:      -1,
		},
		{
			name: "phi merge",
			source: `package p
func weighted(n int, double bool) int {
	if n <= 0 {
		return 0
	}
	amount := 1
	if double {
		amount = 2
	}
	return amount + weighted(n-1, double)
}`,
			function:  "weighted",
			arguments: []int{4, 1},
			want:      8,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := planRecursiveSource(t, tc.source, tc.function)
			rig := newRecursiveMachineTestRig(
				t,
				program,
				tc.arguments...,
			)
			require.Equal(t, tc.want, rig.run(500))
		})
	}
}

// TestRecursiveMachineLowersOperators proves uncovered binary forms execute
// through the emitted circuit with their planned operand ordering intact.
func TestRecursiveMachineLowersOperators(t *testing.T) {
	t.Parallel()
	const quotientSource = `package p
func quotient(depth, dividend, divisor int) int {
	if depth == 0 {
		return dividend / divisor
	}
	return quotient(depth-1, dividend, divisor)
}`
	const atLeastSource = `package p
func atLeast(depth, left, right int) bool {
	if depth == 0 {
		return left >= right
	}
	return atLeast(depth-1, left, right)
}`
	tests := []struct {
		name      string
		source    string
		function  string
		arguments []int
		operator  token.Token
		want      int
	}{
		{
			name:      "division",
			source:    quotientSource,
			function:  "quotient",
			arguments: []int{1, 84, 7},
			operator:  token.QUO,
			want:      12,
		},
		{
			name:      ">= true",
			source:    atLeastSource,
			function:  "atLeast",
			arguments: []int{1, 8, 8},
			operator:  token.GEQ,
			want:      1,
		},
		{
			name:      ">= false",
			source:    atLeastSource,
			function:  "atLeast",
			arguments: []int{1, 7, 8},
			operator:  token.GEQ,
			want:      -1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := planRecursiveSource(
				t,
				test.source,
				test.function,
			)
			matched := false
			for _, instruction := range program.instructions {
				if instruction.operator != test.operator {
					continue
				}
				matched = true
			}
			require.True(t, matched)

			rig := newRecursiveMachineTestRig(
				t,
				program,
				test.arguments...,
			)
			require.Equal(t, test.want, rig.run(200))
		})
	}
}

// TestRecursiveMachineFoldsLiteralBranches proves emitted branch actions keep
// only the source-selected arm for both Boolean constants.
func TestRecursiveMachineFoldsLiteralBranches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		source    string
		condition int
	}{
		{
			name: "true",
			source: `package p
func count(n int) int {
	if n == 0 {
		return 0
	}
	if true {
		return count(n-1) + 1
	}
	return -100
}`,
			condition: 1,
		},
		{
			name: "false",
			source: `package p
func count(n int) int {
	if n == 0 {
		return 0
	}
	if false {
		return -100
	}
	return count(n-1) + 1
}`,
			condition: -1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			program := planRecursiveSource(t, test.source, "count")
			matched := false
			for _, instruction := range program.instructions {
				if instruction.op != recursiveOpBranch ||
					!instruction.x.isConstant ||
					instruction.x.constant != test.condition {
					continue
				}
				matched = true
			}
			require.True(t, matched)

			rig := newRecursiveMachineTestRig(t, program, 4)
			require.Equal(t, 4, rig.run(200))
		})
	}
}

// TestRecursiveMachineCompletesAtMaximumStackDepth proves twelve suspended
// callers retain distinct values while every documented stack frame is in use.
func TestRecursiveMachineCompletesAtMaximumStackDepth(t *testing.T) {
	t.Parallel()
	const maxNestedCalls = 12
	require.Equal(t, maxNestedCalls, recursiveMachineDepth)

	program := foldingRecursiveProgram(t)
	rig := newRecursiveMachineTestRig(t, program, maxNestedCalls)

	require.Equal(t, 78, rig.run(200))
	require.Equal(t, recursiveModeDone, rig.mode())
}

// TestRecursiveMachineStopsOnStackOverflow proves a thirteenth suspended call
// publishes no result, exposes only overflow status, and remains terminal.
func TestRecursiveMachineStopsOnStackOverflow(t *testing.T) {
	t.Parallel()
	const overflowNestedCalls = 13
	require.Equal(t, overflowNestedCalls-1, recursiveMachineDepth)

	program := foldingRecursiveProgram(t)
	rig := newRecursiveMachineTestRig(t, program, overflowNestedCalls)
	for range 20 {
		rig.tick(0)
	}
	for range 200 {
		rig.action()
		if rig.mode() == recursiveModeOverflow {
			requireRecursiveMachineOverflow(t, rig)

			rig.stageInternalResult(37)
			require.Equal(t, 37, rig.internalResultValue())
			rig.tick(0)
			requireRecursiveMachineOverflow(t, rig)

			terminalState := rig.machineState()
			for range 3 {
				rig.action()
				require.Equal(t, terminalState, rig.machineState())
				requireRecursiveMachineOverflow(t, rig)
			}
			return
		}
	}
	t.Fatal("recursive machine did not overflow")
}

// requireRecursiveMachineOverflow protects the complete visible and internal
// overflow contract.
func requireRecursiveMachineOverflow(
	t *testing.T,
	rig *recursiveMachineTestRig,
) {
	t.Helper()
	require.Equal(t, recursiveModeOverflow, rig.mode())
	require.False(t, rig.resultPresent())
	require.False(t, rig.statusVisible("RUNNING"))
	require.False(t, rig.statusVisible("DONE"))
	require.True(t, rig.statusVisible("STACK OVERFLOW"))
	rig.requireActivityLampsDark()
}

// foldingRecursiveProgram supplies one depth-consuming plan shared by overflow
// and footprint tests.
func foldingRecursiveProgram(t *testing.T) *recursiveProgram {
	t.Helper()
	return planRecursiveSource(t, `package p
func fold(n int) int {
	if n == 0 {
		return 0
	}
	return n + fold(n-1)
}`, "fold")
}

// TestRecursiveMachineFootprintCoversEmission proves the conservative routing
// rectangle contains every combinator and panel emitted by planned machines.
func TestRecursiveMachineFootprintCoversEmission(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source, function string
	}{
		{
			name:     "wide frame",
			source:   factorialRecursiveSource,
			function: "factorial",
		},
		{
			name: "frame lamp sets minimum width",
			source: `package p
func recurse() int {
	return recurse()
}`,
			function: "recurse",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			program := planRecursiveSource(t, tc.source, tc.function)
			machine := newRecursiveMachine(program)
			declared := machine.footprint(dirEast)
			for _, ent := range emitModule(machine).entities {
				for _, cell := range entityCells(ent) {
					require.GreaterOrEqual(t, cell.X, 0)
					require.GreaterOrEqual(t, cell.Y, 0)
					require.Lessf(
						t,
						cell.X,
						declared.width,
						"%s at %v lies outside the footprint",
						ent.Name,
						cell,
					)
					require.Lessf(
						t,
						cell.Y,
						declared.height,
						"%s at %v lies outside the footprint",
						ent.Name,
						cell,
					)
				}
			}
		})
	}
}
