# Specification

gofactos compiles one selected top-level Go function, the root, and supported
functions it calls into one Factorio blueprint. Before checking operations,
gofactos resolves branches whose SSA condition is a Boolean constant. It keeps
only the selected arm; runtime conditions keep both arms. This is reachability
filtering, not general dead-code elimination. If the remaining reachable code
uses an unsupported feature or exhausts a resource, compilation produces no
blueprint.

Every generated blueprint targets Factorio 2.0. Blueprints with powered
entities use legendary substations, so importing them also needs Space Age
installed with its `quality` mod enabled. The `space-age` mod may stay disabled.

See the [backend guide](backend.md) for design details and
the [project vision](vision.md) for non-goals.

## Glossary

| Term                                               | Meaning                                                                                                  |
|----------------------------------------------------|----------------------------------------------------------------------------------------------------------|
| Aggregate                                          | A value that groups other values, such as a struct, array, slice, or map.                                |
| [Blueprint](blueprint.md)                          | A Factorio plan describing entities, their settings, and their wires.                                    |
| [Call expansion](backend.md#control-flow-lowering) | Copying a called function's operations into the blueprint at each call instead of making a runtime call. |
| Channel                                            | A Go feature that passes values between goroutines.                                                      |
| [Clock](backend.md#composite-modules)              | A circuit that emits regular pulses measured in ticks.                                                   |
| Closure                                            | A function value that keeps access to variables from its surrounding code.                               |
| [Compile time](architecture.md#pipeline)           | When gofactos builds the blueprint.                                                                      |
| Dynamic call                                       | A call that chooses its target at runtime.                                                               |
| Generic function                                   | A function that accepts type parameters.                                                                 |
| Goroutine                                          | A Go function running at the same time as other work.                                                    |
| Interface call                                     | A call that chooses a concrete method implementation at runtime.                                         |
| [Merge](backend.md#control-flow-lowering)          | A point where different execution paths meet that chooses the value from the path that ran.              |
| Method                                             | A function that belongs to a type.                                                                       |
| Mod                                                | An installable package that adds to or changes Factorio.                                                 |
| Mutual recursion                                   | Two or more functions calling one another in a cycle.                                                    |
| [Native binary](./ssa.md#compiler-flow)            | A program containing instructions for a physical CPU.                                                    |
| Pointer                                            | A value that identifies another value's memory address.                                                  |
| [Quality](blueprint.md#entity)                     | Factorio's system of entity grades.                                                                      |
| Recurrence                                         | An update to loop state using a value from an earlier iteration.                                         |
| Recursion                                          | A function calling itself.                                                                               |
| Root                                               | The chosen Go function that becomes the blueprint's entry point.                                         |
| [Runtime](architecture.md#pipeline)                | When the circuit operates inside Factorio.                                                               |
| Short-circuit operation                            | A Boolean operation that stops evaluating once its result is known, as `&&` and `\|\|` do.               |
| [Signal](blueprint.md)                             | A named integer carried by a circuit network. Zero is indistinguishable from no signal.                  |
| Signed `int32`                                     | A 32-bit integer from -2,147,483,648 to 2,147,483,647.                                                   |
| [Stack frame](backend.md#control-flow-lowering)    | Storage for the values of one active function call.                                                      |
| [Stack overflow](backend.md#control-flow-lowering) | A failure that occurs when a call needs more stack frames than exist.                                    |
| [START](backend.md#composite-modules)              | The signal that enables the circuit clock.                                                               |
| Static call                                        | A call whose target is known while compiling.                                                            |
| [Tick](backend.md#composite-modules)               | One Factorio engine update.                                                                              |
| Variadic function                                  | A function that accepts a variable number of arguments.                                                  |
| Wrapping                                           | Integer behaviour in which passing one end of the range continues at the other end.                      |

## Feature matrix

The table compares runtime support with standard Go on amd64 or arm64.

| Capability              | gofactos                                |
|-------------------------|-----------------------------------------|
| Function signatures     | Partial: exact `int`/`bool`; one result |
| Integer operations      | Partial: signed `int32` core            |
| Bitwise and shifts      | Missing                                 |
| Floating point          | Missing                                 |
| Branches and merges     | Partial: limited control flow           |
| Loops                   | Partial: one counted root loop          |
| Ordinary calls          | Partial: static expansion               |
| Recursion               | Partial: direct self, 13 frames         |
| Methods and closures    | Missing                                 |
| Dynamic/interface calls | Missing                                 |
| Pointers and aggregates | Missing                                 |
| Goroutines and channels | Missing                                 |

A normal Go executable is a native binary. A gofactos executable is a Factorio
blueprint whose circuit runs inside Factorio.

## Supported Go

- The root is not a method or closure. Every compiled function returns one
  value.
- Parameters and results use the exact Go types `int` or `bool`.
- Runtime operators are `+`, `-`, `*`, `/`, `%`, unary integer `-`, and the six
  comparisons `==`, `!=`, `<`, `<=`, `>`, and `>=`.
- In non-recursive code, a merge chooses between two values. Merges may be
  chained, but a function has at most two compile-time-reachable `Return`
  instructions. One runtime branch must lie on every feasible path to both
  returns. Each arm may pass through later supported branches and merges before
  reaching its unique return.
- Short-circuit `&&`, `||`, and Boolean `!` work when they compile to supported
  branches and merges. A `!` that survives as an operation is unsupported.

## Calls

- Each call resolves statically to a top-level, non-generic, non-variadic
  function with a source body in the same package.
- Runtime built-in calls fail. Dynamic, external, method, and closure calls
  fail. `go` and `defer` statements also fail.
- Non-recursive calls and loops cannot coexist in the reachable code.
- Ordinary calls cannot form a cycle. Compilation may create at most 1,024
  function copies, including the root.

## Loops

- The root may have one unnested loop.
- Every loop counts from zero by one while `i < bound`. The bound is an `int`
  parameter or constant, not a computed value. Constants are non-negative.
  Parameter bounds at or below zero run no iterations.
- A simple loop accepts `for i := 0; i < bound; i++` or `for range bound`.
- A simple loop returns its final counter, `0`, or a running total that
  starts at zero and changes only by `result += c`, where `c` is an integer
  constant. Its runtime-reachable path cannot branch or exit early.
- An additive recurrence, such as Fibonacci, updates at least two `int` states.
  It uses the indexed `for` form, not `for range`. On its runtime-reachable
  path, setup only initialises state, the body only adds values and updates
  state, and the exit returns one state.
- Literal-constant detours and the one-input phi aliases they leave may appear
  before, inside, or after a loop. They do not make an impossible `break`,
  `continue`, return, call, or operation part of the supported runtime shape.
  A branch or early exit whose condition is not a constant remains unsupported.
- An auxiliary loop-invariant phi whose feedback is itself aliases its entry
  value when used, or disappears when unused. It is not a clocked state
  register.
- Clocked recurrence states start from signed `int32` constants. A next state
  may copy an old state or add old states, parameters, constants, and earlier
  sums from the same iteration. Every sum contributes to a state update; every
  non-counter state affects the result.
- A recurrence body's longest chain may contain at most 58 dependent additions
  at 1 Hz, or 13 with `--fast`.
  Signal allocation may impose a lower practical limit.
- A recurrence waits one clock period before its first update. Later state
  updates happen together. With inputs unchanged, every supported loop advances
  only while `i < bound` is true, then holds its final result.

## Recursion

- The root may call itself directly. It cannot contain a loop or call another
  function. The recursive function must be the root; a wrapper cannot call it.
  Mutual recursion fails.
- Recursive code supports the listed operators, branches, value merges, and
  direct self calls. It may have several branches, calls, and returns.
- Each stack frame stores the current step and up to ten `int` or `bool` values.
  Thirteen frames hold the root and twelve nested calls.
- A deeper call enters `STACK OVERFLOW` and shows no result.
- After the root returns, the `RUNNING` indicator stays visible for one clock
  step while the display settles. The next step shows `DONE`. This confirms
  completion, especially for zero, whose signal is absent from the wire.

## Values, inputs, and display

- Integers use signed `int32`. Remaining integer constants and `int` values
  passed with `--set` must fit that range. Arithmetic and network sums wrap
  within it.
- Division truncates towards zero. Remainder keeps the dividend's sign. A zero
  divisor produces zero instead of a Go panic.
- Booleans use `1` for true and `-1` for false. Zero means no signal.
- Root parameters default to `1`. `--set name=value` bakes a root input into the
  blueprint; an unknown name fails compilation. For a `bool` parameter,
  `--set name=0` means false and any other integer means true. The value must
  be an integer; boolean literals are not accepted.
- Unused parameters in non-recursive roots emit no input, even when set.
  Recursive roots keep every declared parameter.
- Only the root gets editable parameter inputs, a result display, and the
  blueprint label.
- An `int` result displays values from 0 to 99,999,999. Other values have no
  display contract.
- A `bool` result shows a check for true and a deny symbol for false.

## Clock and reset

- A root with a loop or recursion has one clock: 60 Factorio ticks (1 Hz), or
  15 ticks (4 Hz) with `--fast`.
- Its `START / RESET` control defaults to off. Leave it off until every entity,
  including inputs, is built. Turning it on starts the clock. Turning it off
  restores loop entry values and clears recurrence warm-up and recursive state.
- Keep it off for at least one full clock period before restarting. Power
  cycling has no reset contract.
- A blueprint without a loop or recursion has no clock or START control and
  ignores `--fast`.

## Resource limits

- A retained root parameter takes the signal letter at its declared position.
  The first uses `signal-A`. Position 27 or later fails even when earlier
  parameters are unused.
- At most 21 public values and controls may exist beyond root inputs. Each uses
  one signal even when it feeds several components. Signals are not reused, and
  private component wiring does not count.
