package server

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/oneils/ynab-helper/internal/txn"
	"github.com/oneils/ynab-helper/internal/ynab"
)

func TestEnrichTransactionList_Empty(t *testing.T) {
	ctx := context.Background()
	rows := enrichTransactionList(ctx, nil,
		nil,
		nil,
		nil,
	)
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestEnrichTransactionList_PayeeSuggestion(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "1", Payee: "BIEDRONKA", Account: txn.BankAccount{ID: "acc1"}},
	}

	budgetByAccID := func(_ context.Context, accID string) (ynab.Budget, error) {
		return ynab.Budget{ID: "budget1", Name: "Test Budget"}, nil
	}

	getSuggestions := func(_ context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
		return []txn.PayeeSuggestion{
			{PayeeID: "p1", PayeeName: "Biedronka"},
		}, nil
	}

	getCategorySuggestions := func(_ context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
		return nil, nil
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Txn.ID != "1" {
		t.Errorf("expected txn ID '1', got '%s'", rows[0].Txn.ID)
	}
	if rows[0].SugPayee != "Biedronka" {
		t.Errorf("expected SugPayee 'Biedronka', got '%s'", rows[0].SugPayee)
	}
	if !rows[0].AutoFilled {
		t.Error("expected AutoFilled to be true when payee suggestion exists")
	}
	if rows[0].SugCategory != "" {
		t.Errorf("expected empty SugCategory, got '%s'", rows[0].SugCategory)
	}
}

func TestEnrichTransactionList_CategorySuggestion(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "2", Payee: "LIDL", Account: txn.BankAccount{ID: "acc1"}},
	}

	budgetByAccID := func(_ context.Context, accID string) (ynab.Budget, error) {
		return ynab.Budget{ID: "budget1", Name: "Test Budget"}, nil
	}

	getSuggestions := func(_ context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
		return nil, nil
	}

	getCategorySuggestions := func(_ context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
		return []txn.CategorySuggestion{
			{CategoryID: "cat1", CategoryName: "Groceries"},
		}, nil
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SugCategory != "Groceries" {
		t.Errorf("expected SugCategory 'Groceries', got '%s'", rows[0].SugCategory)
	}
	if !rows[0].AutoFilled {
		t.Error("expected AutoFilled to be true when category suggestion exists")
	}
	if rows[0].SugPayee != "" {
		t.Errorf("expected empty SugPayee, got '%s'", rows[0].SugPayee)
	}
}

func TestEnrichTransactionList_BothSuggestions(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "3", Payee: "ZABKA", Account: txn.BankAccount{ID: "acc1"}},
	}

	budgetByAccID := func(_ context.Context, accID string) (ynab.Budget, error) {
		return ynab.Budget{ID: "budget1"}, nil
	}

	getSuggestions := func(_ context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
		return []txn.PayeeSuggestion{
			{PayeeID: "p1", PayeeName: "Zabka"},
		}, nil
	}

	getCategorySuggestions := func(_ context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
		return []txn.CategorySuggestion{
			{CategoryID: "cat1", CategoryName: "Food"},
		}, nil
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SugPayee != "Zabka" {
		t.Errorf("expected SugPayee 'Zabka', got '%s'", rows[0].SugPayee)
	}
	if rows[0].SugCategory != "Food" {
		t.Errorf("expected SugCategory 'Food', got '%s'", rows[0].SugCategory)
	}
	if !rows[0].AutoFilled {
		t.Error("expected AutoFilled to be true")
	}
}

func TestEnrichTransactionList_NoMatch(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "4", Payee: "UNKNOWN", Account: txn.BankAccount{ID: "acc1"}},
	}

	budgetByAccID := func(_ context.Context, accID string) (ynab.Budget, error) {
		return ynab.Budget{ID: "budget1"}, nil
	}

	getSuggestions := func(_ context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
		return nil, nil
	}

	getCategorySuggestions := func(_ context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
		return nil, nil
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SugPayee != "" {
		t.Errorf("expected empty SugPayee, got '%s'", rows[0].SugPayee)
	}
	if rows[0].SugCategory != "" {
		t.Errorf("expected empty SugCategory, got '%s'", rows[0].SugCategory)
	}
	if rows[0].AutoFilled {
		t.Error("expected AutoFilled to be false when no match")
	}
	if rows[0].Txn.Payee != "UNKNOWN" {
		t.Errorf("expected original payee 'UNKNOWN', got '%s'", rows[0].Txn.Payee)
	}
}

func TestEnrichTransactionList_BudgetLookupError(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "5", Payee: "SOME", Account: txn.BankAccount{ID: "acc1"}},
	}

	budgetByAccID := func(_ context.Context, accID string) (ynab.Budget, error) {
		return ynab.Budget{}, errors.New("budget not found")
	}

	getSuggestions := func(_ context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
		return []txn.PayeeSuggestion{{PayeeName: "should-not-reach"}}, nil
	}

	getCategorySuggestions := func(_ context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
		return nil, nil
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].AutoFilled {
		t.Error("expected AutoFilled to be false when budget lookup fails")
	}
	if rows[0].SugPayee != "" {
		t.Errorf("expected empty SugPayee, got '%s'", rows[0].SugPayee)
	}
}

func TestEnrichTransactionList_SuggestionErrors(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "6", Payee: "ERROR", Account: txn.BankAccount{ID: "acc1"}},
	}

	budgetByAccID := func(_ context.Context, accID string) (ynab.Budget, error) {
		return ynab.Budget{ID: "budget1"}, nil
	}

	getSuggestions := func(_ context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
		return nil, errors.New("suggestion engine error")
	}

	getCategorySuggestions := func(_ context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
		return nil, errors.New("category engine error")
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].AutoFilled {
		t.Error("expected AutoFilled to be false when suggestion calls fail")
	}
}

func TestEnrichTransactionList_BudgetCaching(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "7", Description: "A", Account: txn.BankAccount{ID: "acc1"}},
		{ID: "8", Description: "B", Account: txn.BankAccount{ID: "acc1"}},
		{ID: "9", Description: "C", Account: txn.BankAccount{ID: "acc2"}},
	}

	callCount := 0
	budgetByAccID := func(_ context.Context, accID string) (ynab.Budget, error) {
		callCount++
		return ynab.Budget{ID: "b-" + accID}, nil
	}

	getSuggestions := func(_ context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
		return []txn.PayeeSuggestion{
			{PayeeID: "p1", PayeeName: "Payee-" + description},
		}, nil
	}

	getCategorySuggestions := func(_ context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
		return nil, nil
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions)

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if callCount != 2 {
		t.Errorf("expected 2 budget lookups (one per unique account), got %d", callCount)
	}
	if rows[0].SugPayee != "Payee-A" {
		t.Errorf("expected SugPayee 'Payee-A', got '%s'", rows[0].SugPayee)
	}
	if rows[1].SugPayee != "Payee-B" {
		t.Errorf("expected SugPayee 'Payee-B', got '%s'", rows[1].SugPayee)
	}
	if rows[2].SugPayee != "Payee-C" {
		t.Errorf("expected SugPayee 'Payee-C', got '%s'", rows[2].SugPayee)
	}
}

func TestWrapTransactions(t *testing.T) {
	tests := []struct {
		name string
		txns []txn.Transaction
		want int
	}{
		{name: "empty", txns: nil, want: 0},
		{name: "single", txns: []txn.Transaction{{ID: "1"}}, want: 1},
		{name: "multiple", txns: []txn.Transaction{{ID: "1"}, {ID: "2"}}, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := wrapTransactions(tt.txns)
			if len(rows) != tt.want {
				t.Errorf("expected %d rows, got %d", tt.want, len(rows))
			}
			for i, row := range rows {
				if row.Txn.ID != tt.txns[i].ID {
					t.Errorf("row %d: expected Txn.ID '%s', got '%s'", i, tt.txns[i].ID, row.Txn.ID)
				}
				if row.AutoFilled {
					t.Errorf("row %d: expected AutoFilled to be false for wrapTransactions", i)
				}
			}
		})
	}
}

func TestDetailRouteExists(t *testing.T) {
	s := &Server{}
	r := s.routes()

	// Verify the route matches and returns a handler (not 405 Method Not Allowed)
	var matched bool
	_ = chi.Walk(r, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if method == "GET" && route == "/bank-txns/{id}/detail" {
			matched = true
		}
		return nil
	})

	if !matched {
		t.Error("expected GET /bank-txns/{id}/detail route to exist")
	}
}

func TestDetailRouteNotFoundWhenTxnMissing(t *testing.T) {
	s := &Server{}

	// Verify that the old fetchBankTxnHandler route is no longer registered
	var oldRouteFound bool
	_ = chi.Walk(s.routes(), func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if method == "GET" && route == "/bank-txns/{id}" {
			oldRouteFound = true
		}
		return nil
	})

	if oldRouteFound {
		t.Error("expected GET /bank-txns/{id} (old fetchBankTxnHandler) route to be removed")
	}
}

func TestDetailRouteNotFound_InlineRoutesRemoved(t *testing.T) {
	s := &Server{}

	removedRoutes := []string{
		"/bank-txns/{id}/edit-inline",
		"/bank-txns/{id}/view-inline",
	}

	for _, route := range removedRoutes {
		var found bool
		_ = chi.Walk(s.routes(), func(method, rte string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			if method == "GET" && rte == route {
				found = true
			}
			return nil
		})
		if found {
			t.Errorf("expected route %s to be removed", route)
		}
	}
}

func TestDetailRoute_SaveInlineStillExists(t *testing.T) {
	s := &Server{}

	var found bool
	_ = chi.Walk(s.routes(), func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if method == "POST" && route == "/bank-txns/{id}/save-inline" {
			found = true
		}
		return nil
	})

	if !found {
		t.Error("expected POST /bank-txns/{id}/save-inline route to still exist")
	}
}
