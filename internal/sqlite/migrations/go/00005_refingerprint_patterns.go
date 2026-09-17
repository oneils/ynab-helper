// Package migrations contains Go-coded goose migrations that need Go logic
// (as opposed to the SQL-only migrations embedded from ../*.sql). Files here
// are NOT picked up by the //go:embed migrations/*.sql directive in
// sqlite.go; they register themselves via goose.AddMigrationContext's init()
// side effect, which only runs if something imports this package. sqlite.go
// does so with a blank import.
package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/oneils/ynab-helper/internal/txn"
)

func init() {
	goose.AddMigrationContext(upRefingerprintPatterns, downRefingerprintPatterns)
}

type patternRow struct {
	id              int64
	budgetID        string
	normalizedDesc  string
	payeeID         string
	categoryID      sql.NullString
	occurrenceCount int
	lastSeen        string
}

type groupKey struct {
	budgetID    string
	fingerprint string
	payeeID     string
	categoryID  string
}

func upRefingerprintPatterns(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, budget_id, normalized_description, payee_id, category_id,
		       occurrence_count, last_seen
		FROM payee_patterns
	`)
	if err != nil {
		return fmt.Errorf("select payee_patterns: %w", err)
	}

	var all []patternRow
	for rows.Next() {
		var r patternRow
		if err := rows.Scan(&r.id, &r.budgetID, &r.normalizedDesc, &r.payeeID,
			&r.categoryID, &r.occurrenceCount, &r.lastSeen); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan payee_patterns row: %w", err)
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate payee_patterns rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close payee_patterns rows: %w", err)
	}

	groups := make(map[groupKey][]patternRow)
	var order []groupKey
	for _, r := range all {
		newFP := txn.Fingerprint(r.normalizedDesc)
		key := groupKey{
			budgetID:    r.budgetID,
			fingerprint: newFP,
			payeeID:     r.payeeID,
			categoryID:  r.categoryID.String,
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r)
	}

	for _, key := range order {
		members := groups[key]

		survivor := members[0]
		totalCount := 0
		maxLastSeen := survivor.lastSeen
		maxLastSeenParsed, _ := time.Parse(time.RFC3339, maxLastSeen)
		for _, m := range members {
			totalCount += m.occurrenceCount
			if mParsed, err := time.Parse(time.RFC3339, m.lastSeen); err == nil && mParsed.After(maxLastSeenParsed) {
				maxLastSeen = m.lastSeen
				maxLastSeenParsed = mParsed
			}
			if m.id < survivor.id {
				survivor = m
			}
		}

		for _, m := range members {
			if m.id == survivor.id {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				"DELETE FROM payee_patterns WHERE id = ?", m.id); err != nil {
				return fmt.Errorf("delete duplicate pattern %d: %w", m.id, err)
			}
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE payee_patterns
			SET normalized_description = ?, occurrence_count = ?, last_seen = ?
			WHERE id = ?
		`, key.fingerprint, totalCount, maxLastSeen, survivor.id); err != nil {
			return fmt.Errorf("update pattern %d to new fingerprint: %w", survivor.id, err)
		}
	}

	return nil
}

// downRefingerprintPatterns is an intentional no-op: re-fingerprinting is
// lossy (card numbers, dates, and reference IDs stripped from
// normalized_description are discarded, and colliding rows are merged), so
// the original per-card-number rows cannot be reconstructed on rollback.
func downRefingerprintPatterns(ctx context.Context, tx *sql.Tx) error {
	return nil
}
