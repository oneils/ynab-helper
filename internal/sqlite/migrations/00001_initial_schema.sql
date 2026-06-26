-- +goose Up
-- +goose StatementBegin
CREATE TABLE transactions (
    id TEXT PRIMARY KEY,
    amount REAL NOT NULL,
    currency TEXT NOT NULL,
    description TEXT NOT NULL,
    payee TEXT,
    account_id TEXT NOT NULL,
    account_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('DRAFT','SKIPPED','PROCESSED','INVALID')),
    created_at TEXT DEFAULT (datetime('now')),
    txn_time TEXT NOT NULL,
    raw_text TEXT,
    raw_line_number INTEGER,
    error_msg TEXT
);

CREATE INDEX idx_txn_account_status ON transactions(account_id, status);

CREATE INDEX idx_txn_time ON transactions(txn_time);

CREATE TABLE budgets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    last_modified_on TEXT,
    first_month TEXT,
    last_month TEXT,
    date_format TEXT,
    currency_format TEXT
);

CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    budget_id TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT,
    on_budget INTEGER DEFAULT 0,
    closed INTEGER DEFAULT 0,
    note TEXT,
    balance INTEGER DEFAULT 0,
    cleared_balance INTEGER DEFAULT 0,
    uncleared_balance INTEGER DEFAULT 0,
    direct_import_linked INTEGER DEFAULT 0,
    direct_import_in_error INTEGER DEFAULT 0,
    last_reconciled_at TEXT,
    debt_original_balance INTEGER DEFAULT 0,
    transfer_payee_id TEXT,
    deleted INTEGER DEFAULT 0,
    FOREIGN KEY (budget_id) REFERENCES budgets(id) ON DELETE CASCADE
);

CREATE INDEX idx_accounts_budget ON accounts(budget_id);

CREATE TABLE category_groups (
    id TEXT PRIMARY KEY,
    budget_id TEXT NOT NULL,
    name TEXT NOT NULL,
    hidden INTEGER DEFAULT 0,
    deleted INTEGER DEFAULT 0,
    FOREIGN KEY (budget_id) REFERENCES budgets(id) ON DELETE CASCADE
);

CREATE INDEX idx_category_groups_budget ON category_groups(budget_id);

CREATE TABLE categories (
    id TEXT PRIMARY KEY,
    budget_id TEXT NOT NULL,
    category_group_id TEXT NOT NULL,
    category_group_name TEXT NOT NULL,
    name TEXT NOT NULL,
    hidden INTEGER DEFAULT 0,
    deleted INTEGER DEFAULT 0,
    original_category_group_id TEXT,
    note TEXT,
    budgeted INTEGER DEFAULT 0,
    activity INTEGER DEFAULT 0,
    balance INTEGER DEFAULT 0,
    goal_type TEXT,
    goal_days INTEGER,
    goal_cadence INTEGER,
    goal_cadence_frequency INTEGER,
    goal_target INTEGER,
    goal_target_month TEXT,
    goal_creation_month TEXT,
    goal_percentage_complete INTEGER,
    goal_months_to_budget INTEGER,
    goal_under_funded INTEGER,
    goal_overall_funded INTEGER,
    goal_overall_left INTEGER,
    FOREIGN KEY (budget_id) REFERENCES budgets(id) ON DELETE CASCADE,
    FOREIGN KEY (category_group_id) REFERENCES category_groups(id) ON DELETE CASCADE
);

CREATE INDEX idx_categories_budget_deleted ON categories(budget_id, deleted);

CREATE INDEX idx_categories_group ON categories(category_group_id);

CREATE TABLE payees (
    id TEXT PRIMARY KEY,
    budget_id TEXT NOT NULL,
    name TEXT NOT NULL,
    transfer_account_id TEXT,
    deleted INTEGER DEFAULT 0,
    last_category_id TEXT,
    FOREIGN KEY (budget_id) REFERENCES budgets(id) ON DELETE CASCADE
);

CREATE INDEX idx_payees_budget_deleted ON payees(budget_id, deleted);

CREATE TABLE sync_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    budget_id TEXT,
    status TEXT NOT NULL,
    updated_at TEXT DEFAULT (datetime('now')),
    last_known_version INTEGER,
    added_items INTEGER,
    message TEXT
);

CREATE INDEX idx_sync_history_budget ON sync_history(budget_id);

CREATE UNIQUE INDEX idx_sync_history_name_budget ON sync_history(name, COALESCE(budget_id, ''));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_sync_history_name_budget;

DROP INDEX IF EXISTS idx_sync_history_budget;

DROP TABLE IF EXISTS sync_history;

DROP INDEX IF EXISTS idx_payees_budget_deleted;

DROP TABLE IF EXISTS payees;

DROP INDEX IF EXISTS idx_categories_group;

DROP INDEX IF EXISTS idx_categories_budget_deleted;

DROP TABLE IF EXISTS categories;

DROP INDEX IF EXISTS idx_category_groups_budget;

DROP TABLE IF EXISTS category_groups;

DROP INDEX IF EXISTS idx_accounts_budget;

DROP TABLE IF EXISTS accounts;

DROP TABLE IF EXISTS budgets;

DROP INDEX IF EXISTS idx_txn_time;

DROP INDEX IF EXISTS idx_txn_account_status;

DROP TABLE IF EXISTS transactions;
-- +goose StatementEnd
