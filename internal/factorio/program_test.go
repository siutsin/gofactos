// This file proves recursive SSA plans execute with source-level semantics.
package factorio

import (
	"fmt"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// TestRecursiveProgramRunsGenericRecursion proves one planner executes several
// recursive algorithms and both supported value kinds.
func TestRecursiveProgramRunsGenericRecursion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		fn     string
		args   []int
		want   int
	}{
		{
			name: "Fibonacci",
			source: `package main
func fibonacci(n int) int {
	if n <= 1 { return n }
	return fibonacci(n-1) + fibonacci(n-2)
}
`,
			fn:   "fibonacci",
			args: []int{8},
			want: 21,
		},
		{
			name: "factorial",
			source: `package main
func factorial(n int) int {
	if n <= 1 { return 1 }
	return n * factorial(n-1)
}
`,
			fn:   "factorial",
			args: []int{6},
			want: 720,
		},
		{
			name: "greatest common divisor",
			source: `package main
func gcd(a, b int) int {
	if b == 0 { return a }
	return gcd(b, a%b)
}
`,
			fn:   "gcd",
			args: []int{48, 18},
			want: 6,
		},
		{
			name: "recursive boolean",
			source: `package main
func flip(n int, value bool) bool {
	if n == 0 { return value }
	next := n - 1
	if value { return flip(next, false) }
	return flip(next, true)
}
`,
			fn:   "flip",
			args: []int{5, 1},
			want: -1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := planRecursiveSource(t, test.source, test.fn)
			got, err := runRecursiveTestProgram(program, test.args...)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
			assertRecursivePCs(t, program)
		})
	}
}

// TestRecursiveProgramDefaultsConstantBranchConditions proves SSA's untyped
// boolean branch constants are classified as bool for planning and execution.
func TestRecursiveProgramDefaultsConstantBranchConditions(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, writeTempGo(t, `package main
func count(n int) int {
	if n == 0 { return 0 }
	if true { return count(n-1) + 1 }
	return -1
}
`), "count")

	found := false
	for _, block := range fn.Blocks {
		for _, instruction := range block.Instrs {
			branch, ok := instruction.(*ssa.If)
			if !ok || branch.Cond.Type() != types.Typ[types.UntypedBool] {
				continue
			}
			_, found = branch.Cond.(*ssa.Const)
		}
	}
	require.True(t, found)

	program, err := planRecursiveProgram(fn)
	require.NoError(t, err)
	got, err := runRecursiveTestProgram(program, 5)
	require.NoError(t, err)
	require.Equal(t, 5, got)
}

// TestRecursiveProgramAcceptsScalarAliases proves aliases retain parameter,
// result, input encoding, display, and recursive execution behaviour.
func TestRecursiveProgramAcceptsScalarAliases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		source      string
		fn          string
		args        []int
		params      map[string]int
		want        int
		wantInputs  []int
		wantDisplay string
	}{
		{
			name: "integer parameter and result",
			source: `package main
type Count = int
func triangular(n Count) Count {
	if n == 0 { return 0 }
	return n + triangular(n-1)
}
`,
			fn:          "triangular",
			args:        []int{4},
			params:      map[string]int{"n": 4},
			want:        10,
			wantInputs:  []int{4},
			wantDisplay: "digitDisplay",
		},
		{
			name: "boolean parameter and result",
			source: `package main
type Count = int
type Flag = bool
func flip(n Count, value Flag) Flag {
	if n == 0 { return value }
	if value { return flip(n-1, false) }
	return flip(n-1, true)
}
`,
			fn:          "flip",
			args:        []int{3, -1},
			params:      map[string]int{"n": 3, "value": 0},
			want:        1,
			wantInputs:  []int{3, -1},
			wantDisplay: "boolDisplay",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := planRecursiveSource(t, test.source, test.fn)
			got, err := runRecursiveTestProgram(program, test.args...)
			require.NoError(t, err)
			require.Equal(t, test.want, got)

			selected, err := selectFunc(program.fn, WithParams(test.params))
			require.NoError(t, err)
			require.Equal(t, test.wantInputs, selectedConstValues(selected))
			require.Equal(t, test.wantDisplay, selectedDisplayKind(selected))
		})
	}
}

// TestRecursiveProgramRejectsDefinedScalarTypes proves unaliasing does not
// widen the recursive subset to user-defined integer types.
func TestRecursiveProgramRejectsDefinedScalarTypes(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(t, writeTempGo(t, `package main
type Count int
func countdown(n Count) Count {
	if n == 0 { return 0 }
	return countdown(n-1)
}
`), "countdown")

	program, err := planRecursiveProgram(fn)
	require.Nil(t, program)
	require.ErrorContains(t, err, "parameter 1 has type")
	require.ErrorContains(t, err, "want int or bool")

	selected, err := selectFunc(fn)
	require.Nil(t, selected)
	require.ErrorContains(t, err, "unsupported parameter type")
}

// selectedConstValues exposes baked inputs so planner tests can verify option
// propagation without relying on entity JSON.
func selectedConstValues(selected *selected) []int {
	var values []int
	for _, instance := range selected.insts {
		if source, ok := instance.comp.(*constSrc); ok {
			values = append(values, source.value)
		}
	}
	return values
}

// selectedDisplayKind confirms result typing chooses the intended user-facing
// readout.
func selectedDisplayKind(selected *selected) string {
	for _, instance := range selected.insts {
		switch instance.comp.(type) {
		case *boolDisplay:
			return "boolDisplay"
		case *digitDisplay:
			return "digitDisplay"
		}
	}
	return ""
}

// TestRecursiveProgramAppliesPhiMovesOnEdges proves a recursive CFG merge is
// compiled as edge-specific parallel assignments rather than a special case.
func TestRecursiveProgramAppliesPhiMovesOnEdges(t *testing.T) {
	t.Parallel()
	program := planRecursiveSource(t, `package main
func weighted(n int, double bool) int {
	if n <= 0 { return 0 }
	amount := 1
	if double { amount = 2 }
	return amount + weighted(n-1, double)
}
`, "weighted")

	hasMoves := false
	for _, instruction := range program.instructions {
		hasMoves = hasMoves || len(instruction.moves) > 0 ||
			len(instruction.alternateMoves) > 0
	}
	require.True(t, hasMoves)

	got, err := runRecursiveTestProgram(program, 3, 1)
	require.NoError(t, err)
	require.Equal(t, 6, got)
	got, err = runRecursiveTestProgram(program, 3, -1)
	require.NoError(t, err)
	require.Equal(t, 3, got)
}

// TestRecursiveMachineAppliesPhiMovesInParallel proves emitted edge moves read
// both old values before either destination changes.
func TestRecursiveMachineAppliesPhiMovesInParallel(t *testing.T) {
	t.Parallel()
	program := planRecursiveSource(t, `package main
func swap(n, a, b int) int {
	if n <= 0 { return a - b }
	if a != b { a, b = b, a }
	return swap(n-1, a, b)
}
`, "swap")

	hasParallelMoves := false
	for _, instruction := range program.instructions {
		hasParallelMoves = hasParallelMoves || len(instruction.moves) >= 2 ||
			len(instruction.alternateMoves) >= 2
	}
	require.True(t, hasParallelMoves)

	rig := newRecursiveMachineTestRig(t, program, 1, 2, 7)
	require.Equal(t, 5, rig.run(500))
}

// TestRecursiveProgramAcceptsMaximumSlots proves planning and blueprint import
// accept the exact ten-slot boundary rejected by the eleven-slot case.
func TestRecursiveProgramAcceptsMaximumSlots(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, `package main
func f(n int) int {
	if n <= 0 { return 0 }
	a := n + 1
	b := a + 1
	c := b + 1
	d := c + 1
	e := d + 1
	return e + f(n-1)
}
`)
	fn := parseTestFile(t, path, "f")
	program, err := planRecursiveProgram(fn)
	require.NoError(t, err)

	require.Equal(t, maxRecursiveProgramSlots, program.slotCount)
	generate(
		t,
		path,
		"f",
		WithParams(map[string]int{"n": 3}),
	).validateWithDraftsman()
}

// TestRecursiveProgramRejectsUnsupportedCapabilities pins the boundary around
// the generic direct-self-recursion subset.
func TestRecursiveProgramRejectsUnsupportedCapabilities(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		source  string
		fn      string
		wantErr string
	}{
		{
			name: "no self call",
			source: `package main
func f(n int) int { return n + 1 }
`,
			fn:      "f",
			wantErr: "expected at least one direct self call",
		},
		{
			name: "helper call",
			source: `package main
func helper(n int) int { return n }
func f(n int) int {
	if n == 0 { return 0 }
	return f(n-1) + helper(n)
}
`,
			fn:      "f",
			wantErr: "call to helper is unsupported",
		},
		{
			name: "control flow loop",
			source: `package main
func f(n int) int {
	for n > 0 { n-- }
	return f(n)
}
`,
			fn:      "f",
			wantErr: "control-flow loops are unsupported",
		},
		{
			name: "unsupported binary operator",
			source: `package main
func f(n int) int {
	if n == 0 { return 0 }
	return (n & 1) + f(n-1)
}
`,
			fn:      "f",
			wantErr: "binary operator & is unsupported",
		},
		{
			name: "unsupported unary operator",
			source: `package main
func f(n int, value bool) bool {
	if n == 0 { return value }
	return !f(n-1, value)
}
`,
			fn:      "f",
			wantErr: "unary operator ! is unsupported",
		},
		{
			name: "non ordinary self call",
			source: `package main
func f(n int) int {
	if n == 0 { return 0 }
	go f(n-1)
	return n
}
`,
			fn:      "f",
			wantErr: "non-ordinary calls are unsupported",
		},
		{
			name: "non int parameter",
			source: `package main
func f(n int32) int32 {
	if n == 0 { return 0 }
	return f(n-1)
}
`,
			fn:      "f",
			wantErr: "parameter 1 has type int32; want int or bool",
		},
		{
			name: "too many slots",
			source: `package main
func f(n int) int {
	if n <= 0 { return 0 }
	a := n + 1
	b := a + 1
	c := b + 1
	d := c + 1
	e := d + 1
	g := e + 1
	return g + f(n-1)
}
`,
			fn:      "f",
			wantErr: "value slots; limit is 10",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := parseTestFile(
				t,
				writeTempGo(t, test.source),
				test.fn,
			)
			_, err := planRecursiveProgram(fn)
			require.Error(t, err)
			require.ErrorContains(
				t,
				err,
				"select: unsupported recursive body "+test.fn+":",
			)
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

// TestRecursiveProgramRejectsIrreducibleControlFlow proves the planner rejects
// a CFG cycle even when no cycle entry dominates the other.
func TestRecursiveProgramRejectsIrreducibleControlFlow(t *testing.T) {
	t.Parallel()
	fn := parseTestFile(
		t,
		"../testdata/negative/recursiveirreducible.go",
		"recursiveIrreducible",
	)
	require.Nil(t, loopHeader(fn))
	require.True(t, hasControlFlowCycle(fn))

	program, err := planRecursiveProgram(fn)
	require.Nil(t, program)
	require.ErrorContains(t, err, "control-flow loops are unsupported")
}

// TestRecursiveProgramIgnoresDebugReferences proves source-position metadata
// does not consume slots or PCs and cannot change recursion semantics.
func TestRecursiveProgramIgnoresDebugReferences(t *testing.T) {
	t.Parallel()
	path := writeTempGo(t, `package main
func factorial(n int) int {
	if n <= 1 { return 1 }
	return n * factorial(n-1)
}
`)
	loaded, err := packages.Load(
		&packages.Config{Mode: packages.LoadSyntax},
		path,
	)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	ssaProgram, ssaPackages := ssautil.Packages(
		loaded,
		ssa.SanityCheckFunctions|ssa.GlobalDebug,
	)
	ssaProgram.Build()
	fn, ok := ssaPackages[0].Members["factorial"].(*ssa.Function)
	require.True(t, ok)

	debugRefs := 0
	for _, block := range fn.Blocks {
		for _, instruction := range block.Instrs {
			if _, ok := instruction.(*ssa.DebugRef); ok {
				debugRefs++
			}
		}
	}
	require.Positive(t, debugRefs)
	program, err := planRecursiveProgram(fn)
	require.NoError(t, err)
	got, err := runRecursiveTestProgram(program, 5)
	require.NoError(t, err)
	require.Equal(t, 120, got)
}

// planRecursiveSource turns concise inline test cases into validated plans for
// semantic tests.
func planRecursiveSource(
	t *testing.T,
	source string,
	name string,
) *recursiveProgram {
	t.Helper()
	fn := parseTestFile(t, writeTempGo(t, source), name)
	program, err := planRecursiveProgram(fn)
	require.NoError(t, err)
	return program
}

// recursiveTestInstruction centralises PC bounds so the reference interpreter
// fails invalid plans consistently.
func recursiveTestInstruction(
	program *recursiveProgram,
	pc int,
) (recursiveInstruction, bool) {
	if pc < 1 || pc > len(program.instructions) {
		return recursiveInstruction{}, false
	}
	return program.instructions[pc-1], true
}

// assertRecursivePCs protects the continuation pairing required by calls and
// resumes.
func assertRecursivePCs(t *testing.T, program *recursiveProgram) {
	t.Helper()
	require.Equal(t, 1, program.entry)
	callCount := 0
	resumeCount := 0
	for index, instruction := range program.instructions {
		require.Equal(t, index+1, instruction.pc)
		if instruction.op == recursiveOpCall {
			callCount++
			resume, ok := recursiveTestInstruction(
				program,
				instruction.continuation,
			)
			require.True(t, ok)
			require.Equal(t, recursiveOpResume, resume.op)
			require.Equal(t, instruction.dest, resume.dest)
		}
		if instruction.op == recursiveOpResume {
			resumeCount++
		}
	}
	require.Positive(t, callCount)
	require.Equal(t, callCount, resumeCount)
}

type recursiveTestFrame struct {
	pc    int
	slots []int
}

type recursiveTestMachine struct {
	program  *recursiveProgram
	frames   []recursiveTestFrame
	returned int
	done     bool
	result   int
}

// runRecursiveTestProgram provides a circuit-independent semantic oracle for
// compiled recursive plans.
func runRecursiveTestProgram(
	program *recursiveProgram,
	args ...int,
) (int, error) {
	if len(args) != len(program.params) {
		return 0, fmt.Errorf(
			"got %d arguments; want %d",
			len(args),
			len(program.params),
		)
	}
	slots := make([]int, program.slotCount)
	for index, value := range args {
		slots[program.params[index]] = value
	}
	machine := recursiveTestMachine{
		program: program,
		frames: []recursiveTestFrame{{
			pc:    program.entry,
			slots: slots,
		}},
	}
	for range 100_000 {
		if err := machine.step(); err != nil {
			return 0, err
		}
		if machine.done {
			return machine.result, nil
		}
	}
	return 0, fmt.Errorf("recursive test program did not terminate")
}

// step executes one planned instruction so failures identify the exact opcode
// that diverges from Go semantics.
func (m *recursiveTestMachine) step() error {
	frame := &m.frames[len(m.frames)-1]
	instruction, ok := recursiveTestInstruction(m.program, frame.pc)
	if !ok {
		return fmt.Errorf("invalid recursive PC %d", frame.pc)
	}
	switch instruction.op {
	case recursiveOpBinary:
		value, err := recursiveTestBinary(instruction, frame.slots)
		if err != nil {
			return err
		}
		frame.slots[instruction.dest] = value
		frame.pc = instruction.target
	case recursiveOpUnary:
		value := recursiveTestOperand(instruction.x, frame.slots)
		frame.slots[instruction.dest] = applyArith("*", value, -1)
		frame.pc = instruction.target
	case recursiveOpBranch:
		m.branch(frame, instruction)
	case recursiveOpJump:
		recursiveTestMove(frame.slots, instruction.moves)
		frame.pc = instruction.target
	case recursiveOpCall:
		return m.call(frame, instruction)
	case recursiveOpResume:
		frame.slots[instruction.dest] = m.returned
		frame.pc = instruction.target
	case recursiveOpReturn:
		m.ret(frame, instruction)
	default:
		return fmt.Errorf("unsupported recursive opcode %d", instruction.op)
	}
	return nil
}

// recursiveTestBinary mirrors supported circuit arithmetic and comparison in
// the reference interpreter.
func recursiveTestBinary(
	instruction recursiveInstruction,
	slots []int,
) (int, error) {
	entry, ok := binOpMap[instruction.operator]
	if !ok {
		return 0, fmt.Errorf(
			"unsupported recursive operator %s",
			instruction.operator,
		)
	}
	x := recursiveTestOperand(instruction.x, slots)
	y := recursiveTestOperand(instruction.y, slots)
	if entry.entityName == arithCombinatorName {
		return applyArith(entry.operation, x, y), nil
	}
	if evalCompare(entry.operation, x, y) {
		return 1, nil
	}
	return -1, nil
}

// recursiveTestOperand gives constants and frame slots one evaluation path.
func recursiveTestOperand(operand recursiveOperand, slots []int) int {
	if operand.isConstant {
		return operand.constant
	}
	return slots[operand.slot]
}

// recursiveTestMove preserves parallel phi semantics by staging every source
// before updating a destination.
func recursiveTestMove(slots []int, moves []recursiveMove) {
	values := make([]int, len(moves))
	for index, move := range moves {
		values[index] = recursiveTestOperand(move.source, slots)
	}
	for index, move := range moves {
		slots[move.dest] = values[index]
	}
}

// branch applies edge moves on only the selected control-flow path.
func (m *recursiveTestMachine) branch(
	frame *recursiveTestFrame,
	instruction recursiveInstruction,
) {
	if recursiveTestOperand(instruction.x, frame.slots) == 1 {
		recursiveTestMove(frame.slots, instruction.moves)
		frame.pc = instruction.target
		return
	}
	recursiveTestMove(frame.slots, instruction.alternateMoves)
	frame.pc = instruction.alternate
}

// call models frame creation and continuation storage for the semantic oracle.
func (m *recursiveTestMachine) call(
	frame *recursiveTestFrame,
	instruction recursiveInstruction,
) error {
	if len(m.frames) >= 1_024 {
		return fmt.Errorf("recursive test stack overflow")
	}
	childSlots := make([]int, m.program.slotCount)
	for index, argument := range instruction.args {
		childSlots[m.program.params[index]] = recursiveTestOperand(
			argument,
			frame.slots,
		)
	}
	frame.pc = instruction.continuation
	m.frames = append(m.frames, recursiveTestFrame{
		pc:    instruction.target,
		slots: childSlots,
	})
	return nil
}

// ret distinguishes root completion from a value returning to its caller.
func (m *recursiveTestMachine) ret(
	frame *recursiveTestFrame,
	instruction recursiveInstruction,
) {
	value := recursiveTestOperand(instruction.x, frame.slots)
	if len(m.frames) == 1 {
		m.done = true
		m.result = value
		return
	}
	m.returned = value
	m.frames = m.frames[:len(m.frames)-1]
}
