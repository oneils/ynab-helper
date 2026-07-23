package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

// PatternStore handles payee pattern persistence.
type PatternStore struct {
	db *sql.DB
}

// NewPatternStore creates a new PatternStore.
func NewPatternStore(db *sql.DB) *PatternStore {
	return &PatternStore{db: db}
}

// UpsertPattern inserts or updates a payee pattern.
func (s *PatternStore) UpsertPattern(ctx context.Context, p txn.PayeePattern) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Check if pattern exists
	var existingID int64
	var existingCount int
	query := `
		SELECT id, occurrence_count
		FROM payee_patterns
		WHERE budget_id = ? AND normalized_description = ?
		  AND payee_id = ? AND COALESCE(category_id, '') = COALESCE(?, '')
	`
	err := s.db.QueryRowContext(ctx, query,
		p.BudgetID, p.NormalizedDescription, p.PayeeID, p.CategoryID).
		Scan(&existingID, &existingCount)

	if err == sql.ErrNoRows {
		// Insert new pattern
		insertQuery := `
			INSERT INTO payee_patterns (
				budget_id, normalized_description, payee_id, payee_name,
				category_id, category_name, occurrence_count, last_seen, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		_, err := s.db.ExecContext(ctx, insertQuery,
			p.BudgetID, p.NormalizedDescription, p.PayeeID, p.PayeeName,
			p.CategoryID, p.CategoryName, 1,
			p.LastSeen.Format(time.RFC3339), time.Now().Format(time.RFC3339))

		if err != nil {
			return fmt.Errorf("insert pattern: %w", err)
		}
		slog.Debug("inserted new pattern", "desc", p.NormalizedDescription, "payee", p.PayeeName)
		return nil
	}

	if err != nil {
		return fmt.Errorf("check pattern existence: %w", err)
	}

	// Update existing pattern
	updateQuery := `
		UPDATE payee_patterns
		SET occurrence_count = ?, last_seen = ?, updated_at = ?
		WHERE id = ?
	`
	_, err = s.db.ExecContext(ctx, updateQuery,
		existingCount+1, p.LastSeen.Format(time.RFC3339),
		time.Now().Format(time.RFC3339), existingID)

	if err != nil {
		return fmt.Errorf("update pattern: %w", err)
	}

	slog.Debug("updated pattern", "desc", p.NormalizedDescription,
		"count", existingCount+1)
	return nil
}

// FindPatternsByDescription searches for matching patterns.
func (s *PatternStore) FindPatternsByDescription(ctx context.Context,
	budgetID, normalizedDesc string, limit int) ([]txn.PayeePattern, error) {

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `
		SELECT id, budget_id, normalized_description,
		       payee_id, payee_name, category_id, category_name,
		       occurrence_count, last_seen, created_at, updated_at
		FROM payee_patterns
		WHERE budget_id = ? AND normalized_description LIKE ?
		ORDER BY occurrence_count DESC, last_seen DESC
		LIMIT ?
	`

	likePattern := "%" + normalizedDesc + "%"

	rows, err := s.db.QueryContext(ctx, query, budgetID, likePattern, limit)
	if err != nil {
		return nil, fmt.Errorf("query patterns: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = fmt.Errorf("close rows: %w", closeErr)
		}
	}()

	var patterns []txn.PayeePattern
	for rows.Next() {
		var p txn.PayeePattern
		var lastSeen, createdAt, updatedAt string
		var categoryID, categoryName sql.NullString

		err := rows.Scan(
			&p.ID, &p.BudgetID, &p.NormalizedDescription,
			&p.PayeeID, &p.PayeeName, &categoryID, &categoryName,
			&p.OccurrenceCount, &lastSeen, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan pattern: %w", err)
		}

		if categoryID.Valid {
			p.CategoryID = categoryID.String
		}
		if categoryName.Valid {
			p.CategoryName = categoryName.String
		}

		p.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		patterns = append(patterns, p)
	}

	return patterns, rows.Err()
}

// FindPatternsByPayeeID searches for matching patterns by exact payee ID.
func (s *PatternStore) FindPatternsByPayeeID(ctx context.Context,
	budgetID, payeeID string, limit int) (patterns []txn.PayeePattern, err error) {

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `
		SELECT id, budget_id, normalized_description,
		       payee_id, payee_name, category_id, category_name,
		       occurrence_count, last_seen, created_at, updated_at
		FROM payee_patterns
		WHERE budget_id = ? AND payee_id = ?
		  AND category_id IS NOT NULL AND category_id != ''
		ORDER BY occurrence_count DESC, last_seen DESC
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, budgetID, payeeID, limit)
	if err != nil {
		return nil, fmt.Errorf("query patterns: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", closeErr)
		}
	}()

	for rows.Next() {
		var p txn.PayeePattern
		var lastSeen, createdAt, updatedAt string
		var categoryID, categoryName sql.NullString

		err := rows.Scan(
			&p.ID, &p.BudgetID, &p.NormalizedDescription,
			&p.PayeeID, &p.PayeeName, &categoryID, &categoryName,
			&p.OccurrenceCount, &lastSeen, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan pattern: %w", err)
		}

		if categoryID.Valid {
			p.CategoryID = categoryID.String
		}
		if categoryName.Valid {
			p.CategoryName = categoryName.String
		}

		p.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		patterns = append(patterns, p)
	}

	return patterns, rows.Err()
}

// ClearPatterns removes all patterns for a budget (useful for re-sync).
func (s *PatternStore) ClearPatterns(ctx context.Context, budgetID string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx,
		"DELETE FROM payee_patterns WHERE budget_id = ?", budgetID)
	return err
}
