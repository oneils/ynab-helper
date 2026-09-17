package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

// candidateSearchStopwords are bank/payment boilerplate tokens excluded when
// broadening a fingerprint search to significant tokens (English + Polish).
var candidateSearchStopwords = map[string]bool{
	"payment":     true,
	"card":        true,
	"pos":         true,
	"purchase":    true,
	"transaction": true,
	"platnosc":    true,
	"karta":       true,
	"transakcja":  true,
	"przelew":     true,
	"blik":        true,
}

// significantTokens returns up to n of the longest tokens from desc that are
// not stopwords and are at least 3 characters long.
func significantTokens(desc string, n int) []string {
	words := strings.Fields(desc)
	tokens := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) < 3 || candidateSearchStopwords[w] {
			continue
		}
		tokens = append(tokens, w)
	}

	sort.SliceStable(tokens, func(i, j int) bool {
		return len(tokens[i]) > len(tokens[j])
	})

	if len(tokens) > n {
		tokens = tokens[:n]
	}
	return tokens
}

// PatternStore handles payee pattern persistence.
type PatternStore struct {
	db *sql.DB
}

// NewPatternStore creates a new PatternStore.
func NewPatternStore(db *sql.DB) *PatternStore {
	return &PatternStore{db: db}
}

// UpsertPattern inserts or updates a payee pattern. If a row already exists
// for the same budget+description+payee but with a *different* category
// (the user corrected a suggestion), that stale row is deleted first so it
// can't keep surfacing via its old occurrence_count — the new category
// starts fresh at occurrence_count 1.
func (s *PatternStore) UpsertPattern(ctx context.Context, p txn.PayeePattern) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM payee_patterns
		WHERE budget_id = ? AND normalized_description = ? AND payee_id = ?
		  AND COALESCE(category_id, '') != COALESCE(?, '')
	`, p.BudgetID, p.NormalizedDescription, p.PayeeID, p.CategoryID); err != nil {
		return fmt.Errorf("delete conflicting pattern: %w", err)
	}

	// Check if pattern exists
	var existingID int64
	var existingCount int
	var existingLastSeen string
	query := `
		SELECT id, occurrence_count, last_seen
		FROM payee_patterns
		WHERE budget_id = ? AND normalized_description = ?
		  AND payee_id = ? AND COALESCE(category_id, '') = COALESCE(?, '')
	`
	err = tx.QueryRowContext(ctx, query,
		p.BudgetID, p.NormalizedDescription, p.PayeeID, p.CategoryID).
		Scan(&existingID, &existingCount, &existingLastSeen)

	if err == sql.ErrNoRows {
		// Insert new pattern
		insertQuery := `
			INSERT INTO payee_patterns (
				budget_id, normalized_description, payee_id, payee_name,
				category_id, category_name, occurrence_count, last_seen, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		if _, err := tx.ExecContext(ctx, insertQuery,
			p.BudgetID, p.NormalizedDescription, p.PayeeID, p.PayeeName,
			p.CategoryID, p.CategoryName, 1,
			p.LastSeen.Format(time.RFC3339), time.Now().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("insert pattern: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit transaction: %w", err)
		}
		slog.Debug("inserted new pattern", "desc", p.NormalizedDescription, "payee", p.PayeeName)
		return nil
	}

	if err != nil {
		return fmt.Errorf("check pattern existence: %w", err)
	}

	// Update existing pattern, keeping last_seen monotonic (never regress it
	// on a re-import of an older transaction).
	newLastSeen := p.LastSeen.Format(time.RFC3339)
	existingParsed, parseErr := time.Parse(time.RFC3339, existingLastSeen)
	if parseErr == nil && existingParsed.After(p.LastSeen) {
		newLastSeen = existingLastSeen
	}

	updateQuery := `
		UPDATE payee_patterns
		SET occurrence_count = ?, last_seen = ?, updated_at = ?
		WHERE id = ?
	`
	if _, err := tx.ExecContext(ctx, updateQuery,
		existingCount+1, newLastSeen,
		time.Now().Format(time.RFC3339), existingID); err != nil {
		return fmt.Errorf("update pattern: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	slog.Debug("updated pattern", "desc", p.NormalizedDescription,
		"count", existingCount+1)
	return nil
}

// FindPatternsByDescription searches for matching patterns. It first tries an
// exact fingerprint match; if that finds nothing, it falls back to a
// token-broadened search over the incoming fingerprint's significant tokens,
// leaving fine-grained scoring/filtering to the caller (tokenSimilarity +
// confidence threshold).
func (s *PatternStore) FindPatternsByDescription(ctx context.Context,
	budgetID, normalizedDesc string, limit int) ([]txn.PayeePattern, error) {

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	patterns, err := s.queryPatternsByDescription(ctx,
		"WHERE budget_id = ? AND normalized_description = ?",
		[]any{budgetID, normalizedDesc}, limit)
	if err != nil {
		return nil, err
	}
	if len(patterns) > 0 {
		return patterns, nil
	}

	tokens := significantTokens(normalizedDesc, 3)
	if len(tokens) == 0 {
		return nil, nil
	}

	conditions := make([]string, 0, len(tokens))
	args := make([]any, 0, len(tokens)+2)
	args = append(args, budgetID)
	for _, tok := range tokens {
		conditions = append(conditions, "normalized_description LIKE ?")
		args = append(args, "%"+tok+"%")
	}

	where := "WHERE budget_id = ? AND (" + strings.Join(conditions, " OR ") + ")"
	return s.queryPatternsByDescription(ctx, where, args, limit)
}

func (s *PatternStore) queryPatternsByDescription(ctx context.Context,
	where string, args []any, limit int) (patterns []txn.PayeePattern, err error) {

	query := `
		SELECT id, budget_id, normalized_description,
		       payee_id, payee_name, category_id, category_name,
		       occurrence_count, last_seen, created_at, updated_at
		FROM payee_patterns
		` + where + `
		ORDER BY occurrence_count DESC, last_seen DESC
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, append(args, limit)...)
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
