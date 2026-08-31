package ynab

import (
	"fmt"
	"log/slog"
)

// FakeClient is an in-memory YnabClient implementation backed by a Fixture.
// It's used for local test mode so the app can be exercised end-to-end
// without ever calling the real YNAB API.
type FakeClient struct {
	fixture Fixture
}

// NewFakeClient creates a new FakeClient serving data from fixture.
func NewFakeClient(fixture Fixture) *FakeClient {
	return &FakeClient{fixture: fixture}
}

// FetchBudgets returns the fixture's budgets.
func (c *FakeClient) FetchBudgets() ([]Budget, error) {
	slog.Info("fetching budgets from mock YNAB client")
	return c.fixture.Budgets, nil
}

// FetchAccounts returns the accounts nested under the fixture's budget
// matching req.BudgetID.
func (c *FakeClient) FetchAccounts(req SyncReq) (AccountData, error) {
	slog.Info("fetching accounts from mock YNAB client", "budget_id", req.BudgetID)

	budget, ok := findBudget(c.fixture.Budgets, req.BudgetID)
	if !ok {
		return AccountData{}, fmt.Errorf("mock: unknown budget id %q", req.BudgetID)
	}

	return AccountData{Accounts: budget.Accounts}, nil
}

// FetchCategories returns the fixture's categories for req.BudgetID.
func (c *FakeClient) FetchCategories(req SyncReq) (CategoryData, error) {
	slog.Info("fetching categories from mock YNAB client", "budget_id", req.BudgetID)

	if _, ok := findBudget(c.fixture.Budgets, req.BudgetID); !ok {
		return CategoryData{}, fmt.Errorf("mock: unknown budget id %q", req.BudgetID)
	}

	return c.fixture.Categories[req.BudgetID], nil
}

// FetchPayees returns the fixture's payees for req.BudgetID.
func (c *FakeClient) FetchPayees(req SyncReq) (PayeeData, error) {
	slog.Info("fetching payees from mock YNAB client", "budget_id", req.BudgetID)

	if _, ok := findBudget(c.fixture.Budgets, req.BudgetID); !ok {
		return PayeeData{}, fmt.Errorf("mock: unknown budget id %q", req.BudgetID)
	}

	return c.fixture.Payees[req.BudgetID], nil
}

// Upload logs the transaction and always succeeds (no-op).
func (c *FakeClient) Upload(txn TxnReq) error {
	slog.Info("mock uploading transaction", "budget_id", txn.BudgetID, "account_id", txn.AccountID, "amount", txn.Amount)
	return nil
}

// findBudget looks up a budget by id, reporting whether it was found.
func findBudget(budgets []Budget, id string) (Budget, bool) {
	for _, b := range budgets {
		if b.ID == id {
			return b, true
		}
	}
	return Budget{}, false
}
