# ynab-helper

[![build](https://github.com/oneils/ynab-helper/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/oneils/ynab-helper/actions/workflows/ci.yml)

A self-hosted web app that imports CSV transaction exports from Polish banks directly into [YNAB](https://www.youneedabudget.com/), without relying on third-party bank sync partners.

> **Disclaimer:** This is an unofficial community project and is not affiliated with, endorsed by, or supported by YNAB. YNAB is a registered trademark of You Need A Budget LLC.

**Why?** Polish banks (Santander PL, PKO, Revolut, Millennium) are not supported by YNAB's native sync. Third-party integrations require sharing banking credentials with an external service. This tool keeps your data local: export a CSV from your bank, upload it here, review, confirm — done.

## Supported banks

- [x] Santander Polska
- [x] Revolut
- [x] PKO
- [x] Millennium
- [ ] ING

## How it works

1. In **Settings → Parser Mappings**, map each YNAB account to the bank parser used for its statements
2. Export a transaction CSV from your bank's web interface
3. In ynab-helper, select your YNAB budget and account, upload the CSV
4. Review the preview — new vs. duplicate transactions are highlighted
5. Confirm the import; transactions are pushed to YNAB

The app deduplicates by hashing a stable subset of each transaction's fields (excluding volatile columns like running balance or settlement date, which can change between exports of the same real-world transaction), so re-uploading the same file or an overlapping export is safe.

> **Parser mapping required.** Each YNAB account must be explicitly mapped to a bank parser in **Settings → Parser Mappings** before you can upload a CSV for it. Uploads for unmapped accounts fail with a clear error until you set the mapping.

### Import CSV file
<img width="1965" height="1508" alt="image" src="https://github.com/user-attachments/assets/4ee2e317-4f80-4e2b-8344-8918ae202ffe" />

<img width="1753" height="1500" alt="image" src="https://github.com/user-attachments/assets/67d16cd8-edce-4cb8-94e2-ea48bd6f63c4" />

### Import History

<img width="1788" height="1486" alt="image" src="https://github.com/user-attachments/assets/950b6920-667d-4fcc-b35a-37b8584d06ee" />


## Prerequisites

- A YNAB Personal Access Token — go to **Settings → Developer Settings → New Token** in your YNAB account. The token gives read/write access to your budgets via the [YNAB API](https://api.ynab.com). Treat it like a password: never commit it to version control.
- Docker (recommended) or Go 1.25+

## Quick start

**Using the pre-built image (fastest):**

```bash
docker run -d \
  -e YNAB_TOKEN=your_token \
  -p 8080:8080 \
  -v $(pwd)/data:/data \
  oneils/ynab-helper:latest
```

Images are published to both [Docker Hub](https://hub.docker.com/r/oneils/ynab-helper) (`oneils/ynab-helper`) and GHCR (`ghcr.io/oneils/ynab-helper`).

**Using docker compose (recommended for persistent setup):**

```bash
git clone https://github.com/oneils/ynab-helper.git
cd ynab-helper
cp .env.example .env        # fill in YNAB_TOKEN
docker compose up -d
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
| `account_parser_mappings` | Per-account bank parser assignments configured in Settings |

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
