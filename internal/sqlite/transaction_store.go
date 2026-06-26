package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

// TransactionStore handles transaction persistence in SQLite.
type TransactionStore struct {
	db *sql.DB
}

// NewTransactionStore creates a new TransactionStore.
func NewTransactionStore(db *sql.DB) *TransactionStore {
	return &TransactionStore{db: db}
}

// InsertTransaction inserts a transaction into the database.
// If the transaction already exists (duplicate key), it is skipped.
func (s *TransactionStore) InsertTransaction(ctx context.Context, t txn.Transaction) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `
		INSERT OR IGNORE INTO transactions (
			id, amount, currency, description, payee,
			account_id, account_name, status, created_at, txn_time,
			raw_text, raw_line_number, error_msg
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := s.db.ExecContext(ctx, query,
		t.ID,
		t.Amount,
		t.Currency,
		t.Description,
		t.Payee,
		t.Account.ID,
		t.Account.Name,
		t.Status,
		t.CreatedAt.Format(time.RFC3339),
		t.TxnTime.Format(time.RFC3339),
		t.RawText,
		t.RawLineNumber,
		t.ErrorMsg,
	)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		slog.Debug("Transaction already exists, skipping", "payee", t.Payee)
	}

	return nil
}

// FetchTransactionsByAccount fetches transactions for a specific account.
func (s *TransactionStore) FetchTransactionsByAccount(ctx context.Context, accID string, status string) ([]txn.Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `
		SELECT id, amount, currency, description, payee,
		       account_id, account_name, status, created_at, txn_time,
		       raw_text, raw_line_number, error_msg
		FROM transactions
	`
	args := []interface{}{}
	conditions := []string{}

	// Add account filter only if account ID is provided
	if accID != "" {
		conditions = append(conditions, "account_id = ?")
		args = append(args, accID)
	}

	// Add status filter if provided
	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}

	// Add WHERE clause only if there are conditions
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY txn_time ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find transactions: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var txns []txn.Transaction
	for rows.Next() {
		var t txn.Transaction
		var createdAt, txnTime string

		err := rows.Scan(
			&t.ID,
			&t.Amount,
			&t.Currency,
			&t.Description,
			&t.Payee,
			&t.Account.ID,
			&t.Account.Name,
			&t.Status,
			&createdAt,
			&txnTime,
			&t.RawText,
			&t.RawLineNumber,
			&t.ErrorMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}

		// Parse timestamps
		t.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}

		t.TxnTime, err = time.Parse(time.RFC3339, txnTime)
		if err != nil {
			return nil, fmt.Errorf("parse txn_time: %w", err)
		}

		txns = append(txns, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}

	return txns, nil
}

// FindTransactionByID fetches a transaction by its ID.
func (s *TransactionStore) FindTransactionByID(ctx context.Context, id string) (txn.Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `
		SELECT id, amount, currency, description, payee,
		       account_id, account_name, status, created_at, txn_time,
		       raw_text, raw_line_number, error_msg
		FROM transactions
		WHERE id = ?
	`

	var t txn.Transaction
	var createdAt, txnTime string

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&t.ID,
		&t.Amount,
		&t.Currency,
		&t.Description,
		&t.Payee,
		&t.Account.ID,
		&t.Account.Name,
		&t.Status,
		&createdAt,
		&txnTime,
		&t.RawText,
		&t.RawLineNumber,
		&t.ErrorMsg,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return txn.Transaction{}, fmt.Errorf("find transaction: %w", err)
		}
		return txn.Transaction{}, fmt.Errorf("find transaction: %w", err)
	}

	// Parse timestamps
	t.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return txn.Transaction{}, fmt.Errorf("parse created_at: %w", err)
	}

	t.TxnTime, err = time.Parse(time.RFC3339, txnTime)
	if err != nil {
		return txn.Transaction{}, fmt.Errorf("parse txn_time: %w", err)
	}

	return t, nil
}

// UpdateTransactionStatus updates the status of a transaction.
func (s *TransactionStore) UpdateTransactionStatus(ctx context.Context, id string, status txn.TransactionStatus) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `UPDATE transactions SET status = ? WHERE id = ?`

	_, err := s.db.ExecContext(ctx, query, status, id)
	if err != nil {
		// Check if it's a constraint error
		if strings.Contains(err.Error(), "constraint") {
			return fmt.Errorf("invalid status value: %w", err)
		}
		return fmt.Errorf("update transaction status: %w", err)
	}

	return nil
}
