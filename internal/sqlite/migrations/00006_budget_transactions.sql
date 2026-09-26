-- +goose Up
-- +goose StatementBegin
CREATE TABLE budget_transactions (
    id TEXT PRIMARY KEY,
    budget_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    account_name TEXT,
    date TEXT NOT NULL,
    amount INTEGER NOT NULL,
    memo TEXT,
    cleared TEXT,
    approved INTEGER DEFAULT 0,
    flag_color TEXT,
    payee_id TEXT,
    payee_name TEXT,
    category_id TEXT,
    category_name TEXT,
    transfer_account_id TEXT,
    transfer_transaction_id TEXT,
    matched_transaction_id TEXT,
    import_id TEXT,
    deleted INTEGER DEFAULT 0,
    FOREIGN KEY (budget_id) REFERENCES budgets(id) ON DELETE CASCADE
);

CREATE INDEX idx_budget_txns_account_date ON budget_transactions(account_id, date);

CREATE TABLE budget_subtransactions (
    id TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL,
    amount INTEGER NOT NULL,
    memo TEXT,
    payee_id TEXT,
    payee_name TEXT,
    category_id TEXT,
    category_name TEXT,
    transfer_account_id TEXT,
    deleted INTEGER DEFAULT 0,
    FOREIGN KEY (transaction_id) REFERENCES budget_transactions(id) ON DELETE CASCADE
);

CREATE INDEX idx_budget_subtxns_transaction ON budget_subtransactions(transaction_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_budget_subtxns_transaction;

DROP TABLE IF EXISTS budget_subtransactions;

DROP INDEX IF EXISTS idx_budget_txns_account_date;

DROP TABLE IF EXISTS budget_transactions;
-- +goose StatementEnd
