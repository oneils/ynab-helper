package ynab

import "testing"

func testFixture() Fixture {
	return Fixture{
		Budgets: []Budget{
			{
				ID:   "budget-1",
				Name: "Budget One",
				Accounts: []Account{
					{ID: "acc-1", Name: "Checking"},
				},
			},
			{
				ID:   "budget-no-data",
				Name: "Budget Without Categories/Payees",
			},
		},
		Categories: map[string]CategoryData{
			"budget-1": {Categories: []CategoryGroup{{ID: "cg-1", Name: "Everyday"}}},
		},
		Payees: map[string]PayeeData{
			"budget-1": {Payees: []Payee{{ID: "payee-1", Name: "Local Grocer"}}},
		},
	}
}

func TestFakeClient_FetchBudgets(t *testing.T) {
	client := NewFakeClient(testFixture())

	budgets, err := client.FetchBudgets()
	if err != nil {
		t.Fatalf("FetchBudgets returned error: %v", err)
	}
	if len(budgets) != 2 {
		t.Fatalf("expected 2 budgets, got %d", len(budgets))
	}
}

func TestFakeClient_FetchAccounts(t *testing.T) {
	client := NewFakeClient(testFixture())

	data, err := client.FetchAccounts(SyncReq{BudgetID: "budget-1"})
	if err != nil {
		t.Fatalf("FetchAccounts returned error: %v", err)
	}
	if len(data.Accounts) != 1 || data.Accounts[0].ID != "acc-1" {
		t.Fatalf("expected fixture accounts, got %+v", data.Accounts)
	}
}

func TestFakeClient_FetchAccounts_UnknownBudget(t *testing.T) {
	client := NewFakeClient(testFixture())

	_, err := client.FetchAccounts(SyncReq{BudgetID: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown budget id, got nil")
	}
}

func TestFakeClient_FetchCategories(t *testing.T) {
	client := NewFakeClient(testFixture())

	data, err := client.FetchCategories(SyncReq{BudgetID: "budget-1"})
	if err != nil {
		t.Fatalf("FetchCategories returned error: %v", err)
	}
	if len(data.Categories) != 1 || data.Categories[0].ID != "cg-1" {
		t.Fatalf("expected fixture categories, got %+v", data.Categories)
	}
}

func TestFakeClient_FetchCategories_UnknownBudget(t *testing.T) {
	client := NewFakeClient(testFixture())

	_, err := client.FetchCategories(SyncReq{BudgetID: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown budget id, got nil")
	}
}

func TestFakeClient_FetchCategories_KnownBudgetNoData(t *testing.T) {
	client := NewFakeClient(testFixture())

	data, err := client.FetchCategories(SyncReq{BudgetID: "budget-no-data"})
	if err != nil {
		t.Fatalf("FetchCategories returned error: %v", err)
	}
	if len(data.Categories) != 0 {
		t.Fatalf("expected empty categories, got %+v", data.Categories)
	}
}

func TestFakeClient_FetchPayees(t *testing.T) {
	client := NewFakeClient(testFixture())

	data, err := client.FetchPayees(SyncReq{BudgetID: "budget-1"})
	if err != nil {
		t.Fatalf("FetchPayees returned error: %v", err)
	}
	if len(data.Payees) != 1 || data.Payees[0].ID != "payee-1" {
		t.Fatalf("expected fixture payees, got %+v", data.Payees)
	}
}

func TestFakeClient_FetchPayees_UnknownBudget(t *testing.T) {
	client := NewFakeClient(testFixture())

	_, err := client.FetchPayees(SyncReq{BudgetID: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown budget id, got nil")
	}
}

func TestFakeClient_FetchPayees_KnownBudgetNoData(t *testing.T) {
	client := NewFakeClient(testFixture())

	data, err := client.FetchPayees(SyncReq{BudgetID: "budget-no-data"})
	if err != nil {
		t.Fatalf("FetchPayees returned error: %v", err)
	}
	if len(data.Payees) != 0 {
		t.Fatalf("expected empty payees, got %+v", data.Payees)
	}
}

func TestFakeClient_Upload(t *testing.T) {
	client := NewFakeClient(testFixture())

	if err := client.Upload(TxnReq{BudgetID: "budget-1", AccountID: "acc-1", Amount: -1000}); err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if err := client.Upload(TxnReq{}); err != nil {
		t.Fatalf("Upload with zero-value input returned error: %v", err)
	}
}
