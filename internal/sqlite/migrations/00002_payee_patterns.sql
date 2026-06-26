-- +goose Up
-- +goose StatementBegin
CREATE TABLE payee_patterns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    budget_id TEXT NOT NULL,
    normalized_description TEXT NOT NULL,
    payee_id TEXT NOT NULL,
    payee_name TEXT NOT NULL,
    category_id TEXT,
    category_name TEXT,
    occurrence_count INTEGER DEFAULT 1,
    last_seen TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (budget_id) REFERENCES budgets(id) ON DELETE CASCADE,
    FOREIGN KEY (payee_id) REFERENCES payees(id) ON DELETE CASCADE
);

CREATE INDEX idx_payee_patterns_budget ON payee_patterns(budget_id);
CREATE INDEX idx_payee_patterns_desc ON payee_patterns(normalized_description);
CREATE INDEX idx_payee_patterns_payee ON payee_patterns(payee_id);
CREATE UNIQUE INDEX idx_payee_patterns_unique ON payee_patterns(
    budget_id, normalized_description, payee_id, COALESCE(category_id, '')
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_payee_patterns_unique;
DROP INDEX IF EXISTS idx_payee_patterns_payee;
DROP INDEX IF EXISTS idx_payee_patterns_desc;
DROP INDEX IF EXISTS idx_payee_patterns_budget;
DROP TABLE IF EXISTS payee_patterns;
-- +goose StatementEnd
