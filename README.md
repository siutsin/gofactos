# gofactos

gofactos is built for my GopherCon 2026 talk,
[From Go to Factorio: What Games Can Teach Us About Compilers][talk]. It turns
a supported Go function into a Factorio 2.0 blueprint and can print the
function's static single assignment (SSA) form.

gofactos uses the public, source-oriented
[`golang.org/x/tools/go/ssa`][x-tools-ssa]. The Go compiler has a separate,
lower-level SSA implementation at
[`cmd/compile/internal/ssa`][compiler-ssa], but Go's `internal` package rules
keep it private to the compiler. See the [SSA guide](docs/ssa.md) for details.

[compiler-ssa]: https://go.dev/src/cmd/compile/internal/ssa/
[talk]: https://www.gophercon.com/agenda/session/1880654
[x-tools-ssa]: https://pkg.go.dev/golang.org/x/tools/go/ssa

## Requirements

- Go 1.26 or later
- Factorio 2.0 with Space Age's Quality mod enabled

## Install From Source

```sh
make install
```

This builds `build/gofactos` and creates a symlink at
`~/.local/bin/gofactos`. Ensure `~/.local/bin` is on `PATH`. Use `make build`
to build without installing.

## Quick Start

Generate a blueprint string:

```sh
gofactos blueprint demo/loop.go
```

Copy the output into Factorio's **Import string** dialog. See
[Clock and reset](docs/spec.md#clock-and-reset) to run the loop.

Inspect the blueprint JSON or SSA:

```sh
gofactos blueprint --json demo/loop.go
gofactos analyse demo/loop.go
```

The [specification](docs/spec.md) defines the supported Go subset.

## Development

Install the pinned tools with [Mise](https://mise.jdx.dev/):

```sh
mise install
mise exec -- make test
```

See the [testing guide](docs/testing.md) for race and Factorio E2E tests.

## Documentation

See the [documentation index](docs/README.md) for project goals, supported
features, design, and development guides.

## AI Usage

This project is absolutely just for fun and educational purposes and might
contain AI slop. Use at your own risk ¯\\\_(ツ)\_/¯.
