// This file proves the numeric readout preserves every supported decimal digit.
package factorio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDigitDisplayExtractsDigits proves the numeric chain pulls each decimal
// digit: 152 yields ones 2, tens 5, hundreds 1, on the modulo combinators that
// drive the per-digit panels.
func TestDigitDisplayExtractsDigits(t *testing.T) {
	t.Parallel()
	cv := newInstance(newConstSrc(152))
	dd := newInstance(&digitDisplay{})
	insts := []*instance{cv, dd}
	nets := []*netlistNet{connect(cv.port("out"), dd.port("in"))}
	require.NoError(t, allocateSignals(nets))

	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))

	s := simulate(t, e.entities, e.wires, 50)
	digits := moduloOutputs(t, e, s)
	// ones..ten-millions; higher digits in 00000152 are zero.
	require.Equal(t, []int{2, 5, 1, 0, 0, 0, 0, 0}, digits)
}

// TestDigitDisplayNegativeInputIsNonDisplayable documents the current readout
// limit: negative remainders do not match any 0..9 digit-ladder message.
func TestDigitDisplayNegativeInputIsNonDisplayable(t *testing.T) {
	t.Parallel()
	cv := newInstance(newConstSrc(-152))
	dd := newInstance(&digitDisplay{})
	insts := []*instance{cv, dd}
	nets := []*netlistNet{connect(cv.port("out"), dd.port("in"))}
	require.NoError(t, allocateSignals(nets))

	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))

	s := simulate(t, e.entities, e.wires, 50)
	digits := moduloOutputs(t, e, s)
	require.Equal(t, []int{-2, -5, -1, 0, 0, 0, 0, 0}, digits)
	for _, digit := range digits[:3] {
		require.Negative(t, digit)
	}
}

// TestDigitDisplayLayoutStacksRows proves the numeric readout uses the intended
// calculator shape: vertical dividers on top, vertical modulo digit extractors
// directly above the display panels.
func TestDigitDisplayLayoutStacksRows(t *testing.T) {
	t.Parallel()
	dd := &digitDisplay{}
	in := newInstance(dd)
	in.dir = dirEast
	in.pos = anchorPos(in, 0, 0, dirEast)

	e := newEmitter()
	in.port("in").net = &netlistNet{signal: inputSignals[0]}
	dd.build(e, in)

	divs := map[float64]position{}
	mods := map[float64]position{}
	panels := map[float64]position{}
	for _, ent := range e.entities {
		switch ent.Name {
		case "arithmetic-combinator":
			require.Equal(t, clockUnitDir, ent.Direction)
			op := ent.ControlBehavior.ArithmeticConditions.Operation
			switch op {
			case "/":
				divs[ent.Position.X] = ent.Position
			case "%":
				mods[ent.Position.X] = ent.Position
			}
		case "display-panel":
			panels[ent.Position.X] = ent.Position
		}
	}

	for k := range displayDigits {
		x := in.pos.X + float64(k) - 0.5
		mod, ok := mods[x]
		require.Truef(t, ok, "missing modulo stage %d", k)
		require.InDelta(t, in.pos.Y+1.5, mod.Y, 0)
		panel, ok := panels[x]
		require.Truef(t, ok, "missing digit panel %d", k)
		require.InDelta(t, in.pos.Y+3, panel.Y, 0)
		div, ok := divs[x]
		require.Truef(t, ok, "missing divider stage %d", k)
		require.InDelta(t, in.pos.Y-0.5, div.Y, 0)
	}
}

// TestDigitDisplayInputEntersLeftEdge proves the quotient chain starts at the
// left edge, so generated blueprints do not need a public bus across the
// display or relay poles just to feed the readout.
func TestDigitDisplayInputEntersLeftEdge(t *testing.T) {
	t.Parallel()
	cv := newInstance(newConstSrc(152))
	dd := newInstance(&digitDisplay{})
	insts := []*instance{cv, dd}
	nets := []*netlistNet{connect(cv.port("out"), dd.port("in"))}
	require.NoError(t, allocateSignals(nets))

	place(insts, nets)
	e := emitNetlist(insts, netEdges(nets))
	require.NoError(t, insertRelays(e))

	for _, ent := range e.entities {
		require.NotEqual(t, relayPoleEntityName, ent.Name)
	}
	bound := e.bound[dd.port("in")]
	require.InDelta(t, dd.pos.X-0.5, e.entities[int(bound.h)-1].Position.X, 0)
}

// TestDigitDisplayPanelsReadMostSignificantFirst proves the physical panel row
// reads left-to-right as a normal number even though the quotient chain
// computes ones first.
func TestDigitDisplayPanelsReadMostSignificantFirst(t *testing.T) {
	t.Parallel()
	dd := &digitDisplay{}
	in := newInstance(dd)
	in.dir = dirEast
	in.pos = anchorPos(in, 0, 0, dirEast)

	e := newEmitter()
	in.port("in").net = &netlistNet{signal: inputSignals[0]}
	dd.build(e, in)

	panelSignals := map[float64]string{}
	for _, ent := range e.entities {
		if ent.Name != "display-panel" {
			continue
		}
		params := ent.ControlBehavior.Parameters
		require.NotEmpty(t, params)
		require.NotNil(t, params[0].Condition)
		require.NotNil(t, params[0].Condition.FirstSignal)
		panelSignals[ent.Position.X] = params[0].Condition.FirstSignal.Name
	}

	for visual := range displayDigits {
		x := in.pos.X + float64(visual) - 0.5
		want := digitSignal(displayDigits - 1 - visual).Name
		require.Equal(t, want, panelSignals[x])
	}
}

// TestDigitDisplayFootprintMatchesEmitted checks the numeric module reports the
// tight rectangular bound it emits.
func TestDigitDisplayFootprintMatchesEmitted(t *testing.T) {
	t.Parallel()
	dd := &digitDisplay{}
	in := newInstance(dd)
	in.dir = dirEast
	in.pos = anchorPos(in, 0, 0, dirEast)

	e := newEmitter()
	in.port("in").net = &netlistNet{signal: inputSignals[0]}
	dd.build(e, in)

	reserved := dd.footprint(dirEast)
	require.Equal(t, footprint{width: displayDigits, height: 5}, reserved)
	require.Equal(t, reserved, emittedBounds(t, e))
}

// moduloOutputs exposes each extracted digit so display propagation can be
// checked independently of panel rendering.
func moduloOutputs(t *testing.T, e *emitter, s *sim) []int {
	t.Helper()
	var digits []int
	for _, ent := range e.entities {
		if ent.Name != "arithmetic-combinator" {
			continue
		}
		ac := ent.ControlBehavior.ArithmeticConditions
		if ac.Operation != "%" {
			continue
		}
		require.NotNil(t, ac.OutputSignal)
		digits = append(digits, s.value(
			ent.EntityNumber,
			connectorRedOut,
			ac.OutputSignal.Name,
		))
	}
	return digits
}
