package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/oneils/ynab-helper/internal/txn"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	schema := `
	CREATE TABLE transactions (
	    id TEXT PRIMARY KEY,
	    amount REAL NOT NULL,
	    currency TEXT NOT NULL,
	    description TEXT NOT NULL,
	    payee TEXT,
	    account_id TEXT NOT NULL,
	    account_name TEXT NOT NULL,
	    status TEXT NOT NULL CHECK(status IN ('DRAFT','SKIPPED','PROCESSED','INVALID')),
	    created_at TEXT DEFAULT (datetime('now')),
	    txn_time TEXT NOT NULL,
	    raw_text TEXT,
	    raw_line_number INTEGER,
	    error_msg TEXT,
	    match_note TEXT
	);
	CREATE INDEX idx_txn_account_status ON transactions(account_id, status);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db
}

func insertTestTxn(t *testing.T, db *sql.DB, id, accountID, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	insertTestTxnWithTime(t, db, id, accountID, status, now)
}

func insertTestTxnWithTime(t *testing.T, db *sql.DB, id, accountID, status, txnTime string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
		INSERT INTO transactions (id, amount, currency, description, payee, account_id, account_name, status, created_at, txn_time, raw_text, raw_line_number, error_msg)
		VALUES (?, 12.34, 'USD', 'test desc', 'test payee', ?, 'Test Account', ?, ?, ?, '', 0, '')
	`
	if _, err := db.Exec(query, id, accountID, status, now, txnTime); err != nil {
		t.Fatalf("insert test txn: %v", err)
	}
}

func TestFetchTransactionsByAccount_SortDesc(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	store := NewTransactionStore(db)
	ctx := context.Background()
	accID := "acc-sort-desc"

	insertTestTxnWithTime(t, db, "txn-1", accID, "DRAFT", "2026-01-01T00:00:00Z")
	insertTestTxnWithTime(t, db, "txn-2", accID, "DRAFT", "2026-01-03T00:00:00Z")
	insertTestTxnWithTime(t, db, "txn-3", accID, "DRAFT", "2026-01-02T00:00:00Z")

	txns, err := store.FetchTransactionsByAccount(ctx, accID, "", "desc")
	if err != nil {
		t.Fatalf("FetchTransactionsByAccount: %v", err)
	}

	wantOrder := []string{"txn-2", "txn-3", "txn-1"}
	if len(txns) != len(wantOrder) {
		t.Fatalf("expected %d transactions, got %d", len(wantOrder), len(txns))
	}
	for i, id := range wantOrder {
		if txns[i].ID != id {
			t.Errorf("index %d: expected %s, got %s", i, id, txns[i].ID)
		}
	}
}

func TestFetchTransactionsByAccount_SortAsc(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	store := NewTransactionStore(db)
	ctx := context.Background()
	accID := "acc-sort-asc"

	insertTestTxnWithTime(t, db, "txn-1", accID, "DRAFT", "2026-01-01T00:00:00Z")
	insertTestTxnWithTime(t, db, "txn-2", accID, "DRAFT", "2026-01-03T00:00:00Z")
	insertTestTxnWithTime(t, db, "txn-3", accID, "DRAFT", "2026-01-02T00:00:00Z")

	txns, err := store.FetchTransactionsByAccount(ctx, accID, "", "asc")
	if err != nil {
		t.Fatalf("FetchTransactionsByAccount: %v", err)
	}

	wantOrder := []string{"txn-1", "txn-3", "txn-2"}
	if len(txns) != len(wantOrder) {
		t.Fatalf("expected %d transactions, got %d", len(wantOrder), len(txns))
	}
	for i, id := range wantOrder {
		if txns[i].ID != id {
			t.Errorf("index %d: expected %s, got %s", i, id, txns[i].ID)
		}
	}
}

func TestFetchTransactionsByAccount_TiedTxnTimeIsStable(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	store := NewTransactionStore(db)
	ctx := context.Background()
	accID := "acc-sort-tied"

	// All same txn_time - only the id tiebreaker can order these deterministically.
	insertTestTxnWithTime(t, db, "txn-b", accID, "DRAFT", "2026-01-01T00:00:00Z")
	insertTestTxnWithTime(t, db, "txn-a", accID, "DRAFT", "2026-01-01T00:00:00Z")
	insertTestTxnWithTime(t, db, "txn-c", accID, "DRAFT", "2026-01-01T00:00:00Z")

	wantOrders := map[string][]string{
		"asc":  {"txn-a", "txn-b", "txn-c"},
		"desc": {"txn-c", "txn-b", "txn-a"},
	}

	for _, sortOrder := range []string{"asc", "desc"} {
		first, err := store.FetchTransactionsByAccount(ctx, accID, "", sortOrder)
		if err != nil {
			t.Fatalf("FetchTransactionsByAccount (%s) run 1: %v", sortOrder, err)
		}
		second, err := store.FetchTransactionsByAccount(ctx, accID, "", sortOrder)
		if err != nil {
			t.Fatalf("FetchTransactionsByAccount (%s) run 2: %v", sortOrder, err)
		}

		wantOrder := wantOrders[sortOrder]
		for _, got := range [][]txn.Transaction{first, second} {
			if len(got) != len(wantOrder) {
				t.Fatalf("sortOrder %s: expected %d transactions, got %d", sortOrder, len(wantOrder), len(got))
			}
			for i, id := range wantOrder {
				if got[i].ID != id {
					t.Errorf("sortOrder %s: index %d: expected %s, got %s", sortOrder, i, id, got[i].ID)
				}
			}
		}
	}
}

func TestFetchTransactionsByAccount_SortInvalidDefaultsToDesc(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	store := NewTransactionStore(db)
	ctx := context.Background()
	accID := "acc-sort-invalid"

	insertTestTxnWithTime(t, db, "txn-1", accID, "DRAFT", "2026-01-01T00:00:00Z")
	insertTestTxnWithTime(t, db, "txn-2", accID, "DRAFT", "2026-01-03T00:00:00Z")
	insertTestTxnWithTime(t, db, "txn-3", accID, "DRAFT", "2026-01-02T00:00:00Z")

	txns, err := store.FetchTransactionsByAccount(ctx, accID, "", "'; DROP TABLE")
	if err != nil {
		t.Fatalf("FetchTransactionsByAccount: %v", err)
	}

	wantOrder := []string{"txn-2", "txn-3", "txn-1"}
	if len(txns) != len(wantOrder) {
		t.Fatalf("expected %d transactions, got %d", len(wantOrder), len(txns))
	}
	for i, id := range wantOrder {
		if txns[i].ID != id {
			t.Errorf("index %d: expected %s, got %s", i, id, txns[i].ID)
		}
	}
}

func TestCountByStatus(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	store := NewTransactionStore(db)
	ctx := context.Background()

	t.Run("empty account returns empty map", func(t *testing.T) {
		counts, err := store.CountByStatus(ctx, "acc-1")
		if err != nil {
			t.Fatalf("CountByStatus: %v", err)
		}
		if len(counts) != 0 {
			t.Errorf("expected empty map, got %d entries", len(counts))
		}
	})

	t.Run("mixed statuses for single account", func(t *testing.T) {
		accID := "acc-mixed"
		insertTestTxn(t, db, "txn-1", accID, "DRAFT")
		insertTestTxn(t, db, "txn-2", accID, "DRAFT")
		insertTestTxn(t, db, "txn-3", accID, "PROCESSED")
		insertTestTxn(t, db, "txn-4", accID, "PROCESSED")
		insertTestTxn(t, db, "txn-5", accID, "PROCESSED")
		insertTestTxn(t, db, "txn-6", accID, "SKIPPED")
		insertTestTxn(t, db, "txn-7", accID, "INVALID")

		counts, err := store.CountByStatus(ctx, accID)
		if err != nil {
			t.Fatalf("CountByStatus: %v", err)
		}

		if counts[txn.TransactionDraft] != 2 {
			t.Errorf("expected 2 DRAFT, got %d", counts[txn.TransactionDraft])
		}
		if counts[txn.TransactionProcessed] != 3 {
			t.Errorf("expected 3 PROCESSED, got %d", counts[txn.TransactionProcessed])
		}
		if counts[txn.TransactionSkipped] != 1 {
			t.Errorf("expected 1 SKIPPED, got %d", counts[txn.TransactionSkipped])
		}
		if counts[txn.TransactionInvalid] != 1 {
			t.Errorf("expected 1 INVALID, got %d", counts[txn.TransactionInvalid])
		}
		if len(counts) != 4 {
			t.Errorf("expected 4 distinct statuses, got %d", len(counts))
		}
	})

	t.Run("transactions from other accounts not counted", func(t *testing.T) {
		accA := "acc-a"
		accB := "acc-b"
		insertTestTxn(t, db, "txn-a1", accA, "DRAFT")
		insertTestTxn(t, db, "txn-a2", accA, "DRAFT")
		insertTestTxn(t, db, "txn-b1", accB, "DRAFT")
		insertTestTxn(t, db, "txn-b2", accB, "PROCESSED")

		countsA, err := store.CountByStatus(ctx, accA)
		if err != nil {
			t.Fatalf("CountByStatus A: %v", err)
		}
		if countsA[txn.TransactionDraft] != 2 {
			t.Errorf("accA: expected 2 DRAFT, got %d", countsA[txn.TransactionDraft])
		}
		if len(countsA) != 1 {
			t.Errorf("accA: expected 1 distinct status, got %d", len(countsA))
		}

		countsB, err := store.CountByStatus(ctx, accB)
		if err != nil {
			t.Fatalf("CountByStatus B: %v", err)
		}
		if countsB[txn.TransactionDraft] != 1 {
			t.Errorf("accB: expected 1 DRAFT, got %d", countsB[txn.TransactionDraft])
		}
		if countsB[txn.TransactionProcessed] != 1 {
			t.Errorf("accB: expected 1 PROCESSED, got %d", countsB[txn.TransactionProcessed])
		}
	})
}

func newTestTxn(id, accountID string, matchNote string) txn.Transaction {
	now := time.Now().UTC()
	return txn.Transaction{
		ID:          id,
		Amount:      12.34,
		Currency:    "USD",
		Description: "test desc",
		Payee:       "test payee",
		Account:     txn.BankAccount{ID: accountID, Name: "Test Account"},
		Status:      txn.TransactionDraft,
		CreatedAt:   now,
		TxnTime:     now,
		MatchNote:   matchNote,
	}
}

func TestInsertTransaction_MatchNote(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	store := NewTransactionStore(db)
	ctx := context.Background()

	t.Run("with match note", func(t *testing.T) {
		in := newTestTxn("txn-with-note", "acc-1", "possible match: existing YNAB txn on 2026-06-11")
		if err := store.InsertTransaction(ctx, in); err != nil {
			t.Fatalf("InsertTransaction: %v", err)
		}

		got, err := store.FindTransactionByID(ctx, "txn-with-note")
		if err != nil {
			t.Fatalf("FindTransactionByID: %v", err)
		}
		if got.MatchNote != in.MatchNote {
			t.Errorf("expected MatchNote %q, got %q", in.MatchNote, got.MatchNote)
		}
	})

	t.Run("without match note", func(t *testing.T) {
		in := newTestTxn("txn-without-note", "acc-1", "")
		if err := store.InsertTransaction(ctx, in); err != nil {
			t.Fatalf("InsertTransaction: %v", err)
		}

		got, err := store.FindTransactionByID(ctx, "txn-without-note")
		if err != nil {
			t.Fatalf("FindTransactionByID: %v", err)
		}
		if got.MatchNote != "" {
			t.Errorf("expected empty MatchNote, got %q", got.MatchNote)
		}
	})
}

func TestMatchNote_RoundTripsThroughFetchAndFind(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	store := NewTransactionStore(db)
	ctx := context.Background()
	accID := "acc-match-note"

	// Simulate a pre-existing row that predates the match_note column (NULL).
	insertTestTxn(t, db, "txn-legacy", accID, "DRAFT")

	withNote := newTestTxn("txn-noted", accID, "possible match: existing YNAB txn on 2026-06-11")
	if err := store.InsertTransaction(ctx, withNote); err != nil {
		t.Fatalf("InsertTransaction: %v", err)
	}

	txns, err := store.FetchTransactionsByAccount(ctx, accID, "", "asc")
	if err != nil {
		t.Fatalf("FetchTransactionsByAccount: %v", err)
	}

	byID := make(map[string]txn.Transaction, len(txns))
	for _, tx := range txns {
		byID[tx.ID] = tx
	}

	if got := byID["txn-legacy"].MatchNote; got != "" {
		t.Errorf("expected empty MatchNote for NULL legacy row, got %q", got)
	}
	if got := byID["txn-noted"].MatchNote; got != withNote.MatchNote {
		t.Errorf("expected MatchNote %q, got %q", withNote.MatchNote, got)
	}

	found, err := store.FindTransactionByID(ctx, "txn-legacy")
	if err != nil {
		t.Fatalf("FindTransactionByID: %v", err)
	}
	if found.MatchNote != "" {
		t.Errorf("expected empty MatchNote for NULL legacy row via FindTransactionByID, got %q", found.MatchNote)
	}
}
