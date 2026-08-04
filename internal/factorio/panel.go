// This file adds source-level teaching labels to emitted combinators.
package factorio

import (
	"fmt"
	"strconv"
	"strings"
)

// Combinators that carry a teaching step get a display panel directly above
// them. Pure composite plumbing may
// stay unlabelled when naming it would distract from the source-level flow.
// Leaf combinators are labelled from their own control behaviour at Emit. A
// composite renders its own panels through combinatorLabel. Neither Select nor
// the modules thread a label string. See docs/backend.md ("Backend Phases")
// for the design rationale.

// labeller renders panel text. alias maps an intermediate signal name to the
// SSA value name (t0, t1, ...) driving it, so computed values read as they do
// in the SSA dump. constAlias maps a literal source's signal to its cN token
// (c0, c1, ...), and consts maps the same signal to its value (`7`, `true`), so
// a constant reads as `c0` in expressions and `c0 = 7` on its own panel.
// Signals
// in none of the maps fall back to the item-signal token.
type labeller struct {
	alias      map[string]string
	portAlias  map[*port]string
	consts     map[string]string
	constAlias map[string]string
}

// labelPanel is one panel a module asks the panel pass to place: its text, an
// optional signal icon, and the position to place it at.
type labelPanel struct {
	text string
	icon *signalID
	pos  position
}

// combinatorLabeller is an optional component capability: a composite module
// renders its own panel for one of its emitted combinators instead of the
// generic control-behaviour reader. ok is false when that combinator gets no
// panel. Keeping each module's label knowledge in the module lets the panel
// pass dispatch by capability rather than switching on every concrete module
// type, so
// a new module needs no edit here.
type combinatorLabeller interface {
	combinatorLabel(ent entity, self *instance, l *labeller) (labelPanel, bool)
}

// labelTextForOwner preserves contextual port names while labelling an entity.
func (l *labeller) labelTextForOwner(
	ent entity,
	owner *instance,
) (text string, icon *signalID, ok bool) {
	cb := ent.ControlBehavior
	if cb == nil {
		return "", nil, false
	}
	// Deciders are absent: every component that emits one renders its own panel
	// through combinatorLabel, so labelFor never reaches this reader for them.
	switch ent.Name {
	case constCombinatorName:
		return l.constantLabelText(cb, owner)
	case arithCombinatorName:
		return l.arithmeticLabelText(cb.ArithmeticConditions, owner)
	default:
		return "", nil, false
	}
}

// constantLabelText distinguishes editable parameters from literal sources.
func (l *labeller) constantLabelText(
	cb *controlBehavior,
	owner *instance,
) (string, *signalID, bool) {
	if cb.Sections == nil || len(cb.Sections.Sections) == 0 ||
		len(cb.Sections.Sections[0].Filters) == 0 {
		return "", nil, false
	}
	f := cb.Sections.Sections[0].Filters[0]
	s := &signalID{Type: f.Type, Name: f.Name}
	if lit, ok := l.consts[s.Name]; ok {
		// A literal source reads "c0 = 1" and is referenced as "c0".
		return fmt.Sprintf("%s = %s", l.signalLabel(s), lit), s, true
	}
	return fmt.Sprintf(
		"%s = %d",
		l.portSignalLabel(owner, "out", s),
		f.Count,
	), s, true
}

// arithmeticLabelText turns an arithmetic control rule into a readable formula.
func (l *labeller) arithmeticLabelText(
	ac *arithmeticConditions,
	owner *instance,
) (string, *signalID, bool) {
	if ac == nil || ac.OutputSignal == nil {
		return "", nil, false
	}
	if ac.Operation == "*" && ac.SecondConstant != nil {
		switch *ac.SecondConstant {
		case -1:
			return fmt.Sprintf(
				"%s = -%s",
				l.arithmeticSignalLabel(owner, "out", ac.OutputSignal),
				l.arithmeticOperandLabel(
					owner,
					"first",
					ac.FirstSignal,
					ac.FirstConstant,
				),
			), ac.OutputSignal, true
		case 1:
			return fmt.Sprintf(
				"%s = %s",
				l.arithmeticSignalLabel(owner, "out", ac.OutputSignal),
				l.arithmeticOperandLabel(
					owner,
					"first",
					ac.FirstSignal,
					ac.FirstConstant,
				),
			), ac.OutputSignal, true
		}
	}
	return fmt.Sprintf(
		"%s = %s %s %s",
		l.arithmeticSignalLabel(owner, "out", ac.OutputSignal),
		l.arithmeticOperandLabel(
			owner,
			"first",
			ac.FirstSignal,
			ac.FirstConstant,
		),
		ac.Operation,
		l.arithmeticOperandLabel(
			owner,
			"second",
			ac.SecondSignal,
			ac.SecondConstant,
		),
	), ac.OutputSignal, true
}

// labelFor centralises the choice between composite and generic teaching
// labels.
// It returns the panel for one combinator and whether it carries one: the
// owning composite's own panel when the module is a combinatorLabeller, or the
// generic reader's text wrapped at the combinator's position with the icon rule
// applied. It is the single dispatch point the panel pass and the test adapter
// share, so the rule lives in one place.
func (l *labeller) labelFor(ent entity, owner *instance) (labelPanel, bool) {
	if owner != nil {
		if cl, ok := owner.comp.(combinatorLabeller); ok {
			return cl.combinatorLabel(ent, owner, l)
		}
	}
	text, icon, ok := l.labelTextForOwner(ent, owner)
	if !ok {
		return labelPanel{}, false
	}
	// Only parameter panels carry a signal icon: the input letters (A, B, ...).
	// Literal constants (cX) and every other label are text only. A parameter is
	// a constant combinator whose signal is not a literal-constant alias.
	if ent.Name != constCombinatorName || (icon != nil && l.constAlias[icon.Name] != "") {
		icon = nil
	}
	return labelPanel{text: text, icon: icon, pos: labelPanelPosition(ent)}, true
}

// phiPortSignalLabel gives merge labels the source name of a branch value.
func (l *labeller) phiPortSignalLabel(owner *instance, name string) string {
	if owner == nil {
		return "?"
	}
	p := owner.port(name)
	if p == nil || p.net == nil {
		return "?"
	}
	return l.portSignalLabel(owner, name, &p.net.signal)
}

// portSignalLabel preserves call-site names when one signal crosses contexts.
// A shared physical signal can therefore have a callee name at its
// producer and a caller name at each consuming operand.
func (l *labeller) portSignalLabel(
	owner *instance,
	name string,
	signal *signalID,
) string {
	if owner != nil {
		port := owner.port(name)
		if alias := l.portAlias[port]; alias != "" {
			return alias
		}
	}
	return l.signalLabel(signal)
}

// arithmeticSignalLabel maps an arithmetic role back to its module port.
func (l *labeller) arithmeticSignalLabel(
	owner *instance,
	role string,
	signal *signalID,
) string {
	if owner == nil {
		return l.signalLabel(signal)
	}
	portName := role
	switch owner.comp.(type) {
	case *arith:
		switch role {
		case "first":
			portName = "a"
		case "second":
			portName = "b"
		}
	case *neg:
		if role == "first" {
			portName = "in"
		}
	default:
		return l.signalLabel(signal)
	}
	return l.portSignalLabel(owner, portName, signal)
}

// arithmeticOperandLabel labels signals contextually while retaining constants.
func (l *labeller) arithmeticOperandLabel(
	owner *instance,
	role string,
	signal *signalID,
	constant *int,
) string {
	if signal != nil {
		return l.arithmeticSignalLabel(owner, role, signal)
	}
	return l.operandLabel(nil, constant)
}

// signalLabel keeps panel formulas short: private signals use plain words,
// literals use cN aliases, computed values use SSA names, inputs drop the
// "signal-" prefix, and other intermediates use stable item aliases.
func (l *labeller) signalLabel(s *signalID) string {
	if s == nil {
		return "?"
	}
	switch s.Name {
	case privateData.Name:
		return "value"
	case privateTmp.Name:
		return "scratch"
	case privateInc.Name:
		return "step"
	}
	if c, ok := l.constAlias[s.Name]; ok {
		return c
	}
	if name, ok := l.alias[s.Name]; ok {
		return name
	}
	if alias, ok := intermediateAlias[s.Name]; ok {
		return alias
	}
	return strings.TrimPrefix(s.Name, "signal-")
}

// operandLabel gives generic label renderers a name for either operand form.
func (l *labeller) operandLabel(sig *signalID, constant *int) string {
	if sig != nil {
		return l.signalLabel(sig)
	}
	if constant != nil {
		return strconv.Itoa(*constant)
	}
	return "?"
}

// conditionLabelForPorts retains contextual names in composite conditions.
// boolean spells the 1/-1 sentinels as true/false only when the compared operand
// is Boolean, so an integer equality such as `n == 1` reads as `n = 1`, not
// `n = true`.
func (l *labeller) conditionLabelForPorts(
	c deciderCondition,
	owner *instance,
	first string,
	second string,
	boolean bool,
) string {
	left := l.portSignalLabel(owner, first, c.FirstSignal)
	right := l.operandLabel(c.SecondSignal, c.Constant)
	if boolean && c.SecondSignal == nil && c.Constant != nil {
		switch *c.Constant {
		case 1:
			right = "true"
		case -1:
			right = "false"
		}
	}
	if c.SecondSignal != nil {
		right = l.portSignalLabel(owner, second, c.SecondSignal)
	}
	return fmt.Sprintf("%s %s %s", left, c.Comparator, right)
}

// combinatorWidth lets labels align with differently shaped combinators.
func combinatorWidth(ent entity) int {
	if ent.Name == constCombinatorName {
		return 1
	}
	w, _ := combinatorSize(ent.Direction)
	return w
}

// labelPanelPosition keeps teaching labels directly above their combinators.
func labelPanelPosition(ent entity) position {
	x := ent.Position.X
	if combinatorWidth(ent) == 2 {
		x -= 0.5
	}
	return position{X: x, Y: ent.Position.Y - 1}
}

// addLabelPanels decorates the emitted circuit without burdening selection or
// module wiring with display concerns. A composite module that implements
// combinatorLabeller renders its
// own panel for each of its combinators (a per-combinator step, or one summary
// for the whole compact unit); every other combinator falls back to the generic
// control-behaviour reader. Only parameter panels carry a signal icon.
func addLabelPanels(e *emitter) {
	l := &labeller{
		alias:      e.aliases,
		portAlias:  e.portAlias,
		consts:     e.consts,
		constAlias: e.constAlias,
	}
	entityCount := len(e.entities)
	for i := range entityCount {
		ent := e.entities[i]
		switch ent.Name {
		case constCombinatorName, arithCombinatorName, deciderCombinatorName:
		default:
			continue
		}
		lp, ok := l.labelFor(ent, e.owner[ent.EntityNumber])
		if !ok {
			continue
		}
		e.add(entity{
			Name:       displayPanelName,
			Position:   lp.pos,
			Text:       lp.text,
			Icon:       lp.icon,
			AlwaysShow: true,
		})
	}
}
