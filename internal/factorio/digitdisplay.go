// This file defines the decimal readout used for integer results.
package factorio

import "fmt"

// displayDigits is how many decimal places the numeric readout shows. Eight
// covers the non-negative range 0..99999999. Negative values are not rendered
// with a sign today; the digit ladder only has messages for digit values 0..9.
const displayDigits = 8

// digitDisplay is the integer return readout: a compact horizontal
// digit-extraction chain. Stage k takes value % 10 for its digit and value / 10
// for the next stage. The stages sit side by side, most significant on the
// left,
// so the row of digit panels reads as one decimal number. The whole compound is
// a single placement unit and works on private red nets, so it adds no public
// net to the two signal banks. The extraction combinators carry no label panel,
// to keep the readout tight. Boolean returns use boolDisplay instead.
type digitDisplay struct{}

// kind identifies digit displays in diagnostics and placement metadata.
func (d *digitDisplay) kind() string { return "digitDisplay" }

// ports declares the result input consumed by the terminal display chain.
func (d *digitDisplay) ports() []portSpec {
	return []portSpec{{name: "in", kind: portIn, colour: green}}
}

// footprint reserves a stable rectangular readout for every result value.
func (d *digitDisplay) footprint(_ int) footprint {
	// Each digit stage sits in a one-tile-wide slot. The divider feeding the
	// next digit is the upper 1x2 combinator, the modulo digit extractor is the
	// lower 1x2 combinator, and the display panel sits below it.
	// The most significant digit takes the leftmost slot.
	return footprint{width: displayDigits, height: 5}
}

// build emits the extraction chain that makes an integer readable in decimal.
// Stage k reads value_k, emits value_k / 10 for the next stage, and emits
// value_k % 10 as its digit. The arithmetic chain runs left-to-right from ones
// to higher places; the panels read the shared bus in the opposite order, so
// the visible row reads from most to least significant.
func (d *digitDisplay) build(e *emitter, self *instance) {
	pin := self.port("in")
	inSig := portSignal(pin)
	ten := 10
	gd := privateData
	arith := func(
		first signalID,
		op string,
		out signalID,
		dx int,
		dy float64,
	) handle {
		return e.add(entity{
			Name: arithCombinatorName,
			Position: position{
				X: self.pos.X + float64(dx) - 0.5,
				Y: self.pos.Y + dy,
			},
			Direction: clockUnitDir,
			ControlBehavior: &controlBehavior{
				ArithmeticConditions: &arithmeticConditions{
					FirstSignal:    &first,
					Operation:      op,
					SecondConstant: &ten,
					OutputSignal:   &out,
				},
			},
		})
	}

	var prevDiv handle
	var prevMod handle
	for k := range displayDigits {
		dx := k
		value := gd
		if k == 0 {
			value = inSig // value_0 is the green result
		}
		// The final divider's output is unused, but keeping it makes the
		// in-game readout a full rectangular two-row block.
		div := arith(value, "/", gd, dx, -0.5)
		digitSig := digitSignal(k)
		mod := arith(value, "%", digitSig, dx, 1.5)
		panelSig := digitSignal(displayDigits - 1 - k)
		panel := e.add(entity{
			Name: displayPanelName,
			Position: position{
				X: self.pos.X + float64(dx) - 0.5,
				Y: self.pos.Y + 3,
			},
			AlwaysShow:      true,
			ControlBehavior: &controlBehavior{Parameters: digitLadder(panelSig)},
		})
		e.link(mod, connectorRedOut, panel, connectorRedIn)
		if k == 0 {
			e.bind(pin, div, connectorGreenIn)
			e.link(div, connectorGreenIn, mod, connectorGreenIn)
		} else {
			e.link(prevDiv, connectorRedOut, div, connectorRedIn)
			e.link(div, connectorRedIn, mod, connectorRedIn)
			e.link(prevMod, connectorRedOut, mod, connectorRedOut)
		}
		prevDiv = div
		prevMod = mod
	}
}

// digitSignal gives each decimal place a distinct private display-bus signal.
func digitSignal(place int) signalID {
	return signalID{Type: "virtual", Name: fmt.Sprintf("signal-%d", place)}
}

// digitLadder provides the ten conditional messages needed to show one digit.
// An absent signal reads as 0, so a zero digit shows signal-0.
func digitLadder(sig signalID) []displayPanelMessage {
	msgs := make([]displayPanelMessage, 0, 10)
	for v := 0; v <= 9; v++ {
		s := sig
		msgs = append(msgs, displayPanelMessage{
			Icon: &signalID{Type: "virtual", Name: fmt.Sprintf("signal-%d", v)},
			Condition: &displayPanelCondition{
				FirstSignal: &s,
				Comparator:  "=",
				Constant:    v,
			},
		})
	}
	return msgs
}

// combinatorLabel suppresses labels that would crowd the decimal readout.
func (d *digitDisplay) combinatorLabel(_ entity, _ *instance, _ *labeller) (labelPanel, bool) {
	return labelPanel{}, false
}
