package sqlite

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupParserMappingTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	schema := `
	CREATE TABLE account_parser_mappings (
	    account_id TEXT PRIMARY KEY,
	    parser_name TEXT NOT NULL,
	    updated_at TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db
}

func TestGetParserMapping(t *testing.T) {
	db := setupParserMappingTestDB(t)
	defer func() { _ = db.Close() }()

	store := NewParserMappingStore(db)
	ctx := context.Background()

	t.Run("missing row returns empty string and nil error", func(t *testing.T) {
		name, err := store.GetParserMapping(ctx, "acc-missing")
		if err != nil {
			t.Fatalf("GetParserMapping: %v", err)
		}
		if name != "" {
			t.Errorf("expected empty string, got %q", name)
		}
	})

	t.Run("stored name is returned", func(t *testing.T) {
		if err := store.SaveParserMapping(ctx, "acc-1", "Santander"); err != nil {
			t.Fatalf("SaveParserMapping: %v", err)
		}
		name, err := store.GetParserMapping(ctx, "acc-1")
		if err != nil {
			t.Fatalf("GetParserMapping: %v", err)
		}
		if name != "Santander" {
			t.Errorf("expected Santander, got %q", name)
		}
	})

	t.Run("stored empty string returns empty string and nil error", func(t *testing.T) {
		if err := store.SaveParserMapping(ctx, "acc-2", ""); err != nil {
			t.Fatalf("SaveParserMapping: %v", err)
		}
		name, err := store.GetParserMapping(ctx, "acc-2")
		if err != nil {
			t.Fatalf("GetParserMapping: %v", err)
		}
		if name != "" {
			t.Errorf("expected empty string, got %q", name)
		}
	})
}

func TestSaveParserMapping(t *testing.T) {
	db := setupParserMappingTestDB(t)
	defer func() { _ = db.Close() }()

	store := NewParserMappingStore(db)
	ctx := context.Background()

	t.Run("insert new mapping", func(t *testing.T) {
		if err := store.SaveParserMapping(ctx, "acc-1", "Revolut"); err != nil {
			t.Fatalf("SaveParserMapping: %v", err)
		}
		name, err := store.GetParserMapping(ctx, "acc-1")
		if err != nil {
			t.Fatalf("GetParserMapping: %v", err)
		}
		if name != "Revolut" {
			t.Errorf("expected Revolut, got %q", name)
		}
	})

	t.Run("upsert overwrites existing mapping", func(t *testing.T) {
		if err := store.SaveParserMapping(ctx, "acc-1", "PKO"); err != nil {
			t.Fatalf("SaveParserMapping: %v", err)
		}
		name, err := store.GetParserMapping(ctx, "acc-1")
		if err != nil {
			t.Fatalf("GetParserMapping: %v", err)
		}
		if name != "PKO" {
			t.Errorf("expected PKO, got %q", name)
		}
	})

	t.Run("saving empty string clears mapping", func(t *testing.T) {
		if err := store.SaveParserMapping(ctx, "acc-1", ""); err != nil {
			t.Fatalf("SaveParserMapping: %v", err)
		}
		name, err := store.GetParserMapping(ctx, "acc-1")
		if err != nil {
			t.Fatalf("GetParserMapping: %v", err)
		}
		if name != "" {
			t.Errorf("expected empty string, got %q", name)
		}
	})
}
