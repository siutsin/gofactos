# Static Single Assignment

This guide introduces the compiler terms behind the SSA consumed by gofactos.

## Glossary

| Term                             | Meaning                                                                                                     |
|----------------------------------|-------------------------------------------------------------------------------------------------------------|
| Abstract syntax tree (AST)       | A nested representation of parsed source code.                                                              |
| Basic block                      | A straight-line instruction sequence ending in a control transfer such as a branch, jump, return, or panic. |
| Compiler pass                    | One analysis or transformation over a program representation.                                               |
| Constant propagation             | Replacing calculations with known values.                                                                   |
| Control flow                     | Tracking which operation can run next.                                                                      |
| Control-flow graph (CFG)         | Basic blocks as nodes and possible transfers between them as directed edges.                                |
| Data flow                        | Tracking values between operations.                                                                         |
| Dead-code elimination            | Removing work that cannot affect the result.                                                                |
| Dominator                        | A block that every path from the function entry to another block must pass through.                         |
| Instruction                      | One operation that can produce a value.                                                                     |
| Intermediate representation (IR) | A program form used inside a compiler.                                                                      |
| Lexing                           | Splitting source text into tokens.                                                                          |
| Parsing                          | Arranging source tokens into an abstract syntax tree (AST).                                                 |
| Phi node                         | A value where control-flow paths meet that selects the input from the path that arrived there.              |
| Static single assignment (SSA)   | An IR in which each value is defined once.                                                                  |
| Type checking                    | Verifying that operations use compatible types.                                                             |
| Value                            | A result, parameter, or constant that an instruction can read.                                              |
| x/tools SSA                      | Go's public SSA library for analysis tools. It is separate from the Go compiler's private SSA.              |

## What Is SSA?

Static single assignment (SSA) is an intermediate representation used by
compilers. Each SSA value is defined once. When source code updates a variable,
SSA creates a new value instead of overwriting the old one.

For example, this Go code:

```text
x := 1
x = x + 2
```

becomes:

```text
x0 = 1
x1 = x0 + 2
```

Each SSA value has one definition, so every use refers to one specific value.

## Why SSA Helps

- **Clear data flow:** every value has one definition.
- **Clear control flow:** basic blocks record branches and joins.
- **Simple analysis:** compiler passes can trace uses without tracking
  assignments to the same source variable.
- **Useful optimisations:** constant propagation and dead-code elimination are
  easier on SSA.

## Compiler Flow

```text
Source code
  → Lexing / parsing (Abstract Syntax Tree (AST))
  → Type checking
  → SSA construction    ← we are here
  → Optimisation passes
  → Code generation (machine code or bytecode)
```

SSA usually sits between type checking and optimisation. The Go compiler uses
its own private SSA implementation. `golang.org/x/tools/go/ssa` is a separate
package for building and analysing SSA from Go programs.

## x/tools Concepts

- **Package:** the SSA form of one Go package.
- **Function:** a function split into basic blocks.
- **Basic block:** a straight-line instruction sequence ending in an explicit
  control transfer.
- **Instruction:** one operation, such as `BinOp`, `Call`, `If`, or `Return`.
- **Value:** an instruction, parameter, or constant that other instructions
  can use.
- **Phi node:** a value at a block join. It selects the input from the block
  that transferred control.

## Control-Flow Graphs

A control-flow graph (CFG) represents each basic block as a node. A branch or
jump adds directed edges to the blocks that may run next.

x/tools SSA keeps both edges of `if true` and `if false`, even though only one
arm can execute. Before lowering, gofactos derives a feasible CFG that keeps
only the constant-selected arm. It then:

- visits only blocks and calls reachable through feasible edges;
- removes phi inputs that arrive only from impossible edges;
- aliases a one-input phi directly to its producer in ordinary and loop
  lowering, while recursive planning retains the equivalent edge move; and
- recomputes the immediate dominator used to map mid-function phi merges during
  ordinary lowering from the feasible paths.

This is not general dead-code elimination. A reachable calculation may still
be removed later when its result is unused; feasible control flow answers the
different question of whether the calculation can execute at all.

## SSA in gofactos

gofactos uses x/tools SSA as its input representation. The
[architecture guide](architecture.md) shows where SSA sits in the command
pipeline. The [backend guide](backend.md) explains how supported SSA becomes a
Factorio circuit.
