package ynab

import (
	"context"
	"errors"
	"testing"
)

// Mock implementations

type mockYnabClient struct {
	fetchBudgetsFunc    func() ([]Budget, error)
	fetchAccountsFunc   func(req SyncReq) (AccountData, error)
	fetchCategoriesFunc func(req SyncReq) (CategoryData, error)
	fetchPayeesFunc     func(req SyncReq) (PayeeData, error)
	uploadFunc          func(txn TxnReq) error
}

func (m *mockYnabClient) FetchBudgets() ([]Budget, error) {
	if m.fetchBudgetsFunc != nil {
		return m.fetchBudgetsFunc()
	}
	return []Budget{}, nil
}

func (m *mockYnabClient) FetchAccounts(req SyncReq) (AccountData, error) {
	if m.fetchAccountsFunc != nil {
		return m.fetchAccountsFunc(req)
	}
	return AccountData{}, nil
}

func (m *mockYnabClient) FetchCategories(req SyncReq) (CategoryData, error) {
	if m.fetchCategoriesFunc != nil {
		return m.fetchCategoriesFunc(req)
	}
	return CategoryData{}, nil
}

func (m *mockYnabClient) FetchPayees(req SyncReq) (PayeeData, error) {
	if m.fetchPayeesFunc != nil {
		return m.fetchPayeesFunc(req)
	}
	return PayeeData{}, nil
}

func (m *mockYnabClient) Upload(txn TxnReq) error {
	if m.uploadFunc != nil {
		return m.uploadFunc(txn)
	}
	return nil
}

type mockBudgetStore struct {
	upsertFunc      func(ctx context.Context, budget Budget) error
	fetchAllFunc    func(ctx context.Context) ([]Budget, error)
	findByIDFunc    func(ctx context.Context, id string) (Budget, error)
	findByAccIDFunc func(ctx context.Context, accID string) (Budget, error)
	budgets         []Budget
}

func (m *mockBudgetStore) UpsertBudget(ctx context.Context, budget Budget) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, budget)
	}
	m.budgets = append(m.budgets, budget)
	return nil
}

func (m *mockBudgetStore) FetchAllBudgets(ctx context.Context) ([]Budget, error) {
	if m.fetchAllFunc != nil {
		return m.fetchAllFunc(ctx)
	}
	return m.budgets, nil
}

func (m *mockBudgetStore) FindBudgetByID(ctx context.Context, id string) (Budget, error) {
	if m.findByIDFunc != nil {
		return m.findByIDFunc(ctx, id)
	}
	for _, b := range m.budgets {
		if b.ID == id {
			return b, nil
		}
	}
	return Budget{}, errors.New("budget not found")
}

func (m *mockBudgetStore) FindBudgetByAccountID(ctx context.Context, accID string) (Budget, error) {
	if m.findByAccIDFunc != nil {
		return m.findByAccIDFunc(ctx, accID)
	}
	for _, b := range m.budgets {
		for _, acc := range b.Accounts {
			if acc.ID == accID {
				return b, nil
			}
		}
	}
	return Budget{}, errors.New("budget not found")
}

type mockAccountStore struct {
	upsertFunc   func(ctx context.Context, acc Account) error
	fetchAllFunc func(ctx context.Context) ([]Account, error)
	accounts     []Account
}

func (m *mockAccountStore) UpsertAccount(ctx context.Context, acc Account) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, acc)
	}
	m.accounts = append(m.accounts, acc)
	return nil
}

func (m *mockAccountStore) FetchAllAccounts(ctx context.Context) ([]Account, error) {
	if m.fetchAllFunc != nil {
		return m.fetchAllFunc(ctx)
	}
	return m.accounts, nil
}

type mockCategoryStore struct {
	upsertFunc        func(ctx context.Context, group CategoryGroup) error
	fetchByBudgetFunc func(ctx context.Context, budgetID string) ([]CategoryGroup, error)
	categories        []CategoryGroup
}

func (m *mockCategoryStore) UpsertCategoryGroup(ctx context.Context, group CategoryGroup) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, group)
	}
	m.categories = append(m.categories, group)
	return nil
}

func (m *mockCategoryStore) FetchCategoriesByBudget(ctx context.Context, budgetID string) ([]CategoryGroup, error) {
	if m.fetchByBudgetFunc != nil {
		return m.fetchByBudgetFunc(ctx, budgetID)
	}
	var result []CategoryGroup
	for _, c := range m.categories {
		if c.BudgetID == budgetID {
			result = append(result, c)
		}
	}
	return result, nil
}

type mockPayeeStore struct {
	upsertFunc             func(ctx context.Context, payee Payee) error
	fetchByBudgetFunc      func(ctx context.Context, budgetID string) ([]Payee, error)
	updateLastCategoryFunc func(ctx context.Context, payeeID, categoryID string) error
	payees                 []Payee
}

func (m *mockPayeeStore) UpsertPayee(ctx context.Context, payee Payee) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, payee)
	}
	m.payees = append(m.payees, payee)
	return nil
}

func (m *mockPayeeStore) FetchPayeesByBudget(ctx context.Context, budgetID string) ([]Payee, error) {
	if m.fetchByBudgetFunc != nil {
		return m.fetchByBudgetFunc(ctx, budgetID)
	}
	var result []Payee
	for _, p := range m.payees {
		if p.BudgetID == budgetID {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockPayeeStore) UpdatePayeeLastCategory(ctx context.Context, payeeID, categoryID string) error {
	if m.updateLastCategoryFunc != nil {
		return m.updateLastCategoryFunc(ctx, payeeID, categoryID)
	}
	return nil
}

type mockHistoryStore struct {
	upsertFunc       func(ctx context.Context, h SyncHistory) error
	fetchAllFunc     func(ctx context.Context) ([]SyncHistory, error)
	findByBudgetFunc func(ctx context.Context, budgetID string) ([]SyncHistory, error)
	history          []SyncHistory
}

func (m *mockHistoryStore) UpsertSyncHistory(ctx context.Context, h SyncHistory) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, h)
	}
	m.history = append(m.history, h)
	return nil
}

func (m *mockHistoryStore) FetchAllSyncHistory(ctx context.Context) ([]SyncHistory, error) {
	if m.fetchAllFunc != nil {
		return m.fetchAllFunc(ctx)
	}
	return m.history, nil
}

func (m *mockHistoryStore) FindSyncHistoryByBudget(ctx context.Context, budgetID string) ([]SyncHistory, error) {
	if m.findByBudgetFunc != nil {
		return m.findByBudgetFunc(ctx, budgetID)
	}
	var result []SyncHistory
	for _, h := range m.history {
		if h.BudgetID == budgetID {
			result = append(result, h)
		}
	}
	return result, nil
}

// Tests

func TestSyncer_SyncBudgets(t *testing.T) {
	ctx := context.Background()

	t.Run("Successful sync", func(t *testing.T) {
		client := &mockYnabClient{
			fetchBudgetsFunc: func() ([]Budget, error) {
				return []Budget{
					{ID: "b1", Name: "Personal Budget"},
					{ID: "b2", Name: "Business Budget"},
				}, nil
			},
		}

		budgetStore := &mockBudgetStore{}
		historyStore := &mockHistoryStore{}

		syncer := NewSyncer(client, budgetStore, nil, nil, nil, historyStore)

		err := syncer.SyncBudgets(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(budgetStore.budgets) != 2 {
			t.Errorf("Expected 2 budgets stored, got %d", len(budgetStore.budgets))
		}

		if len(historyStore.history) != 1 {
			t.Fatalf("Expected 1 history entry, got %d", len(historyStore.history))
		}

		history := historyStore.history[0]
		if history.Name != "budgets" {
			t.Errorf("Expected history name 'budgets', got '%s'", history.Name)
		}
		if history.Status != "success" {
			t.Errorf("Expected status 'success', got '%s'", history.Status)
		}
		if history.AddedItems != 2 {
			t.Errorf("Expected 2 added items, got %d", history.AddedItems)
		}
	})

	t.Run("Client fetch error", func(t *testing.T) {
		client := &mockYnabClient{
			fetchBudgetsFunc: func() ([]Budget, error) {
				return nil, errors.New("API error")
			},
		}

		budgetStore := &mockBudgetStore{}
		historyStore := &mockHistoryStore{}

		syncer := NewSyncer(client, budgetStore, nil, nil, nil, historyStore)

		err := syncer.SyncBudgets(ctx)
		if err == nil {
			t.Error("Expected error from failed API call")
		}

		if len(historyStore.history) != 1 {
			t.Fatalf("Expected 1 history entry for failed sync, got %d", len(historyStore.history))
		}

		if historyStore.history[0].Status != "failed" {
			t.Errorf("Expected failed status in history, got '%s'", historyStore.history[0].Status)
		}
	})

	t.Run("Store upsert error", func(t *testing.T) {
		client := &mockYnabClient{
			fetchBudgetsFunc: func() ([]Budget, error) {
				return []Budget{
					{ID: "b1", Name: "Test Budget"},
				}, nil
			},
		}

		budgetStore := &mockBudgetStore{
			upsertFunc: func(ctx context.Context, budget Budget) error {
				return errors.New("database error")
			},
		}
		historyStore := &mockHistoryStore{}

		syncer := NewSyncer(client, budgetStore, nil, nil, nil, historyStore)

		err := syncer.SyncBudgets(ctx)
		if err == nil {
			t.Error("Expected error from failed upsert")
		}

		if len(historyStore.history) != 1 {
			t.Fatalf("Expected 1 history entry for failed sync, got %d", len(historyStore.history))
		}

		if historyStore.history[0].Status != "failed" {
			t.Errorf("Expected failed status in history, got '%s'", historyStore.history[0].Status)
		}
	})
}

func TestSyncer_SyncAccounts(t *testing.T) {
	ctx := context.Background()
	budgetID := "budget-123"

	t.Run("Successful sync", func(t *testing.T) {
		client := &mockYnabClient{
			fetchAccountsFunc: func(req SyncReq) (AccountData, error) {
				return AccountData{
					Accounts: []Account{
						{ID: "acc1", Name: "Checking"},
						{ID: "acc2", Name: "Savings"},
					},
					ServerKnowledge: 150,
				}, nil
			},
		}

		accountStore := &mockAccountStore{}
		historyStore := &mockHistoryStore{}

		syncer := NewSyncer(client, nil, accountStore, nil, nil, historyStore)

		err := syncer.SyncAccounts(ctx, budgetID)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(accountStore.accounts) != 2 {
			t.Errorf("Expected 2 accounts stored, got %d", len(accountStore.accounts))
		}

		if len(historyStore.history) != 1 {
			t.Fatalf("Expected 1 history entry, got %d", len(historyStore.history))
		}

		history := historyStore.history[0]
		if history.LastKnownVersion != 150 {
			t.Errorf("Expected version 150, got %d", history.LastKnownVersion)
		}
		if history.BudgetID != budgetID {
			t.Errorf("Expected budget ID '%s', got '%s'", budgetID, history.BudgetID)
		}
	})

	t.Run("Client fetch error", func(t *testing.T) {
		client := &mockYnabClient{
			fetchAccountsFunc: func(req SyncReq) (AccountData, error) {
				return AccountData{}, errors.New("API error")
			},
		}

		historyStore := &mockHistoryStore{}
		syncer := NewSyncer(client, nil, &mockAccountStore{}, nil, nil, historyStore)

		err := syncer.SyncAccounts(ctx, budgetID)
		if err == nil {
			t.Error("Expected error from failed API call")
		}
	})
}

func TestSyncer_SyncCategories(t *testing.T) {
	ctx := context.Background()
	budgetID := "budget-123"

	t.Run("Successful sync", func(t *testing.T) {
		client := &mockYnabClient{
			fetchCategoriesFunc: func(req SyncReq) (CategoryData, error) {
				return CategoryData{
					Categories: []CategoryGroup{
						{ID: "cat1", Name: "Food"},
						{ID: "cat2", Name: "Transport"},
					},
					ServerKnowledge: 200,
				}, nil
			},
		}

		categoryStore := &mockCategoryStore{}
		historyStore := &mockHistoryStore{}

		syncer := NewSyncer(client, nil, nil, categoryStore, nil, historyStore)

		err := syncer.SyncCategories(ctx, budgetID)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(categoryStore.categories) != 2 {
			t.Errorf("Expected 2 categories stored, got %d", len(categoryStore.categories))
		}

		// Verify budget ID was set
		for _, cat := range categoryStore.categories {
			if cat.BudgetID != budgetID {
				t.Errorf("Expected budget ID '%s', got '%s'", budgetID, cat.BudgetID)
			}
		}
	})
}

func TestSyncer_SyncPayees(t *testing.T) {
	ctx := context.Background()
	budgetID := "budget-123"

	t.Run("Successful sync", func(t *testing.T) {
		client := &mockYnabClient{
			fetchPayeesFunc: func(req SyncReq) (PayeeData, error) {
				return PayeeData{
					Payees: []Payee{
						{ID: "p1", Name: "Amazon"},
						{ID: "p2", Name: "Starbucks"},
					},
					ServerKnowledge: 250,
				}, nil
			},
		}

		payeeStore := &mockPayeeStore{}
		historyStore := &mockHistoryStore{}

		syncer := NewSyncer(client, nil, nil, nil, payeeStore, historyStore)

		err := syncer.SyncPayees(ctx, budgetID)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(payeeStore.payees) != 2 {
			t.Errorf("Expected 2 payees stored, got %d", len(payeeStore.payees))
		}

		// Verify budget ID was set
		for _, payee := range payeeStore.payees {
			if payee.BudgetID != budgetID {
				t.Errorf("Expected budget ID '%s', got '%s'", budgetID, payee.BudgetID)
			}
		}
	})
}

func TestSyncer_FetchBudgets(t *testing.T) {
	ctx := context.Background()

	budgetStore := &mockBudgetStore{
		budgets: []Budget{
			{ID: "b1", Name: "Budget 1"},
			{ID: "b2", Name: "Budget 2"},
		},
	}

	syncer := NewSyncer(nil, budgetStore, nil, nil, nil, nil)

	budgets, err := syncer.FetchBudgets(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(budgets) != 2 {
		t.Errorf("Expected 2 budgets, got %d", len(budgets))
	}
}

func TestSyncer_FetchPayeesByBudget(t *testing.T) {
	ctx := context.Background()
	budgetID := "budget-123"

	payeeStore := &mockPayeeStore{
		payees: []Payee{
			{ID: "p1", Name: "Payee 1", BudgetID: budgetID},
			{ID: "p2", Name: "Payee 2", BudgetID: budgetID},
			{ID: "p3", Name: "Payee 3", BudgetID: "other-budget"},
		},
	}

	syncer := NewSyncer(nil, nil, nil, nil, payeeStore, nil)

	payees, err := syncer.FetchPayeesByBudget(ctx, budgetID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(payees) != 2 {
		t.Errorf("Expected 2 payees for budget, got %d", len(payees))
	}
}

func TestSyncHistory_SaveFailure(t *testing.T) {
	ctx := context.Background()

	// Create a client that returns an error
	client := &mockYnabClient{
		fetchBudgetsFunc: func() ([]Budget, error) {
			return nil, errors.New("API connection failed")
		},
	}

	historyStore := &mockHistoryStore{}
	syncer := NewSyncer(client, nil, nil, nil, nil, historyStore)

	// Trigger a failure
	err := syncer.SyncBudgets(ctx)
	if err == nil {
		t.Error("Expected error from failed API call")
	}

	if len(historyStore.history) != 1 {
		t.Fatalf("Expected 1 history entry for failure, got %d", len(historyStore.history))
	}

	history := historyStore.history[0]
	if history.Status != "failed" {
		t.Errorf("Expected status 'failed', got '%s'", history.Status)
	}
	if history.Message == "" {
		t.Error("Expected error message in history")
	}
}

func TestNewSyncer(t *testing.T) {
	client := &mockYnabClient{}
	budgetStore := &mockBudgetStore{}
	accountStore := &mockAccountStore{}
	categoryStore := &mockCategoryStore{}
	payeeStore := &mockPayeeStore{}
	historyStore := &mockHistoryStore{}

	syncer := NewSyncer(client, budgetStore, accountStore, categoryStore, payeeStore, historyStore)

	if syncer == nil {
		t.Fatal("Expected non-nil syncer")
	}
	if syncer.client == nil {
		t.Error("Expected client to be set")
	}
	if syncer.budgetStore == nil {
		t.Error("Expected budgetStore to be set")
	}
	if syncer.accountStore == nil {
		t.Error("Expected accountStore to be set")
	}
	if syncer.categoryStore == nil {
		t.Error("Expected categoryStore to be set")
	}
	if syncer.payeeStore == nil {
		t.Error("Expected payeeStore to be set")
	}
	if syncer.historyStore == nil {
		t.Error("Expected historyStore to be set")
	}
}
