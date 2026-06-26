# ynab-helper

![Build Status](https://github.com/oneils/ynab-helper/actions/workflows/docker-image.yml/badge.svg)

Tool for processing reported from different bank accounts and exporting them to YNAB app.

Supported reported from the following Banks:

- [x] Santander Polska
- [x] Revolut
- [x] PKO
- [ ] ING

## Database

The application uses SQLite with SQL migrations for data storage.

### Tables

- `transactions` - stores successfully imported/processed transactions from the Bank's report files
- `budgets` - stores budgets imported from YNAB
- `accounts` - stores YNAB accounts associated with budgets
- `category_groups` - stores YNAB category groups
- `categories` - stores YNAB categories
- `payees` - stores YNAB payees
- `sync_history` - stores YNAB sync history (e.g. last sync date and which entities were synced)

## Transaction record example

```json
{
    "id": "229102766bfb459f6503426598c47f7285cf2daf",
    "amount": 100.00,
    "currency": "PLN",
    "description": "Transaction description",
    "payee": "Payee name",
    "account_id": "account123",
    "account_name": "Santander",
    "status": "PROCESSED",
    "created_at": "2014-01-01T12:00:00Z",
    "txn_time": "2014-01-01T12:00:00Z",
    "raw_text": "Original CSV line",
    "raw_line_number": 5,
    "error_msg": null
}
```

Fields description:

- `id` - SHA1 hash of the whole line from the CSV report. Used for duplicate verification
- `account_name` - from which account a record was exported (e.g. `Santander` or `Revolut`)
- `status` - transaction status: `DRAFT`, `SKIPPED`, `PROCESSED`, or `INVALID`
- `account_id` - YNAB account ID for the transaction
- `txn_time` - transaction date and time


## How to run

```bash
env YNAB_TOKEN=token \
    YNAB_API=https://api.youneedabudget.com/v1 \
    DB_PATH=./ynab-helper.db \
    go run app/main.go
```

The application will automatically run SQL migrations on startup to create/update the database schema.

## Generate mocks

```bash
go generate ./app/...
```

## Create a new release

```bash
git tag -a v0.0.2 -m "Release v0.0.2"
git push origin v0.0.2
```
