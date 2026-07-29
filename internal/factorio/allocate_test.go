// This file tests finite signal allocation for public nets.
package factorio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAllocateSignalsBanks proves the two-bank allocation. A parameter takes
// its letter from the input bank by signature index (signal-A is index 0),
// every other net takes the next item signal from the intermediate bank in
// encounter order, and each bank is a hard wall that fails when exhausted.
func TestAllocateSignalsBanks(t *testing.T) {
	t.Parallel()
	param := func(i int) *netlistNet {
		return &netlistNet{isInput: true, inputIndex: i}
	}
	inter := func() *netlistNet { return &netlistNet{} }

	t.Run("inputs by index, intermediates in order", func(t *testing.T) {
		p1, mid, p0 := param(1), inter(), param(0)
		require.NoError(t, allocateSignals([]*netlistNet{p1, mid, p0}))
		require.Equal(t, signalID{Type: "virtual", Name: "signal-B"}, p1.signal)
		require.Equal(t, signalID{Type: "virtual", Name: "signal-A"}, p0.signal)
		require.Equal(t, intermediateSignals[0], mid.signal)
	})

	t.Run("intermediates keep encounter order", func(t *testing.T) {
		a, b := inter(), inter()
		require.NoError(t, allocateSignals([]*netlistNet{a, b}))
		require.Equal(t, intermediateSignals[0], a.signal)
		require.Equal(t, intermediateSignals[1], b.signal)
	})

	t.Run("input bank wall", func(t *testing.T) {
		require.Error(t, allocateSignals([]*netlistNet{param(len(inputSignals))}))
	})

	t.Run("intermediate bank wall", func(t *testing.T) {
		nets := make([]*netlistNet, len(intermediateSignals)+1)
		for i := range nets {
			nets[i] = inter()
		}
		require.Error(t, allocateSignals(nets))
	})
}
