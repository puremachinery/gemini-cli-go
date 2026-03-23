# Contributing

Thanks for considering a contribution! This project is a Go-first reimplementation
inspired by `google-gemini/gemini-cli` and is under active development.

## Before you start

- Please open or comment on an issue so we can align on scope.
- Keep PRs focused and small.
- Follow the existing coding style.

## Quickstart (first 10 minutes)

1. Verify prerequisites (Go 1.25.0+):

```bash
go version
```

2. Clone and enter the repo:

```bash
git clone https://github.com/puremachinery/gemini-cli-go.git
cd gemini-cli-go
```

3. Run baseline checks:

```bash
go test ./...
```

4. Run the CLI locally:

```bash
go run ./cmd/gemini-cli --help
go run ./cmd/gemini-cli --version
```

5. Optional: enable API key auth for manual testing:

```bash
export GEMINI_API_KEY="YOUR_GEMINI_API_KEY"
go run ./cmd/gemini-cli --prompt "ping"
```

## Development workflow

1. Fork the repo and create a feature branch.
2. Make your changes.
3. Run formatting and tests:

```bash
gofmt -w ./...
go test ./...
```

If you have `golangci-lint` installed, run:

```bash
env GOTOOLCHAIN=go1.25.0 golangci-lint run --timeout=5m
```

The optional pre-commit hook only auto-runs `golangci-lint` when your local
`go env GOVERSION` is on the currently supported Go 1.25.x line. Keep that
hook gate and the pinned `GOTOOLCHAIN=go1.25.0` command in sync when the
repo's minimum supported Go toolchain changes. The full `go1.25.0` form is
intentional: `GOTOOLCHAIN` requires a concrete toolchain version, not just the
language version (`go1.25`).

## Smoke test (manual)

For a quick manual pass that builds a temp binary, runs basic checks, and
walks through interactive steps:

```bash
./scripts/smoke.sh
```

## Local hooks (recommended)

This repo uses optional local hooks under `.githooks` to catch issues early:

```bash
git config core.hooksPath .githooks
```

- `pre-commit`: gofmt, gitleaks (if installed), golangci-lint (if installed), go test.
- `pre-push`: go vet; optionally run race tests with `RUN_RACE=1 git push`.

## Commit and PR guidelines

- Use clear, descriptive commit messages.
- Update documentation when behavior or CLI UX changes.
- Include tests when behavior is non-trivial or easily regresses.

## Reporting issues

- Use GitHub Issues with reproduction steps and environment details.
- For security issues, see `SECURITY.md`.

Thanks again for contributing!
