package ynab

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// BudgetStorer defines storage operations for budgets.
type BudgetStorer interface {
	UpsertBudget(ctx context.Context, budget Budget) error
	FetchAllBudgets(ctx context.Context) ([]Budget, error)
	FindBudgetByID(ctx context.Context, id string) (Budget, error)
	FindBudgetByAccountID(ctx context.Context, accID string) (Budget, error)
}

// AccountStorer defines storage operations for accounts.
type AccountStorer interface {
	UpsertAccount(ctx context.Context, acc Account) error
	FetchAllAccounts(ctx context.Context) ([]Account, error)
}

// CategoryStorer defines storage operations for category groups.
type CategoryStorer interface {
	UpsertCategoryGroup(ctx context.Context, group CategoryGroup) error
	FetchCategoriesByBudget(ctx context.Context, budgetID string) ([]CategoryGroup, error)
}

// PayeeStorer defines storage operations for payees.
type PayeeStorer interface {
	UpsertPayee(ctx context.Context, payee Payee) error
	FetchPayeesByBudget(ctx context.Context, budgetID string) ([]Payee, error)
	UpdatePayeeLastCategory(ctx context.Context, payeeID, categoryID string) error
}

// HistoryStorer defines storage operations for sync history.
type HistoryStorer interface {
	UpsertSyncHistory(ctx context.Context, h SyncHistory) error
	FetchAllSyncHistory(ctx context.Context) ([]SyncHistory, error)
	FindSyncHistoryByBudget(ctx context.Context, budgetID string) ([]SyncHistory, error)
}

// YnabClient defines YNAB API operations.
type YnabClient interface {
	FetchBudgets() ([]Budget, error)
	FetchAccounts(req SyncReq) (AccountData, error)
	FetchCategories(req SyncReq) (CategoryData, error)
	FetchPayees(req SyncReq) (PayeeData, error)
	Upload(txn TxnReq) error
}

// Syncer handles all YNAB sync operations.
type Syncer struct {
	client        YnabClient
	budgetStore   BudgetStorer
	accountStore  AccountStorer
	categoryStore CategoryStorer
	payeeStore    PayeeStorer
	historyStore  HistoryStorer
}

// NewSyncer creates a new Syncer.
func NewSyncer(client YnabClient, budgetStore BudgetStorer, accountStore AccountStorer, categoryStore CategoryStorer, payeeStore PayeeStorer, historyStore HistoryStorer) *Syncer {
	return &Syncer{
		client:        client,
		budgetStore:   budgetStore,
		accountStore:  accountStore,
		categoryStore: categoryStore,
		payeeStore:    payeeStore,
		historyStore:  historyStore,
	}
}

// SyncBudgets syncs budgets from YNAB and stores them in the database.
func (s *Syncer) SyncBudgets(ctx context.Context) error {
	budgets, err := s.client.FetchBudgets()
	if err != nil {
		s.saveFailedHistory(ctx, "budgets", "", err)
		return fmt.Errorf("fetching budgets: %w", err)
	}

	var errs []string
	for _, budget := range budgets {
		if err := s.budgetStore.UpsertBudget(ctx, budget); err != nil {
			errs = append(errs, fmt.Sprintf("budget %s (%s)", budget.Name, budget.ID))
		}
	}

	if len(errs) > 0 {
		err := fmt.Errorf("failed to insert: %s", strings.Join(errs, ", "))
		s.saveFailedHistory(ctx, "budgets", "", err)
		return err
	}

	slog.Info("synced budgets", "count", len(budgets))
	return s.historyStore.UpsertSyncHistory(ctx, SyncHistory{
		Name:       "budgets",
		Status:     "success",
		UpdatedAt:  time.Now(),
		AddedItems: len(budgets),
	})
}

// SyncAccounts syncs accounts for a budget from YNAB.
func (s *Syncer) SyncAccounts(ctx context.Context, budgetID string) error {
	data, err := s.client.FetchAccounts(SyncReq{BudgetID: budgetID})
	if err != nil {
		s.saveFailedHistory(ctx, "accounts", budgetID, err)
		return fmt.Errorf("fetching accounts: %w", err)
	}

	var errs []string
	for _, acc := range data.Accounts {
		if err := s.accountStore.UpsertAccount(ctx, acc); err != nil {
			errs = append(errs, fmt.Sprintf("account %s (%s)", acc.Name, acc.ID))
		}
	}

	if len(errs) > 0 {
		err := fmt.Errorf("failed to insert: %s", strings.Join(errs, ", "))
		s.saveFailedHistory(ctx, "accounts", budgetID, err)
		return err
	}

	slog.Info("synced accounts", "count", len(data.Accounts), "budget_id", budgetID)
	return s.historyStore.UpsertSyncHistory(ctx, SyncHistory{
		Name:             "accounts",
		Status:           "success",
		UpdatedAt:        time.Now(),
		AddedItems:       len(data.Accounts),
		LastKnownVersion: data.ServerKnowledge,
		BudgetID:         budgetID,
	})
}

// SyncCategories syncs categories for a budget from YNAB.
func (s *Syncer) SyncCategories(ctx context.Context, budgetID string) error {
	data, err := s.client.FetchCategories(SyncReq{BudgetID: budgetID})
	if err != nil {
		s.saveFailedHistory(ctx, "categories", budgetID, err)
		return fmt.Errorf("fetching categories: %w", err)
	}

	var errs []string
	for _, cat := range data.Categories {
		cat.BudgetID = budgetID
		if err := s.categoryStore.UpsertCategoryGroup(ctx, cat); err != nil {
			errs = append(errs, fmt.Sprintf("category %s (%s)", cat.Name, cat.ID))
		}
	}

	if len(errs) > 0 {
		err := fmt.Errorf("failed to insert: %s", strings.Join(errs, ", "))
		s.saveFailedHistory(ctx, "categories", budgetID, err)
		return err
	}

	slog.Info("synced categories", "count", len(data.Categories), "budget_id", budgetID)
	return s.historyStore.UpsertSyncHistory(ctx, SyncHistory{
		Name:             "categories",
		Status:           "success",
		UpdatedAt:        time.Now(),
		AddedItems:       len(data.Categories),
		LastKnownVersion: data.ServerKnowledge,
		BudgetID:         budgetID,
	})
}

// SyncPayees syncs payees for a budget from YNAB.
func (s *Syncer) SyncPayees(ctx context.Context, budgetID string) error {
	data, err := s.client.FetchPayees(SyncReq{BudgetID: budgetID})
	if err != nil {
		s.saveFailedHistory(ctx, "payees", budgetID, err)
		return fmt.Errorf("fetching payees: %w", err)
	}

	var errs []string
	for _, payee := range data.Payees {
		payee.BudgetID = budgetID
		if err := s.payeeStore.UpsertPayee(ctx, payee); err != nil {
			errs = append(errs, fmt.Sprintf("payee %s (%s)", payee.Name, payee.ID))
		}
	}

	if len(errs) > 0 {
		err := fmt.Errorf("failed to insert: %s", strings.Join(errs, ", "))
		s.saveFailedHistory(ctx, "payees", budgetID, err)
		return err
	}

	slog.Info("synced payees", "count", len(data.Payees), "budget_id", budgetID)
	return s.historyStore.UpsertSyncHistory(ctx, SyncHistory{
		Name:             "payees",
		Status:           "success",
		UpdatedAt:        time.Now(),
		AddedItems:       len(data.Payees),
		LastKnownVersion: data.ServerKnowledge,
		BudgetID:         budgetID,
	})
}

// FetchBudgets returns all budgets from the database.
func (s *Syncer) FetchBudgets(ctx context.Context) ([]Budget, error) {
	return s.budgetStore.FetchAllBudgets(ctx)
}

// FindBudgetByID finds a budget by ID.
func (s *Syncer) FindBudgetByID(ctx context.Context, id string) (Budget, error) {
	return s.budgetStore.FindBudgetByID(ctx, id)
}

// FindBudgetByAccID finds a budget by account ID.
func (s *Syncer) FindBudgetByAccID(ctx context.Context, accID string) (Budget, error) {
	return s.budgetStore.FindBudgetByAccountID(ctx, accID)
}

// FetchCategoriesByBudget returns sorted categories for a budget.
func (s *Syncer) FetchCategoriesByBudget(ctx context.Context, budgetID string) ([]Category, error) {
	groups, err := s.categoryStore.FetchCategoriesByBudget(ctx, budgetID)
	if err != nil {
		return nil, fmt.Errorf("fetching categories: %w", err)
	}

	var categories []Category
	for _, gr := range groups {
		for _, c := range gr.Categories {
			if c.Deleted || c.Hidden {
				continue
			}
			categories = append(categories, c)
		}
	}

	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Name < categories[j].Name
	})

	return categories, nil
}

// FetchPayeesByBudget returns sorted payees for a budget.
func (s *Syncer) FetchPayeesByBudget(ctx context.Context, budgetID string) ([]Payee, error) {
	payees, err := s.payeeStore.FetchPayeesByBudget(ctx, budgetID)
	if err != nil {
		return nil, fmt.Errorf("fetching payees: %w", err)
	}

	sort.Slice(payees, func(i, j int) bool {
		return payees[i].Name < payees[j].Name
	})

	return payees, nil
}

// UpdatePayeeLastCategory updates the last used category for a payee.
func (s *Syncer) UpdatePayeeLastCategory(ctx context.Context, payeeID, categoryID string) error {
	return s.payeeStore.UpdatePayeeLastCategory(ctx, payeeID, categoryID)
}

// FetchHistory returns all sync history.
func (s *Syncer) FetchHistory(ctx context.Context) ([]SyncHistory, error) {
	return s.historyStore.FetchAllSyncHistory(ctx)
}

// FindHistoryByBudget returns sync history for a specific budget.
func (s *Syncer) FindHistoryByBudget(ctx context.Context, budgetID string) ([]SyncHistory, error) {
	return s.historyStore.FindSyncHistoryByBudget(ctx, budgetID)
}

// UploadTransaction uploads a transaction to YNAB.
func (s *Syncer) UploadTransaction(txn TxnReq) error {
	return s.client.Upload(txn)
}

func (s *Syncer) saveFailedHistory(ctx context.Context, name, budgetID string, err error) {
	history := SyncHistory{
		Name:      name,
		Status:    "failed",
		UpdatedAt: time.Now(),
		Message:   err.Error(),
		BudgetID:  budgetID,
	}
	if insertErr := s.historyStore.UpsertSyncHistory(ctx, history); insertErr != nil {
		slog.Error("failed to save sync history", "error", insertErr)
	}
}
