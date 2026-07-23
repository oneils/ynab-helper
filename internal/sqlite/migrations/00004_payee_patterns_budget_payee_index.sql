-- +goose Up
-- +goose StatementBegin
CREATE INDEX idx_payee_patterns_budget_payee ON payee_patterns(budget_id, payee_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_payee_patterns_budget_payee;
-- +goose StatementEnd
