package sqlite

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupYnabTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	schema := `
	CREATE TABLE budgets (
	    id TEXT PRIMARY KEY,
	    name TEXT NOT NULL,
	    last_modified_on TEXT,
	    first_month TEXT,
	    last_month TEXT,
	    date_format TEXT,
	    currency_format TEXT
	);

	CREATE TABLE category_groups (
	    id TEXT PRIMARY KEY,
	    budget_id TEXT NOT NULL,
	    name TEXT NOT NULL,
	    hidden INTEGER DEFAULT 0,
	    deleted INTEGER DEFAULT 0,
	    FOREIGN KEY (budget_id) REFERENCES budgets(id) ON DELETE CASCADE
	);

	CREATE TABLE categories (
	    id TEXT PRIMARY KEY,
	    budget_id TEXT NOT NULL,
	    category_group_id TEXT NOT NULL,
	    category_group_name TEXT NOT NULL,
	    name TEXT NOT NULL,
	    hidden INTEGER DEFAULT 0,
	    deleted INTEGER DEFAULT 0,
	    original_category_group_id TEXT,
	    note TEXT,
	    budgeted INTEGER DEFAULT 0,
	    activity INTEGER DEFAULT 0,
	    balance INTEGER DEFAULT 0,
	    goal_type TEXT,
	    goal_days INTEGER,
	    goal_cadence INTEGER,
	    goal_cadence_frequency INTEGER,
	    goal_target INTEGER,
	    goal_target_month TEXT,
	    goal_creation_month TEXT,
	    goal_percentage_complete INTEGER,
	    goal_months_to_budget INTEGER,
	    goal_under_funded INTEGER,
	    goal_overall_funded INTEGER,
	    goal_overall_left INTEGER,
	    FOREIGN KEY (budget_id) REFERENCES budgets(id) ON DELETE CASCADE,
	    FOREIGN KEY (category_group_id) REFERENCES category_groups(id) ON DELETE CASCADE
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db
}

func insertTestBudget(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO budgets (id, name) VALUES (?, ?)`, id, "Test Budget"); err != nil {
		t.Fatalf("insert test budget: %v", err)
	}
}

func insertTestCategoryGroup(t *testing.T, db *sql.DB, id, budgetID, name string, hidden, deleted int) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO category_groups (id, budget_id, name, hidden, deleted) VALUES (?, ?, ?, ?, ?)`,
		id, budgetID, name, hidden, deleted,
	)
	if err != nil {
		t.Fatalf("insert test category group: %v", err)
	}
}

func insertTestCategory(t *testing.T, db *sql.DB, id, budgetID, groupID, groupName, name string, deleted int) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO categories (id, budget_id, category_group_id, category_group_name, name, deleted) VALUES (?, ?, ?, ?, ?, ?)`,
		id, budgetID, groupID, groupName, name, deleted,
	)
	if err != nil {
		t.Fatalf("insert test category: %v", err)
	}
}

func TestFetchCategoriesByBudget_GroupsWithCategories(t *testing.T) {
	db := setupYnabTestDB(t)
	defer db.Close() //nolint:errcheck

	store := &YnabStore{db: db}
	insertTestBudget(t, db, "budget1")

	for g := 1; g <= 3; g++ {
		groupID := "group" + string(rune('0'+g))
		insertTestCategoryGroup(t, db, groupID, "budget1", "Group "+string(rune('0'+g)), 0, 0)
		for c := 1; c <= 3; c++ {
			catID := groupID + "_cat" + string(rune('0'+c))
			insertTestCategory(t, db, catID, "budget1", groupID, "Group "+string(rune('0'+g)), "Cat "+string(rune('0'+c)), 0)
		}
	}

	groups, err := store.FetchCategoriesByBudget(context.Background(), "budget1")
	if err != nil {
		t.Fatalf("FetchCategoriesByBudget: %v", err)
	}

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}

	for _, g := range groups {
		if len(g.Categories) != 3 {
			t.Errorf("group %s: expected 3 categories, got %d", g.ID, len(g.Categories))
		}
		for _, c := range g.Categories {
			if c.CategoryGroupID != g.ID {
				t.Errorf("category %s has group id %s, expected %s", c.ID, c.CategoryGroupID, g.ID)
			}
		}
	}
}

func TestFetchCategoriesByBudget_EmptyGroup(t *testing.T) {
	db := setupYnabTestDB(t)
	defer db.Close() //nolint:errcheck

	store := &YnabStore{db: db}
	insertTestBudget(t, db, "budget1")
	insertTestCategoryGroup(t, db, "group1", "budget1", "Empty Group", 0, 0)

	groups, err := store.FetchCategoriesByBudget(context.Background(), "budget1")
	if err != nil {
		t.Fatalf("FetchCategoriesByBudget: %v", err)
	}

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Categories) != 0 {
		t.Errorf("expected no categories, got %d", len(groups[0].Categories))
	}
}

func TestFetchCategoriesByBudget_ExcludesDeleted(t *testing.T) {
	db := setupYnabTestDB(t)
	defer db.Close() //nolint:errcheck

	store := &YnabStore{db: db}
	insertTestBudget(t, db, "budget1")

	insertTestCategoryGroup(t, db, "group1", "budget1", "Active Group", 0, 0)
	insertTestCategory(t, db, "cat1", "budget1", "group1", "Active Group", "Active Cat", 0)
	insertTestCategory(t, db, "cat2", "budget1", "group1", "Active Group", "Deleted Cat", 1)

	insertTestCategoryGroup(t, db, "group2", "budget1", "Deleted Group", 0, 1)
	insertTestCategory(t, db, "cat3", "budget1", "group2", "Deleted Group", "Cat In Deleted Group", 0)

	groups, err := store.FetchCategoriesByBudget(context.Background(), "budget1")
	if err != nil {
		t.Fatalf("FetchCategoriesByBudget: %v", err)
	}

	if len(groups) != 1 {
		t.Fatalf("expected 1 group (deleted group excluded), got %d", len(groups))
	}
	if groups[0].ID != "group1" {
		t.Fatalf("expected group1, got %s", groups[0].ID)
	}
	if len(groups[0].Categories) != 1 {
		t.Fatalf("expected 1 category (deleted category excluded), got %d", len(groups[0].Categories))
	}
	if groups[0].Categories[0].ID != "cat1" {
		t.Errorf("expected cat1, got %s", groups[0].Categories[0].ID)
	}
}

func TestFetchCategoriesByBudget_NoGroups(t *testing.T) {
	db := setupYnabTestDB(t)
	defer db.Close() //nolint:errcheck

	store := &YnabStore{db: db}
	insertTestBudget(t, db, "budget1")

	groups, err := store.FetchCategoriesByBudget(context.Background(), "budget1")
	if err != nil {
		t.Fatalf("FetchCategoriesByBudget: %v", err)
	}

	if len(groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(groups))
	}
}
