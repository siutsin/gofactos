# Documentation

## Project

| Document                        | Contents                                            |
|---------------------------------|-----------------------------------------------------|
| [Vision](vision.md)             | Purpose, principles, and non-goals                  |
| [Specification](spec.md)        | What is supported and how generated blueprints work |
| [Architecture](architecture.md) | How commands and packages move from Go to Factorio  |

## Implementation

| Document                         | Contents                                              |
|----------------------------------|-------------------------------------------------------|
| [SSA](./ssa.md)                  | How Go code is represented before blueprint creation  |
| [Factorio backend](backend.md)   | How Go operations become Factorio circuits            |
| [Blueprint format](blueprint.md) | How blueprint strings, entities, and wires are stored |

## Development

| Document                 | Contents                                           |
|--------------------------|----------------------------------------------------|
| [Coding style](style.md) | Rules for code, comments, tests, and documentation |
| [Testing](testing.md)    | Local checks, expected outputs, and Factorio setup |
