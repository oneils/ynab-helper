package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/oneils/ynab-helper/internal/ynab"
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

	CREATE TABLE budget_transactions (
	    id TEXT PRIMARY KEY,
	    budget_id TEXT NOT NULL,
	    account_id TEXT NOT NULL,
	    account_name TEXT,
	    date TEXT NOT NULL,
	    amount INTEGER NOT NULL,
	    memo TEXT,
	    cleared TEXT,
	    approved INTEGER DEFAULT 0,
	    flag_color TEXT,
	    payee_id TEXT,
	    payee_name TEXT,
	    category_id TEXT,
	    category_name TEXT,
	    transfer_account_id TEXT,
	    transfer_transaction_id TEXT,
	    matched_transaction_id TEXT,
	    import_id TEXT,
	    deleted INTEGER DEFAULT 0,
	    FOREIGN KEY (budget_id) REFERENCES budgets(id) ON DELETE CASCADE
	);

	CREATE TABLE budget_subtransactions (
	    id TEXT PRIMARY KEY,
	    transaction_id TEXT NOT NULL,
	    amount INTEGER NOT NULL,
	    memo TEXT,
	    payee_id TEXT,
	    payee_name TEXT,
	    category_id TEXT,
	    category_name TEXT,
	    transfer_account_id TEXT,
	    deleted INTEGER DEFAULT 0,
	    FOREIGN KEY (transaction_id) REFERENCES budget_transactions(id) ON DELETE CASCADE
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

func TestReplaceBudgetTxns_FullFieldsAndSubtransactionsRoundTrip(t *testing.T) {
	db := setupYnabTestDB(t)
	defer db.Close() //nolint:errcheck

	store := &YnabStore{db: db}
	insertTestBudget(t, db, "budget1")

	txn := ynab.BudgetTxn{
		ID:                    "txn1",
		BudgetID:              "budget1",
		AccountID:             "acct1",
		AccountName:           "Checking",
		Date:                  "2026-06-10",
		Amount:                -12340,
		Memo:                  "note",
		Cleared:               "cleared",
		Approved:              true,
		FlagColor:             "red",
		PayeeID:               "payee1",
		PayeeName:             "Store",
		CategoryID:            "cat1",
		CategoryName:          "Groceries",
		TransferAccountID:     "acct2",
		TransferTransactionID: "txn2",
		MatchedTransactionID:  "txn3",
		ImportID:              "import1",
		Deleted:               false,
		Subtransactions: []ynab.BudgetSubTxn{
			{ID: "sub1", TransactionID: "txn1", Amount: -10000, Memo: "part A", PayeeID: "payee1", PayeeName: "Store", CategoryID: "cat1", CategoryName: "Groceries"},
			{ID: "sub2", TransactionID: "txn1", Amount: -2340, Memo: "part B", CategoryID: "cat2", CategoryName: "Household"},
		},
	}

	if err := store.ReplaceBudgetTxns(context.Background(), "budget1", "2026-01-01", []ynab.BudgetTxn{txn}); err != nil {
		t.Fatalf("ReplaceBudgetTxns insert: %v", err)
	}

	match, found, err := store.FindBudgetTxnMatch(context.Background(), "acct1", -12340, mustParseDate(t, "2026-06-10"))
	if err != nil {
		t.Fatalf("FindBudgetTxnMatch: %v", err)
	}
	if !found {
		t.Fatal("expected a match")
	}
	if match.AccountName != "Checking" || match.Cleared != "cleared" || !match.Approved || match.FlagColor != "red" ||
		match.PayeeID != "payee1" || match.CategoryID != "cat1" || match.CategoryName != "Groceries" ||
		match.TransferAccountID != "acct2" || match.TransferTransactionID != "txn2" ||
		match.MatchedTransactionID != "txn3" || match.ImportID != "import1" || match.Deleted {
		t.Fatalf("expected full fields to round-trip, got %+v", match)
	}

	// subtransactions aren't loaded by FindBudgetTxnMatch - check them directly.
	rows, err := db.Query("SELECT id, amount, category_name FROM budget_subtransactions WHERE transaction_id = ? ORDER BY id", "txn1")
	if err != nil {
		t.Fatalf("query subtransactions: %v", err)
	}
	defer rows.Close() //nolint:errcheck
	var got []string
	for rows.Next() {
		var id, categoryName string
		var amount int
		if err := rows.Scan(&id, &amount, &categoryName); err != nil {
			t.Fatalf("scan subtransaction: %v", err)
		}
		got = append(got, fmt.Sprintf("%s:%d:%s", id, amount, categoryName))
	}
	want := []string{"sub1:-10000:Groceries", "sub2:-2340:Household"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected subtransactions %v, got %v", want, got)
	}

	// Replacing the transaction with a different split set must drop the
	// stale subtransactions, not accumulate them.
	txn.Subtransactions = []ynab.BudgetSubTxn{
		{ID: "sub3", TransactionID: "txn1", Amount: -12340, Memo: "single split", CategoryID: "cat3", CategoryName: "Misc"},
	}
	if err := store.ReplaceBudgetTxns(context.Background(), "budget1", "2026-01-01", []ynab.BudgetTxn{txn}); err != nil {
		t.Fatalf("ReplaceBudgetTxns replace: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM budget_subtransactions WHERE transaction_id = ?", "txn1").Scan(&count); err != nil {
		t.Fatalf("count subtransactions after replace: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 subtransaction after replace, got %d", count)
	}
}

func TestReplaceBudgetTxns_InsertThenReplace(t *testing.T) {
	db := setupYnabTestDB(t)
	defer db.Close() //nolint:errcheck

	store := &YnabStore{db: db}
	insertTestBudget(t, db, "budget1")

	txn := ynab.BudgetTxn{
		ID:        "txn1",
		BudgetID:  "budget1",
		AccountID: "acct1",
		Date:      "2026-06-10",
		Amount:    -12340,
		PayeeName: "Store",
		Memo:      "note",
	}

	if err := store.ReplaceBudgetTxns(context.Background(), "budget1", "2026-01-01", []ynab.BudgetTxn{txn}); err != nil {
		t.Fatalf("ReplaceBudgetTxns insert: %v", err)
	}

	match, found, err := store.FindBudgetTxnMatch(context.Background(), "acct1", -12340, mustParseDate(t, "2026-06-10"))
	if err != nil {
		t.Fatalf("FindBudgetTxnMatch: %v", err)
	}
	if !found || match.PayeeName != "Store" {
		t.Fatalf("expected match with payee Store, got found=%v match=%+v", found, match)
	}

	txn.PayeeName = "Updated Store"
	txn.Memo = "updated note"
	if err := store.ReplaceBudgetTxns(context.Background(), "budget1", "2026-01-01", []ynab.BudgetTxn{txn}); err != nil {
		t.Fatalf("ReplaceBudgetTxns replace: %v", err)
	}

	match, found, err = store.FindBudgetTxnMatch(context.Background(), "acct1", -12340, mustParseDate(t, "2026-06-10"))
	if err != nil {
		t.Fatalf("FindBudgetTxnMatch after replace: %v", err)
	}
	if !found || match.PayeeName != "Updated Store" {
		t.Fatalf("expected replaced payee Updated Store, got found=%v match=%+v", found, match)
	}
}

func TestReplaceBudgetTxns_DeletesStaleBeforeInserting(t *testing.T) {
	db := setupYnabTestDB(t)
	defer db.Close() //nolint:errcheck

	store := &YnabStore{db: db}
	insertTestBudget(t, db, "budget1")
	insertTestBudget(t, db, "budget2")

	insertTestBudgetTxn(t, db, "before", "budget1", "acct1", "2026-06-01", 1000)
	insertTestBudgetTxn(t, db, "onDate", "budget1", "acct1", "2026-06-05", 1000)
	insertTestBudgetTxn(t, db, "after", "budget1", "acct1", "2026-06-10", 1000)
	insertTestBudgetTxn(t, db, "otherBudget", "budget2", "acct2", "2026-06-10", 1000)

	if err := store.ReplaceBudgetTxns(context.Background(), "budget1", "2026-06-05", nil); err != nil {
		t.Fatalf("ReplaceBudgetTxns: %v", err)
	}

	remaining := map[string]bool{}
	rows, err := db.Query("SELECT id FROM budget_transactions")
	if err != nil {
		t.Fatalf("query remaining rows: %v", err)
	}
	defer rows.Close() //nolint:errcheck
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan id: %v", err)
		}
		remaining[id] = true
	}

	if !remaining["before"] {
		t.Error("expected row before the cutoff date to remain")
	}
	if remaining["onDate"] {
		t.Error("expected row on the cutoff date to be deleted")
	}
	if remaining["after"] {
		t.Error("expected row after the cutoff date to be deleted")
	}
	if !remaining["otherBudget"] {
		t.Error("expected row for other budget to remain untouched")
	}
}

func TestFindBudgetTxnMatch(t *testing.T) {
	db := setupYnabTestDB(t)
	defer db.Close() //nolint:errcheck

	store := &YnabStore{db: db}
	insertTestBudget(t, db, "budget1")

	t.Run("exact date hit", func(t *testing.T) {
		insertTestBudgetTxn(t, db, "exact", "budget1", "acctA", "2026-06-10", 5000)

		match, found, err := store.FindBudgetTxnMatch(context.Background(), "acctA", 5000, mustParseDate(t, "2026-06-10"))
		if err != nil || !found || match.ID != "exact" {
			t.Fatalf("expected exact match, got found=%v match=%+v err=%v", found, match, err)
		}
	})

	t.Run("boundary hit day 3, day 4 no match", func(t *testing.T) {
		insertTestBudgetTxn(t, db, "day3", "budget1", "acctB", "2026-06-13", 6000)

		match, found, err := store.FindBudgetTxnMatch(context.Background(), "acctB", 6000, mustParseDate(t, "2026-06-10"))
		if err != nil || !found || match.ID != "day3" {
			t.Fatalf("expected day3 to match, got found=%v match=%+v err=%v", found, match, err)
		}

		insertTestBudgetTxn(t, db, "day4", "budget1", "acctC", "2026-06-14", 7000)
		_, found, err = store.FindBudgetTxnMatch(context.Background(), "acctC", 7000, mustParseDate(t, "2026-06-10"))
		if err != nil {
			t.Fatalf("FindBudgetTxnMatch: %v", err)
		}
		if found {
			t.Fatal("expected day4 (outside ±3 day window) not to match")
		}
	})

	t.Run("multiple candidates picks closest", func(t *testing.T) {
		insertTestBudgetTxn(t, db, "far", "budget1", "acctD", "2026-06-08", 8000)
		insertTestBudgetTxn(t, db, "near", "budget1", "acctD", "2026-06-11", 8000)

		match, found, err := store.FindBudgetTxnMatch(context.Background(), "acctD", 8000, mustParseDate(t, "2026-06-10"))
		if err != nil || !found || match.ID != "near" {
			t.Fatalf("expected closest match 'near', got found=%v match=%+v err=%v", found, match, err)
		}
	})

	t.Run("no match returns false not error", func(t *testing.T) {
		match, found, err := store.FindBudgetTxnMatch(context.Background(), "acctZ", 999, mustParseDate(t, "2026-06-10"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if found {
			t.Fatalf("expected no match, got %+v", match)
		}
	})

	t.Run("negative amount outflow", func(t *testing.T) {
		insertTestBudgetTxn(t, db, "outflow", "budget1", "acctE", "2026-06-10", -123450)

		match, found, err := store.FindBudgetTxnMatch(context.Background(), "acctE", -123450, mustParseDate(t, "2026-06-10"))
		if err != nil || !found || match.ID != "outflow" {
			t.Fatalf("expected outflow match, got found=%v match=%+v err=%v", found, match, err)
		}

		_, found, err = store.FindBudgetTxnMatch(context.Background(), "acctE", 123450, mustParseDate(t, "2026-06-10"))
		if err != nil {
			t.Fatalf("FindBudgetTxnMatch: %v", err)
		}
		if found {
			t.Fatal("expected positive amount not to match negative row")
		}
	})

	t.Run("local time boundary case", func(t *testing.T) {
		insertTestBudgetTxn(t, db, "localtz", "budget1", "acctF", "2026-06-10", 4200)

		loc, err := time.LoadLocation("Europe/Warsaw")
		if err != nil {
			t.Fatalf("load location: %v", err)
		}
		lateLocal := time.Date(2026, 6, 10, 23, 0, 0, 0, loc)

		match, found, err := store.FindBudgetTxnMatch(context.Background(), "acctF", 4200, lateLocal)
		if err != nil || !found || match.ID != "localtz" {
			t.Fatalf("expected local-time match not shifted by UTC conversion, got found=%v match=%+v err=%v", found, match, err)
		}
	})
}

func insertTestBudgetTxn(t *testing.T, db *sql.DB, id, budgetID, accountID, date string, amount int) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO budget_transactions (id, budget_id, account_id, date, amount) VALUES (?, ?, ?, ?, ?)`,
		id, budgetID, accountID, date, amount,
	)
	if err != nil {
		t.Fatalf("insert test budget txn: %v", err)
	}
}

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return d
}
