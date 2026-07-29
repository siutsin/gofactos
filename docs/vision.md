# Vision

gofactos is a teaching compiler. It turns one chosen Go function and its
supported callees into a Factorio blueprint for a conference talk about
compiler design.

Use this file to decide whether a change fits the project. See the
[specification](spec.md) for supported behaviour and limits, and
[README.md](../README.md) for build and usage instructions.

## Glossary

| Term                                         | Meaning                                                                                            |
|----------------------------------------------|----------------------------------------------------------------------------------------------------|
| [Blueprint](blueprint.md)                    | A Factorio plan describing entities, their settings, and their wires.                              |
| [Callee](spec.md#calls)                      | A function called by another function.                                                             |
| [Circuit](backend.md)                        | Connected Factorio entities that carry out the compiled logic.                                     |
| [Compiler phase](backend.md#backend-phases)  | One step that transforms or checks the program.                                                    |
| [Target constraint](spec.md#resource-limits) | A limit imposed by Factorio, such as finite signals or maximum wire reach.                         |
| Teaching compiler                            | A program built to explain how source code becomes a runnable form rather than for production use. |

## Principles

1. **Keep code and documentation aligned.** They must describe the same
   behaviour.
2. **Show real Factorio behaviour.** Use circuit mechanisms instead of
   replacing them with shortcuts.
3. **Keep the design small.** Prefer small components and clear compiler
   phases. Follow the [coding style](style.md).
4. **Keep limits visible.** Expose target constraints instead of hiding them.
5. **Test behaviour.** A blueprint that imports may still be wrong.

## Non-goals

- a general-purpose or production compiler
- complete Go language support
- optimising for the fastest or smallest blueprint
