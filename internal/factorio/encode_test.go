// This file keeps encoded blueprints stable and importable by Factorio.
package factorio

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncodeDecodeRoundTrip verifies that encoding a blueprint and
// decoding the result produces an identical structure.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	original := &BlueprintWrapper{
		Blueprint: Blueprint{
			Item:    "blueprint",
			Label:   "test",
			Version: blueprintVersion,
			Entities: []entity{
				{
					EntityNumber: 1,
					Name:         "arithmetic-combinator",
					Position:     position{X: 0.5, Y: 0.5},
					ControlBehavior: &controlBehavior{
						ArithmeticConditions: &arithmeticConditions{
							FirstSignal:  &signalID{Type: "virtual", Name: "signal-A"},
							Operation:    "+",
							SecondSignal: &signalID{Type: "virtual", Name: "signal-B"},
							OutputSignal: &signalID{Type: "virtual", Name: "signal-C"},
						},
					},
				},
				{
					EntityNumber: 2,
					Name:         constCombinatorName,
					Position:     position{X: 3.5, Y: 0.5},
					ControlBehavior: &controlBehavior{
						Sections: &constantCombinatorSections{
							Sections: []logisticSection{{
								Index: 1,
								Filters: []constantFilter{{
									Index: 1, Type: "virtual",
									Name: "signal-B", Quality: "normal",
									Comparator: "=", Count: 7,
								}},
							}},
						},
					},
				},
			},
			Wires: []wire{{1, connectorGreenIn, 2, connectorGreenIn}},
		},
	}

	encoded, err := Encode(original)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)
	require.Equal(t, byte('0'), encoded[0])
	decoded := decodeBlueprint(t, encoded)

	assert.Equal(t, original, decoded)
}

// decodeBlueprint independently restores the value produced by Encode.
func decodeBlueprint(t *testing.T, encoded string) *BlueprintWrapper {
	t.Helper()
	compressed, err := base64.StdEncoding.DecodeString(encoded[1:])
	require.NoError(t, err)
	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	data, readErr := io.ReadAll(zr)
	closeErr := zr.Close()
	require.NoError(t, errors.Join(readErr, closeErr))

	var decoded BlueprintWrapper
	require.NoError(t, json.Unmarshal(data, &decoded))
	return &decoded
}
