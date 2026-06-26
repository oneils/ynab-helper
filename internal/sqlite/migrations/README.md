# Database Migrations

This directory contains SQL migrations for the YNAB Helper application database.

## Overview

We use [goose](https://github.com/pressly/goose) for managing database schema changes. Migrations are automatically applied when the application starts.

## Migration Files

Migration files follow the naming convention:
```
{version}_{description}.sql
```

Examples:
- `00001_initial_schema.sql` - Initial database schema
- `00002_add_user_preferences.sql` - Add user preferences table

Each migration file contains both "up" and "down" migrations separated by `-- +goose Down`:

```sql
-- +goose Up
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS users;
```

### File Components

- **version**: 5-digit sequential number (00001, 00002, etc.)
- **description**: Short snake_case description of the change
- **Up section**: SQL to apply the migration (before `-- +goose Down`)
- **Down section**: SQL to rollback the migration (after `-- +goose Down`)

## Creating a New Migration

### Using goose CLI

Install the goose CLI tool:
```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Create a new migration:
```bash
cd internal/sqlite/migrations
goose create add_transaction_tags sql
```

This creates a new file like `00002_add_transaction_tags.sql` with a template:

```sql
-- +goose Up
-- +goose StatementBegin
-- Your SQL here
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Your SQL here
-- +goose StatementEnd
```

### Manual Creation

Create a file with the next sequential number:

```bash
touch internal/sqlite/migrations/00002_add_transaction_tags.sql
```

**00002_add_transaction_tags.sql:**
```sql
-- +goose Up
ALTER TABLE transactions ADD COLUMN tags TEXT;
CREATE INDEX idx_transactions_tags ON transactions(tags);

-- +goose Down
DROP INDEX IF EXISTS idx_transactions_tags;
-- Note: SQLite doesn't support DROP COLUMN directly
```

## Migration Best Practices

### 1. Always Include Both Up and Down

Every migration MUST have both up and down sections, even if down is just a comment explaining why it can't be reversed.

### 2. Make Migrations Atomic

Each migration should be a single logical change. Don't combine unrelated schema changes.

### 3. Test Your Migrations

Before committing:
```bash
# Start fresh
rm -rf ./data/ynab.db*

# Run application (applies migrations)
go run ./cmd/ynab-helper

# Verify schema
sqlite3 ./data/ynab.db ".schema"

# Test rollback (optional - using goose CLI)
goose -dir internal/sqlite/migrations sqlite3 ./data/ynab.db down
goose -dir internal/sqlite/migrations sqlite3 ./data/ynab.db up
```

### 4. Never Modify Existing Migrations

Once a migration is committed and deployed, NEVER modify it. Create a new migration instead.

### 5. Handle Data Carefully

When migrations affect existing data:

```sql
-- +goose Up
-- WRONG: This fails if table has data
ALTER TABLE transactions ADD COLUMN new_field TEXT NOT NULL;

-- RIGHT: Add nullable, populate, then add constraint if needed
ALTER TABLE transactions ADD COLUMN new_field TEXT;
UPDATE transactions SET new_field = 'default_value' WHERE new_field IS NULL;
```

### 6. SQLite Limitations

SQLite has limited ALTER TABLE support:
- Cannot DROP COLUMN (need to recreate table)
- Cannot ADD CONSTRAINT (need to recreate table)
- Cannot MODIFY COLUMN (need to recreate table)

For complex changes, use the recreate pattern:
```sql
-- +goose Up
CREATE TABLE transactions_new (
    id TEXT PRIMARY KEY,
    -- new schema
);

INSERT INTO transactions_new SELECT id, ... FROM transactions;
DROP TABLE transactions;
ALTER TABLE transactions_new RENAME TO transactions;
CREATE INDEX idx_txn_account_status ON transactions(account_id, status);

-- +goose Down
-- Document reversal strategy
```

### 7. Use StatementBegin/StatementEnd for Complex SQL

For multi-statement migrations (like triggers or stored procedures):

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TRIGGER update_timestamp 
AFTER UPDATE ON transactions
BEGIN
    UPDATE transactions SET updated_at = datetime('now') 
    WHERE id = NEW.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS update_timestamp;
```

## Migration Status

Check current migration version:

```bash
# View goose_db_version table
sqlite3 ./data/ynab.db "SELECT * FROM goose_db_version"

# Using goose CLI
goose -dir internal/sqlite/migrations sqlite3 ./data/ynab.db status
```

The `goose_db_version` table tracks:
- `version_id`: Current migration version
- `is_applied`: Whether migration succeeded
- `tstamp`: When migration was applied

## Manual Migration Operations

### Using goose CLI

```bash
# Set your database path
export DB_PATH="./data/ynab.db"
export MIGRATIONS_DIR="internal/sqlite/migrations"

# Check status
goose -dir $MIGRATIONS_DIR sqlite3 $DB_PATH status

# Apply all pending migrations
goose -dir $MIGRATIONS_DIR sqlite3 $DB_PATH up

# Rollback last migration
goose -dir $MIGRATIONS_DIR sqlite3 $DB_PATH down

# Migrate to specific version
goose -dir $MIGRATIONS_DIR sqlite3 $DB_PATH up-to 2

# Reset database (down to version 0)
goose -dir $MIGRATIONS_DIR sqlite3 $DB_PATH reset

# Redo last migration (down then up)
goose -dir $MIGRATIONS_DIR sqlite3 $DB_PATH redo
```

## Troubleshooting

### Migration fails on startup

1. Check error message in application logs
2. Verify migration SQL syntax:
   ```bash
   sqlite3 ./data/ynab.db < internal/sqlite/migrations/00001_initial_schema.sql
   ```
3. Test manually with goose CLI

### Need to fix a failed migration

1. Backup database first:
   ```bash
   cp ./data/ynab.db ./data/ynab.db.backup
   ```

2. Check current state:
   ```bash
   goose -dir internal/sqlite/migrations sqlite3 ./data/ynab.db status
   ```

3. Fix the issue manually or rollback:
   ```bash
   goose -dir internal/sqlite/migrations sqlite3 ./data/ynab.db down
   ```

## Production Deployment

When deploying to production:

1. **Backup first**:
   ```bash
   cp ./data/ynab.db ./data/ynab.db.backup.$(date +%Y%m%d_%H%M%S)
   ```

2. **Review migrations**: Ensure all new migrations have been tested

3. **Deploy**: Migrations run automatically on application start

4. **Verify**: Check logs for successful migration messages

5. **Rollback plan**: Know how to rollback if needed

## Examples

### Example 1: Adding an Index

**00002_add_payee_name_index.sql:**
```sql
-- +goose Up
CREATE INDEX idx_payees_name ON payees(name);

-- +goose Down
DROP INDEX IF EXISTS idx_payees_name;
```

### Example 2: Adding a Table

**00003_add_user_settings.sql:**
```sql
-- +goose Up
CREATE TABLE user_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT NOT NULL UNIQUE,
    value TEXT NOT NULL,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE INDEX idx_user_settings_key ON user_settings(key);

-- +goose Down
DROP INDEX IF EXISTS idx_user_settings_key;
DROP TABLE IF EXISTS user_settings;
```

### Example 3: Data Migration

**00004_normalize_currency_codes.sql:**
```sql
-- +goose Up
UPDATE transactions SET currency = 'USD' WHERE currency = 'usd';
UPDATE transactions SET currency = 'EUR' WHERE currency = 'eur';
UPDATE transactions SET currency = 'GBP' WHERE currency = 'gbp';

-- +goose Down
-- This migration cannot be easily reversed
-- Data has been permanently transformed
```

### Example 4: Complex Table Modification

**00005_add_not_null_to_payee.sql:**
```sql
-- +goose Up
-- SQLite doesn't support adding NOT NULL to existing column
-- Must recreate table

CREATE TABLE transactions_new (
    id TEXT PRIMARY KEY,
    amount REAL NOT NULL,
    currency TEXT NOT NULL,
    description TEXT NOT NULL,
    payee TEXT NOT NULL,  -- Now NOT NULL
    account_id TEXT NOT NULL,
    account_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('DRAFT','SKIPPED','PROCESSED','INVALID')),
    created_at TEXT DEFAULT (datetime('now')),
    txn_time TEXT NOT NULL,
    raw_text TEXT,
    raw_line_number INTEGER,
    error_msg TEXT
);

-- Copy data (will fail if payee is NULL)
INSERT INTO transactions_new 
SELECT id, amount, currency, description, payee, account_id, account_name,
       status, created_at, txn_time, raw_text, raw_line_number, error_msg
FROM transactions;

-- Replace table
DROP TABLE transactions;
ALTER TABLE transactions_new RENAME TO transactions;

-- Recreate indexes
CREATE INDEX idx_txn_account_status ON transactions(account_id, status);
CREATE INDEX idx_txn_time ON transactions(txn_time);

-- +goose Down
-- Reverting requires recreating without NOT NULL constraint
CREATE TABLE transactions_old (
    id TEXT PRIMARY KEY,
    amount REAL NOT NULL,
    currency TEXT NOT NULL,
    description TEXT NOT NULL,
    payee TEXT,  -- Back to nullable
    account_id TEXT NOT NULL,
    account_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('DRAFT','SKIPPED','PROCESSED','INVALID')),
    created_at TEXT DEFAULT (datetime('now')),
    txn_time TEXT NOT NULL,
    raw_text TEXT,
    raw_line_number INTEGER,
    error_msg TEXT
);

INSERT INTO transactions_old SELECT * FROM transactions;
DROP TABLE transactions;
ALTER TABLE transactions_old RENAME TO transactions;
CREATE INDEX idx_txn_account_status ON transactions(account_id, status);
CREATE INDEX idx_txn_time ON transactions(txn_time);
```

## Goose Features

### Versioned vs Timestamped Migrations

Goose supports both:
- **Versioned** (our choice): `00001_name.sql` - sequential numbers
- **Timestamped**: `20060102150405_name.sql` - timestamp prefix

We use versioned for simplicity and predictability.

### Go Migrations

Goose also supports Go-based migrations for complex logic:

```go
package migrations

import (
    "database/sql"
    "github.com/pressly/goose/v3"
)

func init() {
    goose.AddMigration(upComplexLogic, downComplexLogic)
}

func upComplexLogic(tx *sql.Tx) error {
    // Complex Go logic here
    return nil
}

func downComplexLogic(tx *sql.Tx) error {
    return nil
}
```

For this project, we use SQL-only migrations for simplicity.

## References

- [Goose documentation](https://github.com/pressly/goose)
- [SQLite ALTER TABLE limitations](https://www.sqlite.org/lang_altertable.html)
- [Database migration best practices](https://www.prisma.io/dataguide/types/relational/what-are-database-migrations)
