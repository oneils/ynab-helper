# Contributing

Contributions are welcome. Please read this guide before submitting a pull request.

## Before you start: open an issue

For anything beyond a trivial typo fix, **open an issue before writing code.** Describe what you want to change and why. This avoids duplicate effort and ensures the change aligns with the project's direction before you invest time in implementation.

For bugs, include steps to reproduce. For features or new bank parsers, describe the use case and attach a sample CSV if possible (anonymize any personal data first).

## Prerequisites

- Go 1.25+
- Make

## Running locally

```bash
cp .env.example .env   # fill in YNAB_TOKEN at minimum
make run               # starts the dev server on :8080
```

## Running tests and linter

```bash
make check   # runs golangci-lint + all tests
make test    # tests only
make lint    # linter only
```

All tests must pass and the linter must be clean before submitting a PR.

## Adding a new bank parser

Each bank has its own parser file under `internal/parser/`. Use `pko.go`, `revolut.go`, or `santander.go` as a reference:

1. Create `internal/parser/<bank>.go` implementing the `Parser` interface
2. Add a `_test.go` file with unit tests covering the expected CSV format
3. Register the parser in `internal/parser/parser.go`

Use clearly fictional data in tests — no real account numbers, transaction references, or balances.

## Commit conventions

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(parser): add ING bank parser
fix(ui): correct amount formatting for negative values
docs: update README with new env vars
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`

See `.gitmessage` for the full reference.

## Pull requests

- Open an issue first (see above)
- Keep PRs focused — one logical change per PR
- Reference the related issue in the PR description
- CI must pass (build + lint + tests)
