# Coding Style

## Glossary

| Term                                          | Meaning                                                                                            |
|-----------------------------------------------|----------------------------------------------------------------------------------------------------|
| [Blueprint](blueprint.md)                     | A Factorio plan describing entities, their settings, and their wires.                              |
| [Draftsman](testing.md#draftsman)             | A Python library used to parse and validate blueprint exchange strings without launching Factorio. |
| [E2E](testing.md#headless-factorio-e2e)       | An end-to-end test that exercises the full path through the CLI and Factorio.                      |
| [Internal package](architecture.md#ownership) | A package under `internal/` that can be imported only from its permitted parent tree.              |
| `log/slog`                                    | Go's standard structured-logging package.                                                          |
| [RCON](testing.md#factorio-command)           | The authenticated protocol used to send commands to the E2E Factorio server.                       |
| Structured logging                            | Logging that stores named fields.                                                                  |

## General

- Write dates as `D MMM YYYY` or `YYYY-MM-DD`.

## Go

- Keep `main.go` limited to process setup. Put application logic in `internal/`.
- Use `log/slog` for logs.
- Name Go files with short lowercase nouns. Rely on package context instead of
  repeating subsystem names.
- Reserve underscores for Go-recognised suffixes such as `_test.go`,
  `_linux.go`, and `_amd64.go`.

## Comments

- Start every Go file with a concise `// This file ...` comment that explains
  why the file exists.
- Give every named function and method a concise, name-led comment. This
  includes tests and helpers. Anonymous functions are exempt.

## Tests

- Prefer [testify](https://github.com/stretchr/testify) for assertions. Use
  native testing failures when clearer.
- Omit assertion messages unless they add context the assertion lacks.
- Call `validateWithDraftsman()` in blueprint importability tests.
- Validate command-line JSON output directly. Do not repeat the Draftsman check
  there.

## Lua

- Keep embedded E2E programs directly under `internal/e2e/testdata/`.
- Use two-space indentation and keep source to 80 columns, as configured by
  `.editorconfig` and `.luacheckrc`.
- Run `make lint-lua`; `.luacheckrc` permits the Factorio globals and argument
  names injected by the Go harness.
- Do not add Lua comments. The RCON loader flattens each program to one line and
  rejects the `--` comment marker.

## Documentation

- Keep prose to 80 characters or fewer. Tables and code blocks are exempt.
- Keep glossary terms plain; do not bold them.
- Use lowercase filenames under `docs/`, except `README.md`.
- `spec.md` owns supported behaviour and limits.
- Link to canonical guides instead of repeating their rules.
- Public documentation must not depend on agent instruction files.
