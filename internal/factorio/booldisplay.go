// This file defines the in-game readout used for boolean results.
package factorio

// boolDisplay is the boolean return readout. The 1/-1 result drives one display
// panel that reads the result net directly: it shows the check signal when the
// value is 1 and the deny signal when it is -1. Like the numeric digit display
// it terminates in a panel and adds no public output net.
type boolDisplay struct{}

// kind identifies boolean displays in diagnostics and placement metadata.
func (b *boolDisplay) kind() string { return "boolDisplay" }

// ports declares the result input consumed by the terminal display.
func (b *boolDisplay) ports() []portSpec {
	return []portSpec{{name: "in", kind: portIn, colour: green}}
}

// footprint reserves the display panel without an additional teaching label.
func (b *boolDisplay) footprint(_ int) footprint {
	return footprint{width: 1, height: 1}
}

// build emits the panel that distinguishes the encoded true and false values.
func (b *boolDisplay) build(e *emitter, self *instance) {
	pin := self.port("in")
	inSig := portSignal(pin)

	// One message per encoded state: 1 is true and -1 is false.
	trueSig, falseSig := inSig, inSig
	check := signalID{Type: "virtual", Name: "signal-check"}
	deny := signalID{Type: "virtual", Name: "signal-deny"}
	panel := e.add(entity{
		Name: displayPanelName,
		Position: position{
			X: self.pos.X - 0.5,
			Y: self.pos.Y - 1,
		},
		AlwaysShow: true,
		ControlBehavior: &controlBehavior{Parameters: []displayPanelMessage{
			{Icon: &check, Condition: &displayPanelCondition{
				FirstSignal: &trueSig, Comparator: "=", Constant: 1,
			}},
			{Icon: &deny, Condition: &displayPanelCondition{
				FirstSignal: &falseSig, Comparator: "=", Constant: -1,
			}},
		}},
	})
	e.bind(pin, panel, connectorGreenIn)
}
