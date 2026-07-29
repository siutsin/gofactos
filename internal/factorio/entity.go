// This file models the Factorio entities emitted into blueprint JSON.
package factorio

import (
	"go/token"
)

// BlueprintWrapper is the top-level JSON envelope: {"blueprint": {...}}.
type BlueprintWrapper struct {
	Blueprint Blueprint `json:"blueprint"`
}

// Blueprint holds the Factorio blueprint metadata and entity list.
type Blueprint struct {
	Item     string   `json:"item"`
	Label    string   `json:"label,omitempty"`
	Version  int64    `json:"version"`
	Entities []entity `json:"entities"`
	Wires    []wire   `json:"wires,omitempty"`
}

// blueprintVersion is the Factorio blueprint format version for 2.0.x.
const blueprintVersion = 562949954076672

// wire represents a single wire connection in the Factorio 2.0 format.
// Each wire is [sourceEntity, sourceConnector, destEntity, destConnector].
type wire [4]int

// Wire connector IDs for Factorio 2.0 blueprints.
//
// Factorio 2.0 uses a top-level "wires" array instead of per-entity
// connections. Each wire is [source, sourceConnector, dest, destConnector].
//
// Constant combinators have a single connection point (connectors 1 and 2).
// Arithmetic and decider combinators have separate input (1/2) and
// output (3/4) sides. Power poles use connector 5 for copper wire.
const (
	connectorRedIn      = iota + 1 // circuit_red, input side
	connectorGreenIn               // circuit_green, input side
	connectorRedOut                // combinator_output_red, output side
	connectorGreenOut              // combinator_output_green, output side
	connectorPoleCopper            // pole_copper, electric network
)

// isRedConnector reports whether a circuit connector uses the red wire colour.
func isRedConnector(c int) bool {
	return c == connectorRedIn || c == connectorRedOut
}

type entityShape int

const (
	combinatorShape entityShape = iota
	singleTileShape
	substationShape
)

type entityCapability struct {
	shape      entityShape
	powered    bool
	connectors uint8
	quality    string
}

// connectorMask returns the bit set accepted by one emitted entity kind.
func connectorMask(connectors ...int) uint8 {
	var mask uint8
	for _, connector := range connectors {
		mask |= 1 << connector
	}
	return mask
}

var entityCapabilities = map[string]entityCapability{
	arithCombinatorName: {
		shape: combinatorShape, powered: true,
		connectors: connectorMask(
			connectorRedIn,
			connectorGreenIn,
			connectorRedOut,
			connectorGreenOut,
		),
	},
	deciderCombinatorName: {
		shape: combinatorShape, powered: true,
		connectors: connectorMask(
			connectorRedIn,
			connectorGreenIn,
			connectorRedOut,
			connectorGreenOut,
		),
	},
	constCombinatorName: {
		shape:      singleTileShape,
		connectors: connectorMask(connectorRedIn, connectorGreenIn),
	},
	displayPanelName: {
		shape:      singleTileShape,
		connectors: connectorMask(connectorRedIn, connectorGreenIn),
	},
	relayPoleEntityName: {
		shape:      singleTileShape,
		connectors: connectorMask(connectorRedIn, connectorGreenIn),
	},
	smallLampEntityName: {
		shape: singleTileShape, powered: true,
		connectors: connectorMask(connectorRedIn, connectorGreenIn),
	},
	powerPoleEntityName: {
		shape: substationShape, quality: powerPoleQuality,
		connectors: connectorMask(connectorPoleCopper),
	},
}

// supportsConnector reports whether an entity kind owns a connector ID.
func (c entityCapability) supportsConnector(connector int) bool {
	return connector > 0 && connector < 8 &&
		c.connectors&(1<<connector) != 0
}

// entity represents one Factorio entity within a blueprint.
type entity struct {
	EntityNumber    int              `json:"entity_number"`
	Name            string           `json:"name"`
	Quality         string           `json:"quality,omitempty"`
	Position        position         `json:"position"`
	Direction       int              `json:"direction,omitempty"`
	Colour          *rgbaColour      `json:"color,omitempty"`
	Text            string           `json:"text,omitempty"`
	Icon            *signalID        `json:"icon,omitempty"`
	AlwaysShow      bool             `json:"always_show,omitempty"`
	ControlBehavior *controlBehavior `json:"control_behavior,omitempty"`
}

// position holds half-tile grid coordinates for entity placement.
type position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// signalID identifies a Factorio signal by type and name.
type signalID struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// rgbaColour is a static entity colour in Factorio's RGBA format.
type rgbaColour struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}

// greenLampColour keeps every generated activity lamp visually consistent.
func greenLampColour() *rgbaColour {
	return &rgbaColour{G: 1, A: 1}
}

// controlBehavior holds an entity's circuit configuration.
type controlBehavior struct {
	ArithmeticConditions *arithmeticConditions       `json:"arithmetic_conditions,omitempty"`
	DeciderConditions    *deciderConditions          `json:"decider_conditions,omitempty"`
	Sections             *constantCombinatorSections `json:"sections,omitempty"`
	Parameters           []displayPanelMessage       `json:"parameters,omitempty"`
	IsOn                 *bool                       `json:"is_on,omitempty"`
	CircuitEnabled       bool                        `json:"circuit_enabled,omitempty"`
	CircuitCondition     *circuitCondition           `json:"circuit_condition,omitempty"`
}

// circuitCondition controls whether a circuit-connected entity is active.
type circuitCondition struct {
	FirstSignal *signalID `json:"first_signal,omitempty"`
	Comparator  string    `json:"comparator,omitempty"`
	Constant    int       `json:"constant,omitempty"`
}

// constantCombinatorSections is the Factorio 2.0 wrapper for constant
// combinator filters.
type constantCombinatorSections struct {
	Sections []logisticSection `json:"sections"`
}

// logisticSection groups a set of constant filters under an index within
// a constant combinator.
type logisticSection struct {
	Index   int              `json:"index"`
	Filters []constantFilter `json:"filters"`
}

// arithmeticConditions configures an arithmetic combinator (e.g. A + B → C).
type arithmeticConditions struct {
	FirstSignal    *signalID `json:"first_signal,omitempty"`
	Operation      string    `json:"operation"`
	SecondSignal   *signalID `json:"second_signal,omitempty"`
	OutputSignal   *signalID `json:"output_signal,omitempty"`
	FirstConstant  *int      `json:"first_constant,omitempty"`
	SecondConstant *int      `json:"second_constant,omitempty"`
}

// deciderConditions configures a decider combinator using the Factorio 2.0
// format with separate conditions and outputs arrays.
type deciderConditions struct {
	Conditions []deciderCondition `json:"conditions"`
	Outputs    []deciderOutput    `json:"outputs"`
}

// deciderCondition is a single condition row within a decider combinator.
type deciderCondition struct {
	FirstSignal  *signalID `json:"first_signal,omitempty"`
	Comparator   string    `json:"comparator"`
	Constant     *int      `json:"constant,omitempty"`
	SecondSignal *signalID `json:"second_signal,omitempty"`
	CompareType  string    `json:"compare_type,omitempty"`
}

// deciderOutput is a single output row within a decider combinator.
type deciderOutput struct {
	Signal             *signalID `json:"signal,omitempty"`
	CopyCountFromInput bool      `json:"copy_count_from_input"`
}

// constantFilter sets a signal value on a constant combinator using
// the Factorio 2.0 flat-field format.
type constantFilter struct {
	Index      int    `json:"index"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	Quality    string `json:"quality"`
	Comparator string `json:"comparator"`
	Count      int    `json:"count"`
}

// displayPanelMessage defines conditional text shown on a display panel.
type displayPanelMessage struct {
	Text      string                 `json:"text"`
	Icon      *signalID              `json:"icon,omitempty"`
	Condition *displayPanelCondition `json:"condition,omitempty"`
}

// displayPanelCondition controls when a display panel message is visible,
// based on a circuit signal comparison.
type displayPanelCondition struct {
	FirstSignal *signalID `json:"first_signal,omitempty"`
	Comparator  string    `json:"comparator,omitempty"`
	Constant    int       `json:"constant,omitempty"`
}

// operationEntry maps a Go binary operator to a Factorio combinator operation
// string and the entity type that implements it.
type operationEntry struct {
	operation  string
	entityName string
}

// binOpMap maps Go token operators to their Factorio combinator equivalents.
// Arithmetic operators use arithCombinatorName; comparison operators use
// deciderCombinatorName.
//
// Factorio uses Unicode characters for some comparators:
//
//	Go token  │ Factorio  │ Unicode
//	──────────┼───────────┼─────────
//	==        │ =         │ (ASCII)
//	!=        │ ≠         │ \u2260
//	<         │ <         │ (ASCII)
//	<=        │ ≤         │ \u2264
//	>         │ >         │ (ASCII)
//	>=        │ ≥         │ \u2265
var binOpMap = map[token.Token]operationEntry{
	token.ADD: {"+", arithCombinatorName},
	token.SUB: {"-", arithCombinatorName},
	token.MUL: {"*", arithCombinatorName},
	token.QUO: {"/", arithCombinatorName},
	token.REM: {"%", arithCombinatorName},
	token.EQL: {"=", deciderCombinatorName},
	token.NEQ: {"\u2260", deciderCombinatorName},
	token.LSS: {"<", deciderCombinatorName},
	token.LEQ: {"\u2264", deciderCombinatorName},
	token.GTR: {">", deciderCombinatorName},
	token.GEQ: {"\u2265", deciderCombinatorName},
}

// canonicalComparator normalises convenient aliases to Factorio's spelling.
func canonicalComparator(op string) string {
	switch op {
	case "!=":
		return "≠"
	case "<=":
		return "≤"
	case ">=":
		return "≥"
	default:
		return op
	}
}

// negateComparator supplies the complementary condition for comparison false
// branches:
//
//	<  ↔  ≥
//	>  ↔  ≤
//	=  ↔  ≠
func negateComparator(op string) string {
	switch canonicalComparator(op) {
	case "<":
		return "≥" // ≥
	case ">":
		return "≤" // ≤
	case "=":
		return "≠" // ≠
	case "≥": // ≥
		return "<"
	case "≤": // ≤
		return ">"
	case "≠": // ≠
		return "="
	default:
		return op
	}
}
