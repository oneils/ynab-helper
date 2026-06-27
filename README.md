# ynab-helper

[![build](https://github.com/oneils/ynab-helper/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/oneils/ynab-helper/actions/workflows/ci.yml)

A self-hosted web app that imports CSV transaction exports from Polish banks directly into [YNAB](https://www.youneedabudget.com/), without relying on third-party bank sync partners.

**Why?** Polish banks (Santander PL, PKO, Revolut) are not supported by YNAB's native sync. Third-party integrations require sharing banking credentials with an external service. This tool keeps your data local: export a CSV from your bank, upload it here, review, confirm — done.

## Supported banks

- [x] Santander Polska
- [x] Revolut
- [x] PKO
- [ ] ING
- [ ] Millennium

## How it works

1. Export a transaction CSV from your bank's web interface
2. In ynab-helper, select your YNAB budget and account, upload the CSV
3. Review the preview — new vs. duplicate transactions are highlighted
4. Confirm the import; transactions are pushed to YNAB

The app deduplicates by SHA-256 hash of each CSV line, so re-uploading the same file or an overlapping export is safe.

> **Account naming matters.** The parser is selected by checking whether your YNAB account name *contains* the bank name (case-insensitive). An account named `PKO Something` or `My Santander` works fine — as long as `pko` or `santander` appears somewhere in the name.

## Prerequisites

- A [YNAB Personal Access Token](https://api.youneedabudget.com/#authentication-overview) (Settings → Developer Settings → New Token)
- Docker (recommended) or Go 1.25+

## Quick start

```bash
git clone https://github.com/oneils/ynab-helper.git
cd ynab-helper
cp .env.example .env        # then fill in YNAB_TOKEN
docker compose up
```

Open [http://localhost:8080](http://localhost:8080).

## Running locally (without Docker)

```bash
YNAB_TOKEN=your_token make run
```

The dev server starts on `:5002`.

## Configuration

All options can be set as environment variables or CLI flags.

| Env var | Flag | Default | Description |
|---|---|---|---|
| `YNAB_TOKEN` | `--ynab-token` | — | YNAB Personal Access Token (**required**) |
| `ADDR` | `--addr` | `:8080` | HTTP listen address |
| `YNAB_API` | `--ynab-api` | `https://api.youneedabudget.com/v1` | YNAB API base URL |
| `SYNC_INTERVAL` | `--sync-interval` | `1h` | How often to automatically sync YNAB data (budgets, accounts, payees, categories) |
| `SQLITE_DB_PATH` | `--sqlite-path` | `./data/ynab.db` | SQLite database file path |

## Database

SQLite with automatic migrations on startup.

| Table | Contents |
|---|---|
| `transactions` | Imported bank transactions |
| `budgets` | Budgets synced from YNAB |
| `accounts` | YNAB accounts |
| `category_groups` | YNAB category groups |
| `categories` | YNAB categories |
| `payees` | YNAB payees |
| `sync_history` | Last sync timestamps per entity type |

## Transaction statuses

| Status | Meaning |
|---|---|
| `DRAFT` | Imported from CSV, not yet pushed to YNAB |
| `PROCESSED` | Successfully pushed to YNAB |
| `SKIPPED` | Manually skipped |
| `INVALID` | Could not be parsed |

## Development

```bash
make check      # fmt + vet + lint + test
make test       # tests with race detector and coverage
make build      # cross-compile for linux/amd64 → target/ynab-helper
make docker     # build Docker image locally
```

Regenerate mocks after changing interfaces in `internal/parser`:

```bash
go generate ./internal/parser/...
```

## Create a release

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

CI builds and pushes multi-arch Docker images to Docker Hub and GHCR on every tag.
