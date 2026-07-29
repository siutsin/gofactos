# Testing

Install the pinned development tools with `mise install`. Commands below assume
Mise is active; otherwise, prefix them with `mise exec --`.

## Glossary

| Term                                             | Meaning                                                                                            |
|--------------------------------------------------|----------------------------------------------------------------------------------------------------|
| [Blueprint](blueprint.md)                        | A Factorio plan describing entities, their settings, and their wires.                              |
| Dataflow                                         | Source comments that sketch value dependencies.                                                    |
| Draftsman                                        | A Python library used to parse and validate blueprint exchange strings without launching Factorio. |
| E2E                                              | An end-to-end test that exercises the full path through the CLI and Factorio.                      |
| [Exchange string](blueprint.md#encoding)         | The compressed text form of a blueprint that Factorio imports and exports.                         |
| Headless Factorio                                | The Factorio server and game engine running without its graphical interface.                       |
| Layout                                           | Source comments that show emitted entity positions.                                                |
| Lua                                              | Factorio's scripting language.                                                                     |
| [Mise](../README.md#development)                 | The tool manager that installs this project's pinned utilities.                                    |
| Mod                                              | An installable package that adds to or changes Factorio.                                           |
| [Quality](blueprint.md#entity)                   | Factorio's system of entity grades.                                                                |
| Race detector                                    | A Go tool that finds unsafe concurrent memory access.                                              |
| RCON                                             | The authenticated protocol used to control the running Factorio server.                            |
| [Simulation](backend.md#how-the-simulator-works) | Running a circuit model without launching Factorio.                                                |
| `uv`                                             | The package manager and runner for the locked Draftsman environment.                               |
| Zizmor                                           | A security checker for GitHub Actions workflows.                                                   |

## Local Tests

Run the normal check:

```sh
make test
```

It formats Go, lints Go, Lua, and Markdown, and runs every internal test except
the opt-in live Factorio scenario.

Use `make test-race` to run the same suite with the race detector.
Use `make lint-lua` for a focused check of the embedded Lua programs.

## Draftsman

Draftsman tests use `uv run --locked`, so local runs and CI use the dependency
versions recorded in `uv.lock`. `pyproject.toml` declares Draftsman, and
`mise install` provides the pinned `uv` executable.

Inspect one generated blueprint and print its entities and wires with:

```sh
make validate FILE=internal/testdata/fori.go
```

## Expected Output Files

Some command-line test cases compare their JSON output with expected output
files in `internal/testdata/blueprints/`. Update those files with:

```sh
go test ./internal/integration \
  -run '^TestBlueprintExpectedOutput$' \
  -args -update-expected-outputs
```

Review every changed JSON file, then run `make test`. Keep the set of complete
expected outputs small. Set `expectedOutput` only when a case adds a distinct
output contract.

## Adding Test Cases

- For supported non-recursive code, add source under `internal/testdata/`,
  register a `blueprintCase` in `internal/integration/cases_test.go`, and add or
  extend a simulator test under `internal/factorio/`.
- For recursion, add source under `internal/testdata/recursive/`; extend the
  command-line JSON checks in `internal/integration/recursive_test.go`, the
  simulator cases in `internal/factorio/recursive_test.go`, and the Draftsman
  cases in `internal/factorio/call_test.go`.
- For rejected code, add source under `internal/testdata/negative/` and add its
  command-line case to `internal/integration/negative_test.go`.
- Add focused command-line JSON checks under `internal/integration/` when a
  complete expected-output file would add no distinct contract.
- Update any `Dataflow` or `Layout` source diagram when its logic or emitted
  coordinates change.

## Continuous Integration

CI runs `make test-race` and `git diff --exit-code`.

The GitHub Actions Security Analysis workflow also runs Zizmor on pushes and
pull requests and reports findings as workflow annotations.

## Headless Factorio E2E

The opt-in E2E test runs generated blueprints in Factorio.

### Prerequisites

- Factorio 2.0 with its `data` directory. The `quality` mod ships with the
  Space Age expansion, so Space Age must be installed. The `space-age` mod
  itself may stay disabled
- a loadable Factorio 2.0 save
- a mod directory containing `mod-list.json`, with `base` and `quality` enabled

`mod-settings.dat` is optional. A new base-game save is sufficient; the test
creates its own surfaces and entities.

### Default Paths

On macOS, this prints the default Factorio path:

```sh
steam_apps="$HOME/Library/Application Support/Steam/steamapps/common"
factorio_bin="$steam_apps/Factorio/factorio.app/Contents/MacOS/factorio"
printf '%s\n' "$factorio_bin"
```

Override it with `GOFACTOS_FACTORIO_BIN` (see the overrides table below).

- save: `~/Library/Application Support/factorio/saves/gofactos.zip`
- mods: `~/Library/Application Support/factorio/mods`

On other platforms:

- Factorio: `factorio` on `PATH`
- save: `~/.factorio/saves/gofactos.zip`
- mods: `~/.factorio/mods`

The test finds the `data` directory beside the Factorio binary. Use an override
for another layout.

### Run

```sh
make test-e2e
```

The target builds `build/gofactos` and runs only `internal/e2e`.

### Execution Flow

The E2E test uses the same blueprint produced for a user:

1. The test runs the built `gofactos` CLI to generate a blueprint exchange
   string.
2. The test sends an embedded Lua harness to Factorio over RCON.
3. The harness imports that exact string, places its entities on an isolated
   surface, supplies power, and operates its START control.
4. Factorio's real circuit engine runs the placed blueprint.
5. The harness reads live signals, displays, construction state, and game
   ticks; Go compares those observations with the expected result.

The Lua harness does not recreate or replace the generated circuit. It only
places, controls, and observes it. The readable harness programs live in
[`internal/e2e/testdata/`](../internal/e2e/testdata/).

### Factorio Command

The harness launches a command shaped like this:

```sh
factorio \
  --config /tmp/gofactos/config.ini \
  --mod-directory /tmp/gofactos/mods \
  --start-server /tmp/gofactos/saves/gofactos-e2e.zip \
  --server-settings /tmp/gofactos/server-settings.json \
  --bind 127.0.0.1 \
  --port 34197 \
  --rcon-bind 127.0.0.1:27015 \
  --rcon-password local-e2e-example \
  --server-id /tmp/gofactos/server-id-1.json \
  --no-log-rotation
```

The real command uses a temporary directory, unused ports, and a random
password. RCON is the authenticated TCP protocol used to send Factorio console
commands and read their output.

### Overrides

| Variable                    | Purpose                                  |
|-----------------------------|------------------------------------------|
| `GOFACTOS_BIN`              | Built `gofactos` executable              |
| `GOFACTOS_FACTORIO_BIN`     | Factorio executable                      |
| `GOFACTOS_FACTORIO_DATA`    | Factorio `data` directory                |
| `GOFACTOS_FACTORIO_SAVE`    | Source save                              |
| `GOFACTOS_FACTORIO_MOD_DIR` | Mod directory containing `mod-list.json` |

### Isolation

The test copies the save, mod list, optional mod settings, and enabled
third-party mods into `t.TempDir()`. All writable paths use that directory.
The server binds to `127.0.0.1` and disables public listing, LAN discovery, and
autosaves. It never uses `--sync-mods`.

The source save, live configuration, mod directory, and graphical game are not
modified.

### Coverage

The test covers blueprint loading, construction safety, reset and rerun, clock
cadence, numeric and Boolean displays, completion and stack-overflow states,
multi-argument recursion, a true Boolean result through recursive frames, long
relay routing, signed division and remainder, and signed `int32` wraparound.
Exact assertions are grouped by responsibility in
[`internal/e2e/`](../internal/e2e/), with the scenario list in
[`factorio_test.go`](../internal/e2e/factorio_test.go).

### References

- [Command line parameters][command-line]
- [Dedicated/headless server][headless]
- [Source RCON protocol][rcon]

[command-line]: https://wiki.factorio.com/Command_line_parameters
[headless]: https://wiki.factorio.com/Multiplayer#Dedicated/Headless_server
[rcon]: https://developer.valvesoftware.com/wiki/Source_RCON_Protocol
