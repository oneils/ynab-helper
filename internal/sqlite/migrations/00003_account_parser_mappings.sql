-- +goose Up
-- +goose StatementBegin
CREATE TABLE account_parser_mappings (
    account_id TEXT PRIMARY KEY,
    parser_name TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS account_parser_mappings;
-- +goose StatementEnd
