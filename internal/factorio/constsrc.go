// This file defines editable sources for parameters and literal constants.
package factorio

// constSrc is a leaf module: a constant combinator emitting one signal at a
// fixed value on its single green connection point. It is the parameter or
// literal source. In game the player edits the value to feed inputs.
type constSrc struct{ value int }

// newConstSrc creates the source used to seed a net with a fixed value.
func newConstSrc(value int) *constSrc { return &constSrc{value: value} }

// kind identifies constant sources in diagnostics and placement metadata.
func (c *constSrc) kind() string { return "constSrc" }

// ports exposes the value produced by a constant source.
func (c *constSrc) ports() []portSpec {
	return []portSpec{{name: "out", kind: portOut, colour: green}}
}

// footprint reserves a constant combinator and its editable-value label.
func (c *constSrc) footprint(dir int) footprint {
	return expandFootprint(dir, footprintPart{0, 1, true})
}

// build emits the in-game source that makes an input or literal editable.
func (c *constSrc) build(e *emitter, self *instance) {
	out := self.port("out")
	s := portSignal(out)
	h := e.add(entity{
		Name:     constCombinatorName,
		Position: self.pos,
		ControlBehavior: &controlBehavior{Sections: &constantCombinatorSections{
			Sections: []logisticSection{{Index: 1, Filters: []constantFilter{
				{Index: 1, Type: s.Type, Name: s.Name, Quality: "normal", Comparator: "=", Count: c.value},
			}}},
		}},
	})
	// A constant combinator has a single connection point.
	e.bind(out, h, connectorFor(true, out.spec.kind, out.spec.colour))
}
