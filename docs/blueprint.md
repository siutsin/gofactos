# Factorio 2.0 Blueprint Format

This reference covers the exchange encoding and JSON emitted by gofactos. See
the [specification](spec.md) for supported Go behaviour.

## Glossary

| Term                                         | Meaning                                                                                                                                              |
|----------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| Blueprint                                    | A Factorio plan describing entities, their settings, and their wires.                                                                                |
| [Combinator](backend.md#module-dependencies) | One of Factorio's circuit-processing entities. Constant combinators publish signals, arithmetic combinators calculate, and deciders test conditions. |
| Connector                                    | A numbered red, green, or copper endpoint.                                                                                                           |
| Control behaviour                            | The JSON settings that define an entity's circuit logic.                                                                                             |
| Entity                                       | One placed Factorio object with a number, name, and position.                                                                                        |
| Exchange string                              | The compressed text form that Factorio imports and exports.                                                                                          |
| Filter                                       | A configured signal entry on a constant combinator, including its value and quality.                                                                 |
| Quality                                      | A Factorio grade such as `normal` or `legendary` that changes an entity's capabilities.                                                              |
| [Signal](backend.md#signal-allocation)       | A named integer value carried by a circuit network. Its type can be virtual, meaning a symbolic circuit channel, or refer to a game item.            |
| Wire                                         | Joins two entity connectors.                                                                                                                         |

## Encoding

A blueprint exchange string has this form:

```text
0 + base64(zlib(JSON))
```

The leading ASCII `0` identifies the exchange format version.

## Sources

- [BlueprintEntity][e]:
  Factorio 2.0 entity schema
- [BlueprintLogisticFilter][f]:
  Factorio 2.0 constant-combinator filter schema
- [Blueprint string format][s]:
  exchange encoding; some JSON examples predate Factorio 2.0
- [Talk:Blueprint string format][t]:
  community notes about the Factorio 2.0 `wires` array

## Top-Level JSON

```json
{
  "blueprint": {
    "item": "blueprint",
    "label": "add",
    "version": 562949954076672,
    "entities": []
  }
}
```

gofactos emits `562949954076672` as the Factorio 2.0 blueprint version. It
adds `wires` only when the array is not empty.

## Entity

```json
{
  "entity_number": 1,
  "name": "substation",
  "quality": "legendary",
  "position": { "x": 0.5, "y": 0.5 }
}
```

Every entity has `entity_number`, `name`, and `position`. Substations also set
`"quality": "legendary"`. Other emitted entities omit `quality` and use normal
quality.

## Arithmetic Combinator

```json
{
  "name": "arithmetic-combinator",
  "control_behavior": {
    "arithmetic_conditions": {
      "first_signal":  { "type": "virtual", "name": "signal-A" },
      "operation":     "+",
      "second_signal": { "type": "virtual", "name": "signal-B" },
      "output_signal": { "type": "virtual", "name": "signal-C" }
    }
  }
}
```

Each input uses either its `*_signal` field or its `*_constant` field, not
both. `operation` contains a Factorio arithmetic operation.

## Decider Combinator

Uses separate `conditions` and `outputs` arrays:

```json
{
  "name": "decider-combinator",
  "control_behavior": {
    "decider_conditions": {
      "conditions": [
        {
          "first_signal": { "type": "virtual", "name": "signal-A" },
          "comparator": "=",
          "constant": 0
        },
        {
          "first_signal": { "type": "virtual", "name": "signal-C" },
          "comparator": ">",
          "constant": 1,
          "compare_type": "and"
        }
      ],
      "outputs": [
        {
          "signal": { "type": "virtual", "name": "signal-B" },
          "copy_count_from_input": true
        }
      ]
    }
  }
}
```

Each condition compares `first_signal` with either `second_signal` or
`constant`. Emitted comparators are `=`, `≠` (`\u2260`), `<`, `≤`
(`\u2264`), `>`, and `≥` (`\u2265`).

Every condition after the first may use `compare_type` to join it to the
preceding conditions. The values are `"and"` and `"or"`; omission defaults to
`"or"`. Factorio evaluates `"and"` before `"or"`.

## Constant Combinator

Factorio 2.0 stores filters in `sections`. Each filter uses flat signal fields,
not a nested `signal` object.

```json
{
  "name": "constant-combinator",
  "control_behavior": {
    "sections": {
      "sections": [
        {
          "index": 1,
          "filters": [
            {
              "index": 1,
              "type": "virtual",
              "name": "signal-A",
              "quality": "normal",
              "comparator": "=",
              "count": 1
            }
          ]
        }
      ]
    }
  }
}
```

### Filter Fields

From [BlueprintLogisticFilter][f]:

| Field        | Type    | Notes                                          |
|--------------|---------|------------------------------------------------|
| `index`      | int     | 1-based position within the section            |
| `type`       | string? | Signal type (e.g. `"virtual"`, `"item"`)       |
| `name`       | string? | Signal name (e.g. `"signal-A"`)                |
| `quality`    | string? | Quality prototype name; `"normal"` for default |
| `comparator` | string? | Quality comparator; `"="` for exact match      |
| `count`      | int     | Signal count value                             |

gofactos emits one section per constant combinator. Each filter sets `quality`
to `"normal"` and `comparator` to `"="`.

## Wires

Factorio 2.0 replaced per-entity `connections` with a top-level `wires` array
on the blueprint. Each entry is a 4-element array:

```text
[source_entity_number, source_connector, dest_entity_number, dest_connector]
```

### Wire Connector IDs

From [Talk:Blueprint string format][t]:

| ID | Constant                  | Description                            |
|----|---------------------------|----------------------------------------|
| 1  | `circuit_red`             | Red wire — input side                  |
| 2  | `circuit_green`           | Green wire — input side                |
| 3  | `combinator_output_red`   | Red wire — output side (combinators)   |
| 4  | `combinator_output_green` | Green wire — output side (combinators) |
| 5  | `pole_copper`             | Copper wire (power poles)              |

Constant combinators have a single connection point, so they use connectors
1 (red) and 2 (green) only. Arithmetic and decider combinators have input
(1/2) and output (3/4) sides.

### Example

```json
{
  "wires": [
    [1, 2, 3, 2],
    [2, 2, 3, 2]
  ]
}
```

Both rows connect a green circuit wire to entity 3's green input.

[e]: https://lua-api.factorio.com/latest/concepts/BlueprintEntity.html
[f]: https://lua-api.factorio.com/latest/concepts/BlueprintLogisticFilter.html
[s]: https://wiki.factorio.com/Blueprint_string_format
[t]: https://wiki.factorio.com/Talk:Blueprint_string_format
