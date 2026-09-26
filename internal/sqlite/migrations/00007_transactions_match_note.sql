-- +goose Up
ALTER TABLE transactions ADD COLUMN match_note TEXT;

-- +goose Down
-- Note: SQLite doesn't support DROP COLUMN directly
