package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupPatternTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}

	schema := `
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
	    updated_at TEXT DEFAULT (datetime('now'))
	);
	CREATE INDEX idx_payee_patterns_budget_payee ON payee_patterns(budget_id, payee_id);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db
}

func insertTestPattern(t *testing.T, db *sql.DB, budgetID, payeeID string, categoryID, categoryName sql.NullString, occurrenceCount int, lastSeen time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
		INSERT INTO payee_patterns (budget_id, normalized_description, payee_id, payee_name, category_id, category_name, occurrence_count, last_seen, created_at, updated_at)
		VALUES (?, 'test desc', ?, 'Test Payee', ?, ?, ?, ?, ?, ?)
	`
	if _, err := db.Exec(query, budgetID, payeeID, categoryID, categoryName, occurrenceCount, lastSeen.UTC().Format(time.RFC3339), now, now); err != nil {
		t.Fatalf("insert test pattern: %v", err)
	}
}

func TestFindPatternsByPayeeID_SortedByOccurrenceCount(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	insertTestPattern(t, db, "budget1", "payee1", sql.NullString{String: "cat1", Valid: true}, sql.NullString{String: "Cat 1", Valid: true}, 5, now)
	insertTestPattern(t, db, "budget1", "payee1", sql.NullString{String: "cat2", Valid: true}, sql.NullString{String: "Cat 2", Valid: true}, 10, now)
	insertTestPattern(t, db, "budget1", "payee1", sql.NullString{String: "cat3", Valid: true}, sql.NullString{String: "Cat 3", Valid: true}, 1, now)

	patterns, err := store.FindPatternsByPayeeID(context.Background(), "budget1", "payee1", 50)
	if err != nil {
		t.Fatalf("FindPatternsByPayeeID: %v", err)
	}

	if len(patterns) != 3 {
		t.Fatalf("expected 3 patterns, got %d", len(patterns))
	}

	if patterns[0].CategoryID != "cat2" || patterns[1].CategoryID != "cat1" || patterns[2].CategoryID != "cat3" {
		t.Errorf("patterns not sorted by occurrence_count DESC: %+v", patterns)
	}
}

func TestFindPatternsByPayeeID_ExcludesNullCategory(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	insertTestPattern(t, db, "budget1", "payee1", sql.NullString{String: "cat1", Valid: true}, sql.NullString{String: "Cat 1", Valid: true}, 1, now)
	insertTestPattern(t, db, "budget1", "payee1", sql.NullString{}, sql.NullString{}, 5, now)
	insertTestPattern(t, db, "budget1", "payee1", sql.NullString{String: "", Valid: true}, sql.NullString{String: "", Valid: true}, 5, now)

	patterns, err := store.FindPatternsByPayeeID(context.Background(), "budget1", "payee1", 50)
	if err != nil {
		t.Fatalf("FindPatternsByPayeeID: %v", err)
	}

	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].CategoryID != "cat1" {
		t.Errorf("expected cat1, got %s", patterns[0].CategoryID)
	}
}

func TestFindPatternsByPayeeID_IsolatedByBudget(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	insertTestPattern(t, db, "budget1", "payee1", sql.NullString{String: "cat1", Valid: true}, sql.NullString{String: "Cat 1", Valid: true}, 1, now)
	insertTestPattern(t, db, "budget2", "payee1", sql.NullString{String: "cat2", Valid: true}, sql.NullString{String: "Cat 2", Valid: true}, 1, now)

	patterns, err := store.FindPatternsByPayeeID(context.Background(), "budget1", "payee1", 50)
	if err != nil {
		t.Fatalf("FindPatternsByPayeeID: %v", err)
	}
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern from budget1 only, got %d", len(patterns))
	}
	if patterns[0].CategoryID != "cat1" {
		t.Errorf("expected cat1 from budget1, got %s", patterns[0].CategoryID)
	}
}

func TestFindPatternsByPayeeID_UnknownPayeeReturnsEmpty(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	insertTestPattern(t, db, "budget1", "payee1", sql.NullString{String: "cat1", Valid: true}, sql.NullString{String: "Cat 1", Valid: true}, 1, time.Now())

	patterns, err := store.FindPatternsByPayeeID(context.Background(), "budget1", "unknown-payee", 50)
	if err != nil {
		t.Fatalf("FindPatternsByPayeeID: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(patterns))
	}
}

func TestFindPatternsByPayeeID_RespectsLimit(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	for i := 0; i < 5; i++ {
		insertTestPattern(t, db, "budget1", "payee1", sql.NullString{String: "cat1", Valid: true}, sql.NullString{String: "Cat 1", Valid: true}, i+1, now)
	}

	patterns, err := store.FindPatternsByPayeeID(context.Background(), "budget1", "payee1", 2)
	if err != nil {
		t.Fatalf("FindPatternsByPayeeID: %v", err)
	}
	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(patterns))
	}
}
