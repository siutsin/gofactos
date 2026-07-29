// This file defines the shared pulse source for clocked state machines.
package factorio

import "fmt"

const (
	// clockPeriod is the one blueprint clock's divisor: 60 game ticks, so a
	// loop steps once a second.
	clockPeriod = 60
	// fastClockPeriod is the smallest period that lets all eight result digits
	// settle while a recursive machine is in PRESENT mode.
	fastClockPeriod = 15

	clockStartLabel = "START / RESET"
)

// effectiveClockPeriod supplies the default period when no option overrides it.
func effectiveClockPeriod(period int) int {
	if period == 0 {
		return clockPeriod
	}
	return period
}

// clockedStateSettleBudgetFor reserves the ticks available to state logic.
func clockedStateSettleBudgetFor(period int) int { return period - 2 }

// clockSummary formats the rate label shared by clocked components.
func clockSummary(name string, period int) string {
	rate := float64(60) / float64(effectiveClockPeriod(period))
	return fmt.Sprintf("%s clock (%g Hz)", name, rate)
}

// clockUnitDir rotates the clock's combinators to 1x2 (output facing down) so
// the three pack into a tight horizontal row, the same trick the numeric
// display uses. Place pins the whole unit to a reserved leftmost column.
const clockUnitDir = 8

// clockDiv is the shared blueprint clock and manual START control. Its bounded
// feedback state advances only while START is on and emits one green pulse
// every period ticks. Loops and recursive machines use that pulse to advance at
// a readable rate.
//
//	one:     start * 1         -> count
//	counter: count % period    -> count  (one shared bounded red net)
//	pulse:   if count == 2 and start == 1 -> pulse
//
// The three clock combinators are 1x2 and sit beside the manual control as one
// compact unit under rate and START labels. Placement pins the unit to the
// start of the blueprint rather than after the result display.
type clockDiv struct {
	period  int
	summary string
}

// newClockDivWithSummary creates the shared clock with its consumer's label.
func newClockDivWithSummary(period int, summary string) *clockDiv {
	period = effectiveClockPeriod(period)
	if summary == "" {
		summary = clockSummary("loop", period)
	}
	return &clockDiv{period: period, summary: summary}
}

// kind identifies the clock for placement and component-specific handling.
func (c *clockDiv) kind() string { return "clockDiv" }

// ports exposes the gated pulse and the control shared by clocked state.
func (c *clockDiv) ports() []portSpec {
	return []portSpec{
		{name: "pulse", kind: portOut, colour: green},
		{name: "start", kind: portOut, colour: green},
	}
}

// footprint reserves the compact clock, manual control, and their labels.
func (c *clockDiv) footprint(_ int) footprint {
	return footprint{width: 4, height: 3}
}

// build emits the default-off control, bounded state, and periodic pulse.
func (c *clockDiv) build(e *emitter, self *instance) {
	ppulse, pstart := self.port("pulse"), self.port("start")
	pulseSig, startSig := portSignal(ppulse), portSignal(pstart)
	gd := privateData
	one, pulseAt, period := 1, 2, c.period

	// Each combinator is a 1x2 unit centred on a one-tile column; the three sit
	// in a tight row at columns 0, 1, 2.
	at := func(dx int) position {
		return position{X: self.pos.X + float64(dx) - 0.5, Y: self.pos.Y + 0.5}
	}

	// START is the sole player control for every clocked blueprint. Its green
	// connector exposes the allocated signal; its red connector feeds the clock's
	// private state network.
	off := false
	control := e.add(entity{
		Name:     constCombinatorName,
		Position: position{X: self.pos.X + 2.5, Y: self.pos.Y},
		ControlBehavior: &controlBehavior{
			IsOn: &off,
			Sections: &constantCombinatorSections{
				Sections: []logisticSection{{
					Index: 1,
					Filters: []constantFilter{{
						Index: 1, Type: startSig.Type,
						Name: startSig.Name, Quality: "normal",
						Comparator: "=", Count: 1,
					}},
				}},
			},
		},
	})
	// one contributes one to the shared red state only while START is on.
	oneCell := e.add(entity{
		Name:      arithCombinatorName,
		Position:  at(0),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{ArithmeticConditions: &arithmeticConditions{
			FirstSignal: &startSig, Operation: "*", SecondConstant: &one,
			OutputSignal: &gd,
		}},
	})
	// counter bounds the shared one-plus-prior-state value before feeding it
	// back. The visible red state therefore cycles from 1 through period instead
	// of eventually overflowing Factorio's signed 32-bit signal range.
	counter := e.add(entity{
		Name:      arithCombinatorName,
		Position:  at(1),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{ArithmeticConditions: &arithmeticConditions{
			FirstSignal: &gd, Operation: "%", SecondConstant: &period,
			OutputSignal: &gd,
		}},
	})
	// pulse fires at state 2 while START remains on. State 0 is absent and state
	// 1 is the first startup tick, so state 2 avoids the historical startup
	// double-pulse.
	pulse := e.add(entity{
		Name:      deciderCombinatorName,
		Position:  at(2),
		Direction: clockUnitDir,
		ControlBehavior: &controlBehavior{DeciderConditions: &deciderConditions{
			Conditions: []deciderCondition{
				{
					FirstSignal: &gd,
					Comparator:  "=",
					Constant:    &pulseAt,
				},
				{
					FirstSignal: &startSig,
					Comparator:  "=",
					Constant:    &one,
					CompareType: "and",
				},
			},
			Outputs: []deciderOutput{{Signal: &pulseSig, CopyCountFromInput: false}},
		}},
	})
	// The summary sits above the clock; the control's instruction sits beside it.
	e.add(entity{
		Name:       displayPanelName,
		Position:   position{X: self.pos.X + 0.5, Y: self.pos.Y - 1},
		Text:       c.summary,
		AlwaysShow: true,
	})
	e.add(entity{
		Name:       displayPanelName,
		Position:   position{X: self.pos.X + 2.5, Y: self.pos.Y - 1},
		Text:       clockStartLabel,
		AlwaysShow: true,
	})

	// One private red net carries START, oneCell, and prior bounded state. Signals
	// remain distinct: the arithmetic reads START, while counter and pulse read
	// count. The counter feeds its bounded output back.
	e.link(control, connectorRedIn, counter, connectorRedIn)
	e.link(oneCell, connectorRedIn, counter, connectorRedIn)
	e.link(oneCell, connectorRedOut, counter, connectorRedIn)
	e.link(counter, connectorRedOut, counter, connectorRedIn)
	e.link(pulse, connectorRedIn, counter, connectorRedIn)

	e.bind(ppulse, pulse, connectorGreenOut)
	e.bind(pstart, control, connectorGreenIn)
}

// combinatorLabel suppresses detail labels in favour of the clock summary.
func (c *clockDiv) combinatorLabel(_ entity, _ *instance, _ *labeller) (labelPanel, bool) {
	return labelPanel{}, false
}
