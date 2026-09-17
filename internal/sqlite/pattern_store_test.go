package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
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

func insertTestPatternWithDesc(t *testing.T, db *sql.DB, budgetID, normalizedDesc, payeeID string, occurrenceCount int, lastSeen time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
		INSERT INTO payee_patterns (budget_id, normalized_description, payee_id, payee_name, category_id, category_name, occurrence_count, last_seen, created_at, updated_at)
		VALUES (?, ?, ?, 'Test Payee', 'cat1', 'Cat 1', ?, ?, ?, ?)
	`
	if _, err := db.Exec(query, budgetID, normalizedDesc, payeeID, occurrenceCount, lastSeen.UTC().Format(time.RFC3339), now, now); err != nil {
		t.Fatalf("insert test pattern: %v", err)
	}
}

func TestFindPatternsByDescription_FallbackFindsTokenOverlapNotJustSubstring(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	insertTestPatternWithDesc(t, db, "budget1", "payment card lidl warszawa", "payee1", 3, now)

	patterns, err := store.FindPatternsByDescription(context.Background(), "budget1", "payment card lidl krakow", 50)
	if err != nil {
		t.Fatalf("FindPatternsByDescription: %v", err)
	}

	if len(patterns) != 1 {
		t.Fatalf("expected fallback token search to find 1 pattern, got %d: %+v", len(patterns), patterns)
	}
	if patterns[0].NormalizedDescription != "payment card lidl warszawa" {
		t.Errorf("unexpected pattern found: %+v", patterns[0])
	}
}

func TestFindPatternsByDescription_ExactMatchFastPathRanksFirst(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	insertTestPatternWithDesc(t, db, "budget1", "lidl warszawa", "payee1", 1, now)
	insertTestPatternWithDesc(t, db, "budget1", "lidl krakow", "payee2", 1, now)

	patterns, err := store.FindPatternsByDescription(context.Background(), "budget1", "lidl warszawa", 50)
	if err != nil {
		t.Fatalf("FindPatternsByDescription: %v", err)
	}

	if len(patterns) == 0 {
		t.Fatalf("expected at least one pattern")
	}
	if patterns[0].NormalizedDescription != "lidl warszawa" {
		t.Errorf("expected exact match to rank first, got %+v", patterns[0])
	}
}

func TestFindPatternsByDescription_NoCandidatesReturnsEmptyNoError(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	insertTestPatternWithDesc(t, db, "budget1", "lidl warszawa", "payee1", 1, now)

	patterns, err := store.FindPatternsByDescription(context.Background(), "budget1", "completely unrelated merchant", 50)
	if err != nil {
		t.Fatalf("FindPatternsByDescription: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns, got %d: %+v", len(patterns), patterns)
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

func queryPatternRow(t *testing.T, db *sql.DB, budgetID, normalizedDesc, payeeID string) (found bool, categoryID string, occurrenceCount int, lastSeen time.Time) {
	t.Helper()
	row := db.QueryRow(`
		SELECT COALESCE(category_id, ''), occurrence_count, last_seen
		FROM payee_patterns
		WHERE budget_id = ? AND normalized_description = ? AND payee_id = ?
	`, budgetID, normalizedDesc, payeeID)

	var lastSeenStr string
	err := row.Scan(&categoryID, &occurrenceCount, &lastSeenStr)
	if err == sql.ErrNoRows {
		return false, "", 0, time.Time{}
	}
	if err != nil {
		t.Fatalf("query pattern row: %v", err)
	}
	lastSeen, _ = time.Parse(time.RFC3339, lastSeenStr)
	return true, categoryID, occurrenceCount, lastSeen
}

func countPatternRows(t *testing.T, db *sql.DB, budgetID, normalizedDesc, payeeID string) int {
	t.Helper()
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM payee_patterns
		WHERE budget_id = ? AND normalized_description = ? AND payee_id = ?
	`, budgetID, normalizedDesc, payeeID).Scan(&count)
	if err != nil {
		t.Fatalf("count pattern rows: %v", err)
	}
	return count
}

func TestUpsertPattern_CategoryCorrectionDeletesOldConflictingRow(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	insertTestPatternWithDesc(t, db, "budget1", "lidl warszawa", "payee1", 5, now)

	err := store.UpsertPattern(context.Background(), txn.PayeePattern{
		BudgetID:              "budget1",
		NormalizedDescription: "lidl warszawa",
		PayeeID:               "payee1",
		PayeeName:             "Test Payee",
		CategoryID:            "cat-new",
		CategoryName:          "New Category",
		LastSeen:              now,
	})
	if err != nil {
		t.Fatalf("UpsertPattern: %v", err)
	}

	count := countPatternRows(t, db, "budget1", "lidl warszawa", "payee1")
	if count != 1 {
		t.Fatalf("expected exactly 1 surviving row after correction, got %d", count)
	}

	found, categoryID, _, _ := queryPatternRow(t, db, "budget1", "lidl warszawa", "payee1")
	if !found {
		t.Fatalf("expected a row to exist")
	}
	if categoryID != "cat-new" {
		t.Errorf("expected surviving row's category to be cat-new, got %q", categoryID)
	}
}

func TestUpsertPattern_SameCategoryIncrementsOccurrenceCount(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	insertTestPatternWithDesc(t, db, "budget1", "lidl warszawa", "payee1", 3, now)

	err := store.UpsertPattern(context.Background(), txn.PayeePattern{
		BudgetID:              "budget1",
		NormalizedDescription: "lidl warszawa",
		PayeeID:               "payee1",
		PayeeName:             "Test Payee",
		CategoryID:            "cat1",
		CategoryName:          "Cat 1",
		LastSeen:              now,
	})
	if err != nil {
		t.Fatalf("UpsertPattern: %v", err)
	}

	_, categoryID, occurrenceCount, _ := queryPatternRow(t, db, "budget1", "lidl warszawa", "payee1")
	if categoryID != "cat1" {
		t.Errorf("expected category to remain cat1, got %q", categoryID)
	}
	if occurrenceCount != 4 {
		t.Errorf("expected occurrence_count to increment to 4, got %d", occurrenceCount)
	}
}

func TestUpsertPattern_CorrectionResetsOccurrenceCountToOne(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	insertTestPatternWithDesc(t, db, "budget1", "lidl warszawa", "payee1", 10, now)

	err := store.UpsertPattern(context.Background(), txn.PayeePattern{
		BudgetID:              "budget1",
		NormalizedDescription: "lidl warszawa",
		PayeeID:               "payee1",
		PayeeName:             "Test Payee",
		CategoryID:            "cat-new",
		CategoryName:          "New Category",
		LastSeen:              now,
	})
	if err != nil {
		t.Fatalf("UpsertPattern: %v", err)
	}

	_, categoryID, occurrenceCount, _ := queryPatternRow(t, db, "budget1", "lidl warszawa", "payee1")
	if categoryID != "cat-new" {
		t.Fatalf("expected category to be cat-new, got %q", categoryID)
	}
	if occurrenceCount != 1 {
		t.Errorf("expected occurrence_count to reset to 1 after correction, got %d", occurrenceCount)
	}
}

func TestUpsertPattern_LastSeenIsMonotonic(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	newer := time.Now().UTC()
	older := newer.Add(-48 * time.Hour)

	insertTestPatternWithDesc(t, db, "budget1", "lidl warszawa", "payee1", 1, newer)

	err := store.UpsertPattern(context.Background(), txn.PayeePattern{
		BudgetID:              "budget1",
		NormalizedDescription: "lidl warszawa",
		PayeeID:               "payee1",
		PayeeName:             "Test Payee",
		CategoryID:            "cat1",
		CategoryName:          "Cat 1",
		LastSeen:              older,
	})
	if err != nil {
		t.Fatalf("UpsertPattern: %v", err)
	}

	_, _, _, lastSeen := queryPatternRow(t, db, "budget1", "lidl warszawa", "payee1")
	if !lastSeen.Equal(newer.Truncate(time.Second)) {
		t.Errorf("expected last_seen to remain the newer existing value %v, got %v", newer, lastSeen)
	}
}

func TestUpsertPattern_MalformedExistingLastSeenIsOverwritten(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)

	query := `
		INSERT INTO payee_patterns (budget_id, normalized_description, payee_id, payee_name, category_id, category_name, occurrence_count, last_seen, created_at, updated_at)
		VALUES ('budget1', 'lidl warszawa', 'payee1', 'Test Payee', 'cat1', 'Cat 1', 1, 'not-a-timestamp', datetime('now'), datetime('now'))
	`
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("insert test pattern: %v", err)
	}

	newLastSeen := time.Now().UTC()
	err := store.UpsertPattern(context.Background(), txn.PayeePattern{
		BudgetID:              "budget1",
		NormalizedDescription: "lidl warszawa",
		PayeeID:               "payee1",
		PayeeName:             "Test Payee",
		CategoryID:            "cat1",
		CategoryName:          "Cat 1",
		LastSeen:              newLastSeen,
	})
	if err != nil {
		t.Fatalf("UpsertPattern: %v", err)
	}

	_, _, _, lastSeen := queryPatternRow(t, db, "budget1", "lidl warszawa", "payee1")
	if !lastSeen.Equal(newLastSeen.Truncate(time.Second)) {
		t.Errorf("expected malformed existing last_seen to be overwritten with %v, got %v", newLastSeen, lastSeen)
	}
}

func TestFindPatternsByDescription_AllStopwordFallbackReturnsEmptyNoError(t *testing.T) {
	db := setupPatternTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewPatternStore(db)
	now := time.Now()

	insertTestPatternWithDesc(t, db, "budget1", "lidl warszawa", "payee1", 1, now)

	patterns, err := store.FindPatternsByDescription(context.Background(), "budget1", "payment card pos", 50)
	if err != nil {
		t.Fatalf("FindPatternsByDescription: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns when query has no significant tokens, got %d: %+v", len(patterns), patterns)
	}
}
