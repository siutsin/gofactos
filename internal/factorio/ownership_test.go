// This file verifies compiled blueprints own their mutable signal values.
package factorio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCompileOwnsPrivateSignals prevents separate compilations from sharing
// mutable pointers to the package's private signal identities.
func TestCompileOwnsPrivateSignals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path, function string
	}{
		{name: "branch", path: "../testdata/abs.go", function: "abs"},
		{name: "loop", path: "../testdata/fori.go", function: "forI"},
		{
			name:     "recursion",
			path:     "../testdata/recursive/factorial.go",
			function: "factorial",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseTestFile(t, tc.path, tc.function)
			first, err := Compile(fn)
			require.NoError(t, err)
			second, err := Compile(fn)
			require.NoError(t, err)
			assertDisjointSignalPointers(
				t,
				first.Blueprint.Entities,
				second.Blueprint.Entities,
			)
		})
	}
}

// assertDisjointSignalPointers checks every signal pointer emitted into JSON.
func assertDisjointSignalPointers(t *testing.T, first, second []entity) {
	t.Helper()
	owners := make(map[*signalID]int)
	for _, entity := range first {
		for _, signal := range entitySignalPointers(entity) {
			if signal != nil {
				owners[signal] = entity.EntityNumber
			}
		}
	}
	for _, entity := range second {
		for _, signal := range entitySignalPointers(entity) {
			if firstEntity, ok := owners[signal]; ok {
				t.Errorf(
					"entity %d shares a signal pointer with first entity %d",
					entity.EntityNumber,
					firstEntity,
				)
			}
		}
	}
}

// entitySignalPointers gathers every mutable signal pointer encoded by an
// entity and its control behaviour.
func entitySignalPointers(entity entity) []*signalID {
	signals := []*signalID{entity.Icon}
	behavior := entity.ControlBehavior
	if behavior == nil {
		return signals
	}
	if conditions := behavior.ArithmeticConditions; conditions != nil {
		signals = append(
			signals,
			conditions.FirstSignal,
			conditions.SecondSignal,
			conditions.OutputSignal,
		)
	}
	if conditions := behavior.DeciderConditions; conditions != nil {
		for _, condition := range conditions.Conditions {
			signals = append(
				signals,
				condition.FirstSignal,
				condition.SecondSignal,
			)
		}
		for _, output := range conditions.Outputs {
			signals = append(signals, output.Signal)
		}
	}
	if condition := behavior.CircuitCondition; condition != nil {
		signals = append(signals, condition.FirstSignal)
	}
	for _, parameter := range behavior.Parameters {
		signals = append(signals, parameter.Icon)
		if parameter.Condition != nil {
			signals = append(signals, parameter.Condition.FirstSignal)
		}
	}
	return signals
}
