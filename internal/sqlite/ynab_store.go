package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/oneils/ynab-helper/internal/ynab"
)

// YnabStore implements all YNAB-related storage operations.
type YnabStore struct {
	db *sql.DB
}

// NewYnabStore creates a new YnabStore.
func NewYnabStore(db *sql.DB) *YnabStore {
	return &YnabStore{db: db}
}

// --- Budget operations ---

// UpsertBudget inserts or replaces a budget in the database.
func (s *YnabStore) UpsertBudget(ctx context.Context, budget ynab.Budget) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Marshal complex fields to JSON
	dateFormat, err := json.Marshal(budget.DateFormat)
	if err != nil {
		return fmt.Errorf("marshal date format: %w", err)
	}

	currencyFormat, err := json.Marshal(budget.CurrencyFormat)
	if err != nil {
		return fmt.Errorf("marshal currency format: %w", err)
	}

	// Check if budget exists
	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM budgets WHERE id = ?", budget.ID).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("check budget existence: %w", err)
	}

	// Upsert budget
	query := `
		INSERT OR REPLACE INTO budgets (
			id, name, last_modified_on, first_month, last_month,
			date_format, currency_format
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err = tx.ExecContext(ctx, query,
		budget.ID,
		budget.Name,
		budget.LastModifiedOn.Format(time.RFC3339),
		budget.FirstMonth,
		budget.LastMonth,
		string(dateFormat),
		string(currencyFormat),
	)
	if err != nil {
		return fmt.Errorf("upsert budget: %w", err)
	}

	if exists {
		slog.Debug("Budget found, replacing", "name", budget.Name)
	} else {
		slog.Debug("Budget not found, inserting", "name", budget.Name)
	}

	// Delete existing accounts for this budget
	_, err = tx.ExecContext(ctx, "DELETE FROM accounts WHERE budget_id = ?", budget.ID)
	if err != nil {
		return fmt.Errorf("delete old accounts: %w", err)
	}

	// Insert accounts
	for _, acc := range budget.Accounts {
		err = s.insertAccount(ctx, tx, budget.ID, acc)
		if err != nil {
			return fmt.Errorf("insert account: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// insertAccount inserts an account within a transaction.
func (s *YnabStore) insertAccount(ctx context.Context, tx *sql.Tx, budgetID string, acc ynab.Account) error {
	query := `
		INSERT INTO accounts (
			id, budget_id, name, type, on_budget, closed, note,
			balance, cleared_balance, uncleared_balance,
			direct_import_linked, direct_import_in_error,
			last_reconciled_at, debt_original_balance,
			transfer_payee_id, deleted
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	lastReconciledAt := ""
	if acc.LastReconciledAt != nil && !acc.LastReconciledAt.IsZero() {
		lastReconciledAt = acc.LastReconciledAt.Format(time.RFC3339)
	}

	_, err := tx.ExecContext(ctx, query,
		acc.ID,
		budgetID,
		acc.Name,
		acc.Type,
		boolToInt(acc.OnBudget),
		boolToInt(acc.Closed),
		acc.Note,
		acc.Balance,
		acc.ClearedBalance,
		acc.UnclearedBalance,
		boolToInt(acc.DirectImportLinked),
		boolToInt(acc.DirectImportInError),
		lastReconciledAt,
		acc.DebtOriginalBalance,
		acc.TransferPayeeId,
		boolToInt(acc.Deleted),
	)
	return err
}

// FetchAllBudgets returns all budgets from the database.
func (s *YnabStore) FetchAllBudgets(ctx context.Context) ([]ynab.Budget, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// First, fetch all budgets
	query := `
		SELECT id, name, last_modified_on, first_month, last_month,
		       date_format, currency_format
		FROM budgets
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("find budgets: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var budgets []ynab.Budget
	budgetIDs := make([]string, 0)

	for rows.Next() {
		budget, err := s.scanBudget(rows)
		if err != nil {
			return nil, err
		}
		budgets = append(budgets, budget)
		budgetIDs = append(budgetIDs, budget.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate budgets: %w", err)
	}

	// If no budgets, return early
	if len(budgets) == 0 {
		slog.Info("fetched budgets from database", "count", 0)
		return budgets, nil
	}

	// Fetch all accounts for all budgets in one query
	accountsMap, err := s.fetchAccountsForBudgets(ctx, budgetIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch accounts: %w", err)
	}

	// Assign accounts to budgets
	for i := range budgets {
		budgets[i].Accounts = accountsMap[budgets[i].ID]
	}

	slog.Info("fetched budgets from database", "count", len(budgets))
	return budgets, nil
}

// FindBudgetByID finds a budget by its ID.
func (s *YnabStore) FindBudgetByID(ctx context.Context, id string) (ynab.Budget, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `
		SELECT id, name, last_modified_on, first_month, last_month,
		       date_format, currency_format
		FROM budgets
		WHERE id = ?
	`

	row := s.db.QueryRowContext(ctx, query, id)
	budget, err := s.scanBudget(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return ynab.Budget{}, fmt.Errorf("find budget: %w", err)
		}
		return ynab.Budget{}, fmt.Errorf("find budget: %w", err)
	}

	// Load accounts for this budget
	accounts, err := s.fetchAccountsByBudget(ctx, budget.ID)
	if err != nil {
		return ynab.Budget{}, fmt.Errorf("fetch accounts: %w", err)
	}
	budget.Accounts = accounts

	return budget, nil
}

// FindBudgetByAccountID finds a budget by account ID.
func (s *YnabStore) FindBudgetByAccountID(ctx context.Context, accID string) (ynab.Budget, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `
		SELECT b.id, b.name, b.last_modified_on, b.first_month, b.last_month,
		       b.date_format, b.currency_format
		FROM budgets b
		JOIN accounts a ON a.budget_id = b.id
		WHERE a.id = ?
	`

	row := s.db.QueryRowContext(ctx, query, accID)
	budget, err := s.scanBudget(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return ynab.Budget{}, fmt.Errorf("find budget by account: %w", err)
		}
		return ynab.Budget{}, fmt.Errorf("find budget by account: %w", err)
	}

	// Load accounts for this budget
	accounts, err := s.fetchAccountsByBudget(ctx, budget.ID)
	if err != nil {
		return ynab.Budget{}, fmt.Errorf("fetch accounts: %w", err)
	}
	budget.Accounts = accounts

	return budget, nil
}

// scanBudget scans a budget from a row.
func (s *YnabStore) scanBudget(row interface{ Scan(...interface{}) error }) (ynab.Budget, error) {
	var budget ynab.Budget
	var lastModifiedOn string
	var dateFormatJSON, currencyFormatJSON string

	err := row.Scan(
		&budget.ID,
		&budget.Name,
		&lastModifiedOn,
		&budget.FirstMonth,
		&budget.LastMonth,
		&dateFormatJSON,
		&currencyFormatJSON,
	)
	if err != nil {
		return ynab.Budget{}, err
	}

	// Parse timestamp
	if lastModifiedOn != "" {
		budget.LastModifiedOn, err = time.Parse(time.RFC3339, lastModifiedOn)
		if err != nil {
			return ynab.Budget{}, fmt.Errorf("parse last_modified_on: %w", err)
		}
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(dateFormatJSON), &budget.DateFormat); err != nil {
		return ynab.Budget{}, fmt.Errorf("unmarshal date format: %w", err)
	}

	if err := json.Unmarshal([]byte(currencyFormatJSON), &budget.CurrencyFormat); err != nil {
		return ynab.Budget{}, fmt.Errorf("unmarshal currency format: %w", err)
	}

	return budget, nil
}

// fetchAccountsByBudget fetches all accounts for a budget.
func (s *YnabStore) fetchAccountsByBudget(ctx context.Context, budgetID string) ([]ynab.Account, error) {
	query := `
		SELECT id, name, type, on_budget, closed, note,
		       balance, cleared_balance, uncleared_balance,
		       direct_import_linked, direct_import_in_error,
		       last_reconciled_at, debt_original_balance,
		       transfer_payee_id, deleted
		FROM accounts
		WHERE budget_id = ?
	`

	rows, err := s.db.QueryContext(ctx, query, budgetID)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var accounts []ynab.Account
	for rows.Next() {
		var acc ynab.Account
		var lastReconciledAt sql.NullString
		var onBudget, closed, directImportLinked, directImportInError, deleted int

		err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.Type,
			&onBudget,
			&closed,
			&acc.Note,
			&acc.Balance,
			&acc.ClearedBalance,
			&acc.UnclearedBalance,
			&directImportLinked,
			&directImportInError,
			&lastReconciledAt,
			&acc.DebtOriginalBalance,
			&acc.TransferPayeeId,
			&deleted,
		)
		if err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}

		acc.OnBudget = intToBool(onBudget)
		acc.Closed = intToBool(closed)
		acc.DirectImportLinked = intToBool(directImportLinked)
		acc.DirectImportInError = intToBool(directImportInError)
		acc.Deleted = intToBool(deleted)

		if lastReconciledAt.Valid && lastReconciledAt.String != "" {
			parsed, err := time.Parse(time.RFC3339, lastReconciledAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse last_reconciled_at: %w", err)
			}
			acc.LastReconciledAt = &parsed
		}

		accounts = append(accounts, acc)
	}

	return accounts, rows.Err()
}

// fetchAccountsForBudgets fetches all accounts for multiple budgets in one query.
func (s *YnabStore) fetchAccountsForBudgets(ctx context.Context, budgetIDs []string) (map[string][]ynab.Account, error) {
	if len(budgetIDs) == 0 {
		return make(map[string][]ynab.Account), nil
	}

	// Build IN clause for multiple budget IDs
	query := `
		SELECT id, budget_id, name, type, on_budget, closed, note,
		       balance, cleared_balance, uncleared_balance,
		       direct_import_linked, direct_import_in_error,
		       last_reconciled_at, debt_original_balance,
		       transfer_payee_id, deleted
		FROM accounts
		WHERE budget_id IN (?` + strings.Repeat(",?", len(budgetIDs)-1) + `)
	`

	// Convert budgetIDs to []interface{} for QueryContext
	args := make([]interface{}, len(budgetIDs))
	for i, id := range budgetIDs {
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query accounts for budgets: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	accountsMap := make(map[string][]ynab.Account)

	for rows.Next() {
		var acc ynab.Account
		var budgetID string
		var lastReconciledAt sql.NullString
		var onBudget, closed, directImportLinked, directImportInError, deleted int

		err := rows.Scan(
			&acc.ID,
			&budgetID,
			&acc.Name,
			&acc.Type,
			&onBudget,
			&closed,
			&acc.Note,
			&acc.Balance,
			&acc.ClearedBalance,
			&acc.UnclearedBalance,
			&directImportLinked,
			&directImportInError,
			&lastReconciledAt,
			&acc.DebtOriginalBalance,
			&acc.TransferPayeeId,
			&deleted,
		)
		if err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}

		acc.OnBudget = intToBool(onBudget)
		acc.Closed = intToBool(closed)
		acc.DirectImportLinked = intToBool(directImportLinked)
		acc.DirectImportInError = intToBool(directImportInError)
		acc.Deleted = intToBool(deleted)

		if lastReconciledAt.Valid && lastReconciledAt.String != "" {
			parsed, err := time.Parse(time.RFC3339, lastReconciledAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse last_reconciled_at: %w", err)
			}
			acc.LastReconciledAt = &parsed
		}

		accountsMap[budgetID] = append(accountsMap[budgetID], acc)
	}

	return accountsMap, rows.Err()
}

// --- Account operations ---

// UpsertAccount inserts or replaces an account in the database.
func (s *YnabStore) UpsertAccount(ctx context.Context, acc ynab.Account) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Note: This method is called for standalone account updates
	// We need to find the budget_id for this account first
	var budgetID string
	err := s.db.QueryRowContext(ctx, "SELECT budget_id FROM accounts WHERE id = ?", acc.ID).Scan(&budgetID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("find account budget: %w", err)
	}

	if err == sql.ErrNoRows {
		slog.Debug("Account not found, inserting", "name", acc.Name)
		// Cannot insert without budget_id - this shouldn't happen in normal operation
		return fmt.Errorf("cannot insert account without budget_id")
	}

	slog.Debug("Account found, replacing", "name", acc.Name)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Delete old account
	_, err = tx.ExecContext(ctx, "DELETE FROM accounts WHERE id = ?", acc.ID)
	if err != nil {
		return fmt.Errorf("delete old account: %w", err)
	}

	// Insert new account
	err = s.insertAccount(ctx, tx, budgetID, acc)
	if err != nil {
		return fmt.Errorf("insert account: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// FetchAllAccounts returns all accounts from the database.
func (s *YnabStore) FetchAllAccounts(ctx context.Context) ([]ynab.Account, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `
		SELECT id, name, type, on_budget, closed, note,
		       balance, cleared_balance, uncleared_balance,
		       direct_import_linked, direct_import_in_error,
		       last_reconciled_at, debt_original_balance,
		       transfer_payee_id, deleted
		FROM accounts
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var accounts []ynab.Account
	for rows.Next() {
		var acc ynab.Account
		var lastReconciledAt sql.NullString
		var onBudget, closed, directImportLinked, directImportInError, deleted int

		err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.Type,
			&onBudget,
			&closed,
			&acc.Note,
			&acc.Balance,
			&acc.ClearedBalance,
			&acc.UnclearedBalance,
			&directImportLinked,
			&directImportInError,
			&lastReconciledAt,
			&acc.DebtOriginalBalance,
			&acc.TransferPayeeId,
			&deleted,
		)
		if err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}

		acc.OnBudget = intToBool(onBudget)
		acc.Closed = intToBool(closed)
		acc.DirectImportLinked = intToBool(directImportLinked)
		acc.DirectImportInError = intToBool(directImportInError)
		acc.Deleted = intToBool(deleted)

		if lastReconciledAt.Valid && lastReconciledAt.String != "" {
			parsed, err := time.Parse(time.RFC3339, lastReconciledAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse last_reconciled_at: %w", err)
			}
			acc.LastReconciledAt = &parsed
		}

		accounts = append(accounts, acc)
	}

	return accounts, rows.Err()
}

// --- Category operations ---

// UpsertCategoryGroup inserts or replaces a category group in the database.
func (s *YnabStore) UpsertCategoryGroup(ctx context.Context, group ynab.CategoryGroup) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Check if group exists
	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM category_groups WHERE id = ?", group.ID).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("check category group existence: %w", err)
	}

	// Upsert category group
	query := `
		INSERT OR REPLACE INTO category_groups (
			id, budget_id, name, hidden, deleted
		) VALUES (?, ?, ?, ?, ?)
	`

	_, err = tx.ExecContext(ctx, query,
		group.ID,
		group.BudgetID,
		group.Name,
		boolToInt(group.Hidden),
		boolToInt(group.Deleted),
	)
	if err != nil {
		return fmt.Errorf("upsert category group: %w", err)
	}

	if exists {
		slog.Debug("Category group found, replacing", "name", group.Name)
	} else {
		slog.Debug("Category group not found, inserting", "name", group.Name)
	}

	// Delete existing categories for this group
	_, err = tx.ExecContext(ctx, "DELETE FROM categories WHERE category_group_id = ?", group.ID)
	if err != nil {
		return fmt.Errorf("delete old categories: %w", err)
	}

	// Insert categories
	for _, cat := range group.Categories {
		err = s.insertCategory(ctx, tx, group.BudgetID, group.ID, group.Name, cat)
		if err != nil {
			return fmt.Errorf("insert category: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// insertCategory inserts a category within a transaction.
func (s *YnabStore) insertCategory(ctx context.Context, tx *sql.Tx, budgetID, groupID, groupName string, cat ynab.Category) error {
	query := `
		INSERT INTO categories (
			id, budget_id, category_group_id, category_group_name,
			name, hidden, deleted, original_category_group_id, note,
			budgeted, activity, balance, goal_type, goal_days,
			goal_cadence, goal_cadence_frequency, goal_target,
			goal_target_month, goal_creation_month, goal_percentage_complete,
			goal_months_to_budget, goal_under_funded, goal_overall_funded,
			goal_overall_left
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := tx.ExecContext(ctx, query,
		cat.ID,
		budgetID,
		groupID,
		groupName,
		cat.Name,
		boolToInt(cat.Hidden),
		boolToInt(cat.Deleted),
		cat.OriginalCategoryGroupId,
		cat.Note,
		cat.Budgeted,
		cat.Activity,
		cat.Balance,
		cat.GoalType,
		cat.GoalDays,
		cat.GoalCadence,
		cat.GoalCadenceFrequency,
		cat.GoalTarget,
		cat.GoalTargetMonth,
		cat.GoalCreationMonth,
		cat.GoalPercentageComplete,
		cat.GoalMonthsToBudget,
		cat.GoalUnderFunded,
		cat.GoalOverallFunded,
		cat.GoalOverallLeft,
	)
	return err
}

// FetchCategoriesByBudget returns category groups for a specific budget.
func (s *YnabStore) FetchCategoriesByBudget(ctx context.Context, budgetID string) ([]ynab.CategoryGroup, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Fetch category groups
	groupQuery := `
		SELECT id, budget_id, name, hidden, deleted
		FROM category_groups
		WHERE budget_id = ? AND deleted = 0
	`

	rows, err := s.db.QueryContext(ctx, groupQuery, budgetID)
	if err != nil {
		return nil, fmt.Errorf("query category groups: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var groups []ynab.CategoryGroup
	for rows.Next() {
		var group ynab.CategoryGroup
		var hidden, deleted int

		err := rows.Scan(
			&group.ID,
			&group.BudgetID,
			&group.Name,
			&hidden,
			&deleted,
		)
		if err != nil {
			return nil, fmt.Errorf("scan category group: %w", err)
		}

		group.Hidden = intToBool(hidden)
		group.Deleted = intToBool(deleted)

		// Fetch categories for this group
		categories, err := s.fetchCategoriesByGroup(ctx, group.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch categories: %w", err)
		}
		group.Categories = categories

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// fetchCategoriesByGroup fetches all categories for a category group.
func (s *YnabStore) fetchCategoriesByGroup(ctx context.Context, groupID string) ([]ynab.Category, error) {
	query := `
		SELECT id, category_group_id, category_group_name, name,
		       hidden, deleted, original_category_group_id, note,
		       budgeted, activity, balance, goal_type, goal_days,
		       goal_cadence, goal_cadence_frequency, goal_target,
		       goal_target_month, goal_creation_month, goal_percentage_complete,
		       goal_months_to_budget, goal_under_funded, goal_overall_funded,
		       goal_overall_left
		FROM categories
		WHERE category_group_id = ?
	`

	rows, err := s.db.QueryContext(ctx, query, groupID)
	if err != nil {
		return nil, fmt.Errorf("query categories: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var categories []ynab.Category
	for rows.Next() {
		var cat ynab.Category
		var hidden, deleted int

		err := rows.Scan(
			&cat.ID,
			&cat.CategoryGroupID,
			&cat.CategoryGroupName,
			&cat.Name,
			&hidden,
			&deleted,
			&cat.OriginalCategoryGroupId,
			&cat.Note,
			&cat.Budgeted,
			&cat.Activity,
			&cat.Balance,
			&cat.GoalType,
			&cat.GoalDays,
			&cat.GoalCadence,
			&cat.GoalCadenceFrequency,
			&cat.GoalTarget,
			&cat.GoalTargetMonth,
			&cat.GoalCreationMonth,
			&cat.GoalPercentageComplete,
			&cat.GoalMonthsToBudget,
			&cat.GoalUnderFunded,
			&cat.GoalOverallFunded,
			&cat.GoalOverallLeft,
		)
		if err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}

		cat.Hidden = intToBool(hidden)
		cat.Deleted = intToBool(deleted)

		categories = append(categories, cat)
	}

	return categories, rows.Err()
}

// --- Payee operations ---

// UpsertPayee inserts or replaces a payee in the database.
func (s *YnabStore) UpsertPayee(ctx context.Context, payee ynab.Payee) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check if payee exists
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM payees WHERE id = ?", payee.ID).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("check payee existence: %w", err)
	}

	query := `
		INSERT OR REPLACE INTO payees (
			id, budget_id, name, transfer_account_id, deleted, last_category_id
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		payee.ID,
		payee.BudgetID,
		payee.Name,
		payee.TransferAccountId,
		boolToInt(payee.Deleted),
		payee.LastCategoryID,
	)
	if err != nil {
		return fmt.Errorf("upsert payee: %w", err)
	}

	if exists {
		slog.Debug("Payee found, replacing", "name", payee.Name)
	} else {
		slog.Debug("Payee not found, inserting", "name", payee.Name)
	}

	return nil
}

// FetchPayeesByBudget returns payees for a specific budget.
func (s *YnabStore) FetchPayeesByBudget(ctx context.Context, budgetID string) ([]ynab.Payee, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `
		SELECT id, budget_id, name, transfer_account_id, deleted, last_category_id
		FROM payees
		WHERE budget_id = ? AND deleted = 0
	`

	rows, err := s.db.QueryContext(ctx, query, budgetID)
	if err != nil {
		return nil, fmt.Errorf("query payees: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var payees []ynab.Payee
	for rows.Next() {
		var payee ynab.Payee
		var deleted int

		err := rows.Scan(
			&payee.ID,
			&payee.BudgetID,
			&payee.Name,
			&payee.TransferAccountId,
			&deleted,
			&payee.LastCategoryID,
		)
		if err != nil {
			return nil, fmt.Errorf("scan payee: %w", err)
		}

		payee.Deleted = intToBool(deleted)
		payees = append(payees, payee)
	}

	return payees, rows.Err()
}

// UpdatePayeeLastCategory updates the last used category for a payee.
func (s *YnabStore) UpdatePayeeLastCategory(ctx context.Context, payeeID, categoryID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `UPDATE payees SET last_category_id = ? WHERE id = ?`

	_, err := s.db.ExecContext(ctx, query, categoryID, payeeID)
	if err != nil {
		return fmt.Errorf("update payee last category: %w", err)
	}

	return nil
}

// --- Sync History operations ---

// UpsertSyncHistory inserts or updates a sync history record.
func (s *YnabStore) UpsertSyncHistory(ctx context.Context, h ynab.SyncHistory) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Build filter for finding existing record
	findQuery := "SELECT id FROM sync_history WHERE name = ?"
	args := []interface{}{h.Name}

	if h.BudgetID != "" {
		findQuery += " AND budget_id = ?"
		args = append(args, h.BudgetID)
	} else {
		findQuery += " AND budget_id IS NULL"
	}

	var existingID int64
	err := s.db.QueryRowContext(ctx, findQuery, args...).Scan(&existingID)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("find sync history: %w", err)
	}

	if err == sql.ErrNoRows {
		// Insert new record
		slog.Debug("History record not found, inserting", "name", h.Name)

		query := `
			INSERT INTO sync_history (
				name, budget_id, status, updated_at,
				last_known_version, added_items, message
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`

		budgetID := sql.NullString{String: h.BudgetID, Valid: h.BudgetID != ""}

		result, err := s.db.ExecContext(ctx, query,
			h.Name,
			budgetID,
			h.Status,
			h.UpdatedAt.Format(time.RFC3339),
			h.LastKnownVersion,
			h.AddedItems,
			h.Message,
		)
		if err != nil {
			return fmt.Errorf("insert sync history: %w", err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("get last insert id: %w", err)
		}

		h.ID = strconv.FormatInt(id, 10)
	} else {
		// Update existing record
		slog.Debug("History record found, replacing", "name", h.Name)

		query := `
			UPDATE sync_history
			SET status = ?, updated_at = ?, last_known_version = ?,
			    added_items = ?, message = ?
			WHERE id = ?
		`

		_, err = s.db.ExecContext(ctx, query,
			h.Status,
			h.UpdatedAt.Format(time.RFC3339),
			h.LastKnownVersion,
			h.AddedItems,
			h.Message,
			existingID,
		)
		if err != nil {
			return fmt.Errorf("update sync history: %w", err)
		}

		h.ID = strconv.FormatInt(existingID, 10)
	}

	return nil
}

// FetchAllSyncHistory returns all sync history records from the database.
func (s *YnabStore) FetchAllSyncHistory(ctx context.Context) ([]ynab.SyncHistory, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `
		SELECT id, name, budget_id, status, updated_at,
		       last_known_version, added_items, message
		FROM sync_history
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query sync history: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var history []ynab.SyncHistory
	for rows.Next() {
		h, err := s.scanSyncHistory(rows)
		if err != nil {
			return nil, err
		}
		history = append(history, h)
	}

	slog.Info("fetched sync history from database", "count", len(history))
	return history, rows.Err()
}

// FindSyncHistoryByBudget returns sync history for a specific budget.
func (s *YnabStore) FindSyncHistoryByBudget(ctx context.Context, budgetID string) ([]ynab.SyncHistory, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `
		SELECT id, name, budget_id, status, updated_at,
		       last_known_version, added_items, message
		FROM sync_history
		WHERE budget_id = ?
	`

	rows, err := s.db.QueryContext(ctx, query, budgetID)
	if err != nil {
		return nil, fmt.Errorf("query sync history: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var history []ynab.SyncHistory
	for rows.Next() {
		h, err := s.scanSyncHistory(rows)
		if err != nil {
			return nil, err
		}
		history = append(history, h)
	}

	slog.Info("fetched sync history from database", "count", len(history), "budget_id", budgetID)
	return history, rows.Err()
}

// scanSyncHistory scans a sync history record from a row.
func (s *YnabStore) scanSyncHistory(rows *sql.Rows) (ynab.SyncHistory, error) {
	var h ynab.SyncHistory
	var updatedAt string
	var budgetID sql.NullString

	err := rows.Scan(
		&h.ID,
		&h.Name,
		&budgetID,
		&h.Status,
		&updatedAt,
		&h.LastKnownVersion,
		&h.AddedItems,
		&h.Message,
	)
	if err != nil {
		return ynab.SyncHistory{}, fmt.Errorf("scan sync history: %w", err)
	}

	if budgetID.Valid {
		h.BudgetID = budgetID.String
	}

	h.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return ynab.SyncHistory{}, fmt.Errorf("parse updated_at: %w", err)
	}

	return h, nil
}

// --- Helper functions ---

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool {
	return i != 0
}
