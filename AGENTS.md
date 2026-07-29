# gofactos

Go CLI using `urfave/cli/v3`.

## Workflow

- Before changing a file, read every `AGENTS.md` from the repository root to
  its directory; the closest one takes precedence.
- Follow [docs/style.md](docs/style.md) for code, tests, comments, and public
  documentation.
- Use the tools pinned by Mise. Run `mise install`; use `mise exec --` when
  Mise is not activated.
- Write build artifacts only under `build/`.

## Validation

- Documentation only: run `make lint`.
- Go: use narrow tests or `make test` while iterating; finish with
  `make test-race`.
- Factorio runtime changes: also run `make test-e2e` when Factorio is
  available; otherwise rely on simulator tests.
