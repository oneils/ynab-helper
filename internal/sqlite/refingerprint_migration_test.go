package sqlite

import (
	"database/sql"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func setupMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	goose.SetBaseFS(embedMigrations)

	return db
}

func TestRefingerprintMigration_MergesCollidingRowsAndSumsEvidence(t *testing.T) {
	db := setupMigrationTestDB(t)

	if err := goose.UpTo(db, "migrations", 4); err != nil {
		t.Fatalf("migrate to version 4: %v", err)
	}

	now := time.Now().UTC()
	older := now.Add(-24 * time.Hour).Format(time.RFC3339)
	newer := now.Format(time.RFC3339)

	insert := `
		INSERT INTO payee_patterns (
			budget_id, normalized_description, payee_id, payee_name,
			category_id, category_name, occurrence_count, last_seen
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	if _, err := db.Exec(insert,
		"budget1", "payment card 1234 lidl warszawa", "payee1", "Lidl",
		"cat1", "Groceries", 3, older); err != nil {
		t.Fatalf("seed row 1: %v", err)
	}
	if _, err := db.Exec(insert,
		"budget1", "payment card 5678 lidl warszawa", "payee1", "Lidl",
		"cat1", "Groceries", 2, newer); err != nil {
		t.Fatalf("seed row 2: %v", err)
	}

	if err := goose.UpTo(db, "migrations", 5); err != nil {
		t.Fatalf("migrate to version 5: %v", err)
	}

	rows, err := db.Query(`
		SELECT normalized_description, occurrence_count, last_seen
		FROM payee_patterns
		WHERE budget_id = 'budget1' AND payee_id = 'payee1'
	`)
	if err != nil {
		t.Fatalf("query patterns: %v", err)
	}
	defer func() { _ = rows.Close() }()

	type result struct {
		desc     string
		count    int
		lastSeen string
	}
	var got []result
	for rows.Next() {
		var r result
		if err := rows.Scan(&r.desc, &r.count, &r.lastSeen); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate rows: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected exactly one surviving row after merge, got %d: %+v", len(got), got)
	}

	if got[0].desc != "payment card lidl warszawa" {
		t.Errorf("normalized_description = %q, want %q", got[0].desc, "payment card lidl warszawa")
	}
	if got[0].count != 5 {
		t.Errorf("occurrence_count = %d, want 5 (summed)", got[0].count)
	}
	if got[0].lastSeen != newer {
		t.Errorf("last_seen = %q, want newer value %q", got[0].lastSeen, newer)
	}
}

func TestRefingerprintMigration_NoCollisionLeavesRowUnmerged(t *testing.T) {
	db := setupMigrationTestDB(t)

	if err := goose.UpTo(db, "migrations", 4); err != nil {
		t.Fatalf("migrate to version 4: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	insert := `
		INSERT INTO payee_patterns (
			budget_id, normalized_description, payee_id, payee_name,
			category_id, category_name, occurrence_count, last_seen
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	if _, err := db.Exec(insert,
		"budget1", "payment card 1234 lidl warszawa", "payee1", "Lidl",
		"cat1", "Groceries", 3, now); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	if err := goose.UpTo(db, "migrations", 5); err != nil {
		t.Fatalf("migrate to version 5: %v", err)
	}

	var desc string
	var count int
	if err := db.QueryRow(`
		SELECT normalized_description, occurrence_count
		FROM payee_patterns WHERE budget_id = 'budget1' AND payee_id = 'payee1'
	`).Scan(&desc, &count); err != nil {
		t.Fatalf("query pattern: %v", err)
	}

	if desc != "payment card lidl warszawa" {
		t.Errorf("normalized_description = %q, want %q", desc, "payment card lidl warszawa")
	}
	if count != 3 {
		t.Errorf("occurrence_count = %d, want unchanged 3", count)
	}
}

func TestRefingerprintMigration_IsRegisteredAndReachesVersion5(t *testing.T) {
	db := setupMigrationTestDB(t)

	if err := goose.Up(db, "migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	version, err := goose.GetDBVersion(db)
	if err != nil {
		t.Fatalf("get db version: %v", err)
	}

	if version < 5 {
		t.Fatalf("db version = %d, want >= 5 (migration 00005 not registered — check the blank import in sqlite.go)", version)
	}
}
