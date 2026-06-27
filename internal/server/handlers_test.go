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

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, nil, nil)

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

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, nil, nil)

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

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, nil, nil)

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

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, nil, nil)

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

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, nil, nil)

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

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, nil, nil)

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

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, nil, nil)

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

func TestPayeeNameByID(t *testing.T) {
	payees := []ynab.Payee{
		{ID: "p1", Name: "Amazon"},
		{ID: "p2", Name: "Netflix"},
		{ID: "p3", Name: "Spotify"},
	}

	tests := []struct {
		name   string
		payees []ynab.Payee
		id     string
		want   string
	}{
		{name: "match found", payees: payees, id: "p2", want: "Netflix"},
		{name: "no match", payees: payees, id: "p99", want: ""},
		{name: "empty slice", payees: nil, id: "p1", want: ""},
		{name: "empty id", payees: payees, id: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := payeeNameByID(tt.payees, tt.id)
			if got != tt.want {
				t.Errorf("payeeNameByID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCategoryNameByID(t *testing.T) {
	categories := []ynab.Category{
		{ID: "c1", Name: "Groceries"},
		{ID: "c2", Name: "Rent"},
		{ID: "c3", Name: "Utilities"},
	}

	tests := []struct {
		name       string
		categories []ynab.Category
		id         string
		want       string
	}{
		{name: "match found", categories: categories, id: "c1", want: "Groceries"},
		{name: "no match", categories: categories, id: "c99", want: ""},
		{name: "empty slice", categories: nil, id: "c1", want: ""},
		{name: "empty id", categories: categories, id: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := categoryNameByID(tt.categories, tt.id)
			if got != tt.want {
				t.Errorf("categoryNameByID() = %q, want %q", got, tt.want)
			}
		})
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

func TestEnrichTransactionList_FallbackPayeeMatch(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "1", Description: "Purchase at BIEDRONKA", Account: txn.BankAccount{ID: "acc1"}},
	}

	budgetByAccID := func(_ context.Context, _ string) (ynab.Budget, error) {
		return ynab.Budget{ID: "budget1"}, nil
	}
	getSuggestions := func(_ context.Context, _, _ string) ([]txn.PayeeSuggestion, error) {
		return nil, nil // no learned patterns
	}
	getCategorySuggestions := func(_ context.Context, _, _, _ string) ([]txn.CategorySuggestion, error) {
		return nil, nil
	}
	getPayeesByBudget := func(_ context.Context, _ string) ([]ynab.Payee, error) {
		return []ynab.Payee{{ID: "p1", Name: "Biedronka"}}, nil
	}
	suggestPayee := func(t txn.Transaction, payees []ynab.Payee) ynab.Payee {
		return ynab.Payee{ID: "p1", Name: "Biedronka"}
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, getPayeesByBudget, suggestPayee)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SugPayee != "Biedronka" {
		t.Errorf("expected SugPayee 'Biedronka', got '%s'", rows[0].SugPayee)
	}
	if !rows[0].AutoFilled {
		t.Error("expected AutoFilled to be true when fallback payee found")
	}
}

func TestEnrichTransactionList_FallbackNoMatch(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "1", Description: "Unknown transaction", Account: txn.BankAccount{ID: "acc1"}},
	}

	budgetByAccID := func(_ context.Context, _ string) (ynab.Budget, error) {
		return ynab.Budget{ID: "budget1"}, nil
	}
	getSuggestions := func(_ context.Context, _, _ string) ([]txn.PayeeSuggestion, error) {
		return nil, nil
	}
	getCategorySuggestions := func(_ context.Context, _, _, _ string) ([]txn.CategorySuggestion, error) {
		return nil, nil
	}
	getPayeesByBudget := func(_ context.Context, _ string) ([]ynab.Payee, error) {
		return []ynab.Payee{{ID: "p1", Name: "Biedronka"}}, nil
	}
	suggestPayee := func(_ txn.Transaction, _ []ynab.Payee) ynab.Payee {
		return ynab.Payee{} // no match
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, getPayeesByBudget, suggestPayee)

	if rows[0].SugPayee != "" {
		t.Errorf("expected empty SugPayee on no match, got '%s'", rows[0].SugPayee)
	}
	if rows[0].AutoFilled {
		t.Error("expected AutoFilled false when no match")
	}
}

func TestEnrichTransactionList_FallbackSkippedWhenPatternExists(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "1", Description: "LIDL 123", Account: txn.BankAccount{ID: "acc1"}},
	}

	budgetByAccID := func(_ context.Context, _ string) (ynab.Budget, error) {
		return ynab.Budget{ID: "budget1"}, nil
	}
	getSuggestions := func(_ context.Context, _, _ string) ([]txn.PayeeSuggestion, error) {
		return []txn.PayeeSuggestion{{PayeeID: "pattern-p", PayeeName: "PatternPayee"}}, nil
	}
	getCategorySuggestions := func(_ context.Context, _, _, _ string) ([]txn.CategorySuggestion, error) {
		return nil, nil
	}
	payeeLookupCalled := false
	getPayeesByBudget := func(_ context.Context, _ string) ([]ynab.Payee, error) {
		payeeLookupCalled = true
		return []ynab.Payee{{ID: "p1", Name: "Lidl"}}, nil
	}
	suggestPayee := func(_ txn.Transaction, _ []ynab.Payee) ynab.Payee {
		return ynab.Payee{ID: "p1", Name: "Lidl"}
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, getPayeesByBudget, suggestPayee)

	if payeeLookupCalled {
		t.Error("expected getPayeesByBudget not to be called when pattern suggestion exists")
	}
	if rows[0].SugPayee != "PatternPayee" {
		t.Errorf("expected pattern payee 'PatternPayee', got '%s'", rows[0].SugPayee)
	}
}

func TestEnrichTransactionList_FallbackEmptyPayeeID(t *testing.T) {
	ctx := context.Background()

	txns := []txn.Transaction{
		{ID: "1", Description: "Some transaction", Account: txn.BankAccount{ID: "acc1"}},
	}

	budgetByAccID := func(_ context.Context, _ string) (ynab.Budget, error) {
		return ynab.Budget{ID: "budget1"}, nil
	}
	getSuggestions := func(_ context.Context, _, _ string) ([]txn.PayeeSuggestion, error) {
		return nil, nil
	}
	getCategorySuggestions := func(_ context.Context, _, _, _ string) ([]txn.CategorySuggestion, error) {
		return nil, nil
	}
	getPayeesByBudget := func(_ context.Context, _ string) ([]ynab.Payee, error) {
		return []ynab.Payee{{ID: "", Name: ""}}, nil // empty name/ID payee
	}
	suggestPayee := func(_ txn.Transaction, _ []ynab.Payee) ynab.Payee {
		return ynab.Payee{} // SuggestPayee returns zero value for empty name
	}

	rows := enrichTransactionList(ctx, txns, budgetByAccID, getSuggestions, getCategorySuggestions, getPayeesByBudget, suggestPayee)

	if rows[0].SugPayee != "" {
		t.Errorf("expected no prefill for empty payee name, got '%s'", rows[0].SugPayee)
	}
	if rows[0].AutoFilled {
		t.Error("expected AutoFilled false for empty payee")
	}
}

func TestApplyYnabPayeeFallback_PatternWins(t *testing.T) {
	t1 := txn.Transaction{ID: "1", Description: "test"}
	payees := []ynab.Payee{{ID: "ynab-p", Name: "YnabPayee", LastCategoryID: "ynab-cat"}}
	suggestFn := func(_ txn.Transaction, _ []ynab.Payee) ynab.Payee {
		return ynab.Payee{ID: "ynab-p", Name: "YnabPayee", LastCategoryID: "ynab-cat"}
	}

	payeeID, catID := applyYnabPayeeFallback(t1, payees, suggestFn, "pattern-p", "pattern-cat")

	if payeeID != "pattern-p" {
		t.Errorf("expected pattern payee ID, got '%s'", payeeID)
	}
	if catID != "pattern-cat" {
		t.Errorf("expected pattern category ID, got '%s'", catID)
	}
}

func TestApplyYnabPayeeFallback_PayeeAndCategoryFromLastCategoryID(t *testing.T) {
	t1 := txn.Transaction{ID: "1", Description: "BIEDRONKA"}
	payees := []ynab.Payee{{ID: "p1", Name: "Biedronka", LastCategoryID: "cat-groceries"}}
	suggestFn := func(_ txn.Transaction, _ []ynab.Payee) ynab.Payee {
		return ynab.Payee{ID: "p1", Name: "Biedronka", LastCategoryID: "cat-groceries"}
	}

	payeeID, catID := applyYnabPayeeFallback(t1, payees, suggestFn, "", "")

	if payeeID != "p1" {
		t.Errorf("expected fallback payee 'p1', got '%s'", payeeID)
	}
	if catID != "cat-groceries" {
		t.Errorf("expected LastCategoryID 'cat-groceries', got '%s'", catID)
	}
}

func TestApplyYnabPayeeFallback_CategoryEmptyWhenLastCategoryIDMissing(t *testing.T) {
	t1 := txn.Transaction{ID: "1", Description: "LIDL"}
	payees := []ynab.Payee{{ID: "p2", Name: "Lidl", LastCategoryID: ""}}
	suggestFn := func(_ txn.Transaction, _ []ynab.Payee) ynab.Payee {
		return ynab.Payee{ID: "p2", Name: "Lidl", LastCategoryID: ""}
	}

	payeeID, catID := applyYnabPayeeFallback(t1, payees, suggestFn, "", "")

	if payeeID != "p2" {
		t.Errorf("expected fallback payee 'p2', got '%s'", payeeID)
	}
	if catID != "" {
		t.Errorf("expected empty catID when LastCategoryID is empty, got '%s'", catID)
	}
}

func TestApplyYnabPayeeFallback_NoMatchReturnsEmpty(t *testing.T) {
	t1 := txn.Transaction{ID: "1", Description: "Unknown"}
	payees := []ynab.Payee{{ID: "p1", Name: "Biedronka"}}
	suggestFn := func(_ txn.Transaction, _ []ynab.Payee) ynab.Payee {
		return ynab.Payee{}
	}

	payeeID, catID := applyYnabPayeeFallback(t1, payees, suggestFn, "", "")

	if payeeID != "" {
		t.Errorf("expected empty payeeID on no match, got '%s'", payeeID)
	}
	if catID != "" {
		t.Errorf("expected empty catID on no match, got '%s'", catID)
	}
}
