package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ParserMappingStore handles account-to-parser mapping persistence.
type ParserMappingStore struct {
	db *sql.DB
}

// NewParserMappingStore creates a new ParserMappingStore.
func NewParserMappingStore(db *sql.DB) *ParserMappingStore {
	return &ParserMappingStore{db: db}
}

// GetParserMapping returns the parser name mapped to an account.
// Both a missing row and a stored empty string are treated as "not mapped"
// and return ("", nil).
func (s *ParserMappingStore) GetParserMapping(ctx context.Context, accountID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var parserName string
	err := s.db.QueryRowContext(ctx,
		"SELECT parser_name FROM account_parser_mappings WHERE account_id = ?", accountID).
		Scan(&parserName)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get parser mapping: %w", err)
	}

	return parserName, nil
}

// SaveParserMapping upserts the parser mapping for an account. Passing an
// empty parserName clears the mapping (treated identically to a missing row).
func (s *ParserMappingStore) SaveParserMapping(ctx context.Context, accountID, parserName string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `
		INSERT INTO account_parser_mappings (account_id, parser_name, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET parser_name = excluded.parser_name, updated_at = excluded.updated_at
	`
	_, err := s.db.ExecContext(ctx, query, accountID, parserName, time.Now().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save parser mapping: %w", err)
	}

	return nil
}
