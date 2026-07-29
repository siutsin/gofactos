// This file protects the JSON schema emitted for Factorio entities and wires.
package factorio

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNegateComparator covers every branch of the comparator negation that the
// compare module uses for its false arm, including the inverse forms and the
// passthrough default that the test cases do not exercise.
func TestNegateComparator(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		op, want string
	}{
		{"<", "≥"},
		{">", "≤"},
		{"=", "≠"},
		{"!=", "="},
		{"<=", ">"},
		{">=", "<"},
		{"≥", "<"},
		{"≤", ">"},
		{"≠", "="},
		{"unknown", "unknown"},
	} {
		t.Run(tc.op, func(t *testing.T) {
			if got := negateComparator(tc.op); got != tc.want {
				t.Errorf("negateComparator(%q) = %q, want %q", tc.op, got, tc.want)
			}
		})
	}
}

// TestDeciderConditionCompareTypeJSON verifies that compound joins use the
// Factorio 2.0 field and that a first-row empty join is omitted.
func TestDeciderConditionCompareTypeJSON(t *testing.T) {
	t.Parallel()
	one := 1
	signalA := signalID{Type: "virtual", Name: "signal-A"}
	conditions := []deciderCondition{
		{
			FirstSignal: &signalA,
			Comparator:  "=",
			Constant:    &one,
		},
		{
			FirstSignal: &signalA,
			Comparator:  ">",
			Constant:    &one,
			CompareType: "and",
		},
	}

	encoded, err := json.Marshal(conditions)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{
			"first_signal": {"type": "virtual", "name": "signal-A"},
			"comparator": "=",
			"constant": 1
		},
		{
			"first_signal": {"type": "virtual", "name": "signal-A"},
			"comparator": ">",
			"constant": 1,
			"compare_type": "and"
		}
	]`, string(encoded))

	var decoded []deciderCondition
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Len(t, decoded, 2)
	assert.Empty(t, decoded[0].CompareType)
	assert.Equal(t, "and", decoded[1].CompareType)
}

// TestActivityLampJSON pins the Factorio 2.x control behaviour and static
// green colour used by recursive-machine activity lamps.
func TestActivityLampJSON(t *testing.T) {
	t.Parallel()
	lamp := entity{
		EntityNumber: 7,
		Name:         smallLampEntityName,
		Position:     position{X: 2.5, Y: 3.5},
		Colour:       greenLampColour(),
		ControlBehavior: &controlBehavior{
			CircuitEnabled: true,
			CircuitCondition: &circuitCondition{
				FirstSignal: &privateInc,
				Comparator:  "=",
				Constant:    1,
			},
		},
	}

	encoded, err := json.Marshal(lamp)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"entity_number": 7,
		"name": "small-lamp",
		"position": {"x": 2.5, "y": 3.5},
		"color": {"r": 0, "g": 1, "b": 0, "a": 1},
		"control_behavior": {
			"circuit_enabled": true,
			"circuit_condition": {
				"first_signal": {
					"type": "virtual",
					"name": "signal-check"
				},
				"comparator": "=",
				"constant": 1
			}
		}
	}`, string(encoded))
}

// TestActivityLampOccupiesOneTile proves lamp geometry does not inherit the
// default two-tile combinator footprint.
func TestActivityLampOccupiesOneTile(t *testing.T) {
	t.Parallel()
	lamp := entity{
		Name:     smallLampEntityName,
		Position: position{X: 2.5, Y: 3.5},
	}

	assert.Equal(t, []tile{{X: 2, Y: 3}}, entityCells(lamp))
}
