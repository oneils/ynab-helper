package txn

import (
	"context"
	"errors"
	"testing"

	"github.com/oneils/ynab-helper/internal/ynab"
)

// Mock implementations for testing

type mockTransactionStore struct {
	insertFunc         func(ctx context.Context, t Transaction) error
	fetchByAccountFunc func(ctx context.Context, accID string, status string) ([]Transaction, error)
	findByIDFunc       func(ctx context.Context, id string) (Transaction, error)
	updateStatusFunc   func(ctx context.Context, id string, status TransactionStatus) error
	countByStatusFunc  func(ctx context.Context, accountID string) (map[TransactionStatus]int, error)
	transactions       []Transaction
	statusUpdates      map[string]TransactionStatus
}

func (m *mockTransactionStore) InsertTransaction(ctx context.Context, t Transaction) error {
	if m.insertFunc != nil {
		return m.insertFunc(ctx, t)
	}
	m.transactions = append(m.transactions, t)
	return nil
}

func (m *mockTransactionStore) FetchTransactionsByAccount(ctx context.Context, accID string, status string) ([]Transaction, error) {
	if m.fetchByAccountFunc != nil {
		return m.fetchByAccountFunc(ctx, accID, status)
	}
	var result []Transaction
	for _, t := range m.transactions {
		if t.Account.ID == accID && (status == "" || string(t.Status) == status) {
			result = append(result, t)
		}
	}
	return result, nil
}

func (m *mockTransactionStore) FindTransactionByID(ctx context.Context, id string) (Transaction, error) {
	if m.findByIDFunc != nil {
		return m.findByIDFunc(ctx, id)
	}
	for _, t := range m.transactions {
		if t.ID == id {
			return t, nil
		}
	}
	return Transaction{}, errors.New("transaction not found")
}

func (m *mockTransactionStore) UpdateTransactionStatus(ctx context.Context, id string, status TransactionStatus) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, id, status)
	}
	if m.statusUpdates == nil {
		m.statusUpdates = make(map[string]TransactionStatus)
	}
	m.statusUpdates[id] = status
	for i, t := range m.transactions {
		if t.ID == id {
			m.transactions[i].Status = status
			return nil
		}
	}
	return nil
}

func (m *mockTransactionStore) CountByStatus(ctx context.Context, accountID string) (map[TransactionStatus]int, error) {
	if m.countByStatusFunc != nil {
		return m.countByStatusFunc(ctx, accountID)
	}
	counts := make(map[TransactionStatus]int)
	for _, t := range m.transactions {
		if t.Account.ID == accountID {
			counts[t.Status]++
		}
	}
	return counts, nil
}

type mockYnabClient struct {
	uploadFunc func(req ynab.TxnReq) error
	uploads    []ynab.TxnReq
}

func (m *mockYnabClient) Upload(req ynab.TxnReq) error {
	if m.uploadFunc != nil {
		return m.uploadFunc(req)
	}
	m.uploads = append(m.uploads, req)
	return nil
}

type mockBudgetFinder struct {
	findFunc func(ctx context.Context, accID string) (ynab.Budget, error)
	budget   ynab.Budget
}

func (m *mockBudgetFinder) FindBudgetByAccountID(ctx context.Context, accID string) (ynab.Budget, error) {
	if m.findFunc != nil {
		return m.findFunc(ctx, accID)
	}
	return m.budget, nil
}

type mockParserMappingLookup struct {
	getFunc    func(ctx context.Context, accountID string) (string, error)
	parserName string
	err        error
}

func (m *mockParserMappingLookup) GetParserMapping(ctx context.Context, accountID string) (string, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, accountID)
	}
	return m.parserName, m.err
}

type mockReportParser struct {
	parseFunc func(acc BankAccount, data [][]string) []Transaction
}

func (m *mockReportParser) Parse(acc BankAccount, data [][]string) []Transaction {
	if m.parseFunc != nil {
		return m.parseFunc(acc, data)
	}
	return nil
}

// Tests

func TestProcessor_Fetch(t *testing.T) {
	ctx := context.Background()

	store := &mockTransactionStore{
		transactions: []Transaction{
			{ID: "1", Account: BankAccount{ID: "acc1"}, Status: TransactionDraft},
			{ID: "2", Account: BankAccount{ID: "acc1"}, Status: TransactionProcessed},
			{ID: "3", Account: BankAccount{ID: "acc2"}, Status: TransactionDraft},
		},
	}

	processor := NewProcessor(nil, store, nil, nil, nil, nil)

	t.Run("Fetch by account ID", func(t *testing.T) {
		params := ProcessParams{AccountID: "acc1", Status: ""}
		txns, err := processor.Fetch(ctx, params)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(txns) != 2 {
			t.Errorf("Expected 2 transactions, got %d", len(txns))
		}
	})

	t.Run("Fetch by account ID and status", func(t *testing.T) {
		params := ProcessParams{AccountID: "acc1", Status: string(TransactionDraft)}
		txns, err := processor.Fetch(ctx, params)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction, got %d", len(txns))
		}
		if txns[0].ID != "1" {
			t.Errorf("Expected transaction ID '1', got '%s'", txns[0].ID)
		}
	})
}

func TestProcessor_FetchByID(t *testing.T) {
	ctx := context.Background()

	store := &mockTransactionStore{
		transactions: []Transaction{
			{ID: "txn-123", Account: BankAccount{ID: "acc1"}, Amount: -100.50},
		},
	}

	processor := NewProcessor(nil, store, nil, nil, nil, nil)

	t.Run("Fetch existing transaction", func(t *testing.T) {
		txn, err := processor.FetchByID(ctx, "txn-123")

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if txn.ID != "txn-123" {
			t.Errorf("Expected transaction ID 'txn-123', got '%s'", txn.ID)
		}
		if txn.Amount != -100.50 {
			t.Errorf("Expected amount -100.50, got %f", txn.Amount)
		}
	})

	t.Run("Fetch non-existent transaction", func(t *testing.T) {
		_, err := processor.FetchByID(ctx, "non-existent")

		if err == nil {
			t.Error("Expected error for non-existent transaction")
		}
	})
}

func TestProcessor_Skip(t *testing.T) {
	ctx := context.Background()

	store := &mockTransactionStore{
		transactions: []Transaction{
			{ID: "txn-123", Status: TransactionDraft},
		},
	}

	processor := NewProcessor(nil, store, nil, nil, nil, nil)

	err := processor.Skip(ctx, "txn-123")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if store.statusUpdates["txn-123"] != TransactionSkipped {
		t.Errorf("Expected status TransactionSkipped, got %v", store.statusUpdates["txn-123"])
	}
}

func TestProcessor_SaveToYnab(t *testing.T) {
	ctx := context.Background()

	t.Run("Successful save", func(t *testing.T) {
		store := &mockTransactionStore{
			transactions: []Transaction{
				{ID: "txn-123", Status: TransactionDraft},
			},
		}

		client := &mockYnabClient{}

		processor := NewProcessor(nil, store, nil, client, nil, nil)

		form := SaveForm{
			TxnID:      "txn-123",
			BudgetID:   "budget-1",
			AccountID:  "acc-1",
			TxnDate:    "2024-01-15",
			PayeeID:    "payee-1",
			Amount:     "-100.50",
			CategoryID: "cat-1",
			Memo:       "Test transaction",
		}

		err := processor.SaveToYnab(ctx, form)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if len(client.uploads) != 1 {
			t.Fatalf("Expected 1 upload, got %d", len(client.uploads))
		}

		upload := client.uploads[0]
		if upload.BudgetID != "budget-1" {
			t.Errorf("Expected budget ID 'budget-1', got '%s'", upload.BudgetID)
		}
		if upload.Amount != -100500 { // -100.50 * 1000
			t.Errorf("Expected amount -100500, got %d", upload.Amount)
		}
		if upload.Cleared != "cleared" {
			t.Errorf("Expected cleared status 'cleared', got '%s'", upload.Cleared)
		}
		if !upload.Approved {
			t.Error("Expected approved to be true")
		}

		if store.statusUpdates["txn-123"] != TransactionProcessed {
			t.Errorf("Expected status TransactionProcessed, got %v", store.statusUpdates["txn-123"])
		}
	})

	t.Run("Invalid date format", func(t *testing.T) {
		processor := NewProcessor(nil, &mockTransactionStore{}, nil, &mockYnabClient{}, nil, nil)

		form := SaveForm{
			TxnID:   "txn-123",
			TxnDate: "invalid-date",
			Amount:  "100.00",
		}

		err := processor.SaveToYnab(ctx, form)

		if err == nil {
			t.Error("Expected error for invalid date format")
		}
	})

	t.Run("Invalid amount format", func(t *testing.T) {
		processor := NewProcessor(nil, &mockTransactionStore{}, nil, &mockYnabClient{}, nil, nil)

		form := SaveForm{
			TxnID:   "txn-123",
			TxnDate: "2024-01-15",
			Amount:  "invalid",
		}

		err := processor.SaveToYnab(ctx, form)

		if err == nil {
			t.Error("Expected error for invalid amount format")
		}
	})

	t.Run("YNAB upload error", func(t *testing.T) {
		store := &mockTransactionStore{}
		client := &mockYnabClient{
			uploadFunc: func(req ynab.TxnReq) error {
				return errors.New("YNAB API error")
			},
		}

		processor := NewProcessor(nil, store, nil, client, nil, nil)

		form := SaveForm{
			TxnID:   "txn-123",
			TxnDate: "2024-01-15",
			Amount:  "100.00",
		}

		err := processor.SaveToYnab(ctx, form)

		if err == nil {
			t.Error("Expected error from YNAB upload")
		}
	})
}

func TestProcessor_SuggestPayee(t *testing.T) {
	processor := NewProcessor(nil, nil, nil, nil, nil, nil)

	payees := []ynab.Payee{
		{ID: "p1", Name: "BIEDRONKA"},
		{ID: "p2", Name: "LIDL"},
		{ID: "p3", Name: "McDonald's"},
	}

	tests := []struct {
		name              string
		transaction       Transaction
		expectedPayeeID   string
		expectedPayeeName string
	}{
		{
			name: "Exact match in Payee field",
			transaction: Transaction{
				Payee:       "BIEDRONKA",
				Description: "Some other text",
			},
			expectedPayeeID:   "p1",
			expectedPayeeName: "BIEDRONKA",
		},
		{
			name: "Partial match in Payee field",
			transaction: Transaction{
				Payee:       "BIEDRONKA POZNAN",
				Description: "Some other text",
			},
			expectedPayeeID:   "p1",
			expectedPayeeName: "BIEDRONKA",
		},
		{
			name: "Match in Description when Payee is empty",
			transaction: Transaction{
				Payee:       "",
				Description: "Purchase at LIDL store",
			},
			expectedPayeeID:   "p2",
			expectedPayeeName: "LIDL",
		},
		{
			name: "No match",
			transaction: Transaction{
				Payee:       "UNKNOWN SHOP",
				Description: "Unknown transaction",
			},
			expectedPayeeID:   "",
			expectedPayeeName: "",
		},
		{
			name: "Case insensitive match",
			transaction: Transaction{
				Payee:       "mcdonald's",
				Description: "",
			},
			expectedPayeeID:   "p3",
			expectedPayeeName: "McDonald's",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.SuggestPayee(tt.transaction, payees)

			if result.ID != tt.expectedPayeeID {
				t.Errorf("Expected payee ID '%s', got '%s'", tt.expectedPayeeID, result.ID)
			}
			if result.Name != tt.expectedPayeeName {
				t.Errorf("Expected payee name '%s', got '%s'", tt.expectedPayeeName, result.Name)
			}
		})
	}
}

func TestProcessor_Normalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "ąćęłńóśźż",
			expected: "acelnoszz",
		},
		{
			input:    "ĄĆĘŁŃÓŚŹŻ",
			expected: "ACELNOSZZ",
		},
		{
			input:    "Zażółć gęślą jaźń",
			expected: "Zazolc gesla jazn",
		},
		{
			input:    "Regular text 123",
			expected: "Regular text 123",
		},
		{
			input:    "Text with émojis 🎉",
			expected: "Text with mojis ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalize(tt.input)
			if result != tt.expected {
				t.Errorf("normalize(%q) = %q; expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestProcessor_ParseYnabTime(t *testing.T) {
	processor := NewProcessor(nil, nil, nil, nil, nil, nil)

	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			name:      "Valid date format",
			input:     "2024-01-15",
			expectErr: false,
		},
		{
			name:      "Valid date different month",
			input:     "2024-12-31",
			expectErr: false,
		},
		{
			name:      "Alternative date format (DD-MM-YYYY not valid YYYY-MM-DD)",
			input:     "15-01-2024",
			expectErr: true,
		},
		{
			name:      "Invalid characters",
			input:     "2024-ab-cd",
			expectErr: true,
		},
		{
			name:      "Empty string",
			input:     "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.parseYnabTime(tt.input)

			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.input {
					t.Errorf("Expected result %q, got %q", tt.input, result)
				}
			}
		})
	}
}

func TestProcessor_ParserNames(t *testing.T) {
	parsers := map[string]ReportParser{
		"Santander": nil,
		"Revolut":   nil,
	}
	processor := NewProcessor(parsers, nil, nil, nil, nil, nil)

	names := processor.ParserNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["Santander"] || !found["Revolut"] {
		t.Errorf("expected Santander and Revolut, got %v", names)
	}
}

func TestProcessor_Process_ParserMapping(t *testing.T) {
	ctx := context.Background()

	budgetStore := &mockBudgetFinder{
		budget: ynab.Budget{
			Accounts: []ynab.Account{
				{ID: "acc1", Name: "My Checking"},
			},
		},
	}

	t.Run("errors when no mapping found", func(t *testing.T) {
		store := &mockTransactionStore{}
		mapping := &mockParserMappingLookup{parserName: ""}
		processor := NewProcessor(nil, store, budgetStore, nil, nil, mapping)

		err := processor.Process(ctx, ProcessParams{AccountID: "acc1", Data: [][]string{{"header"}}})

		if err == nil {
			t.Fatal("Expected error when no mapping found")
		}
	})

	t.Run("errors when mapped parser is not registered", func(t *testing.T) {
		store := &mockTransactionStore{}
		mapping := &mockParserMappingLookup{parserName: "Unknown"}
		processor := NewProcessor(map[string]ReportParser{}, store, budgetStore, nil, nil, mapping)

		err := processor.Process(ctx, ProcessParams{AccountID: "acc1", Data: [][]string{{"header"}}})

		if err == nil {
			t.Fatal("Expected error when mapped parser is not registered")
		}
	})

	t.Run("succeeds when mapping exists and parser is registered", func(t *testing.T) {
		store := &mockTransactionStore{}
		mapping := &mockParserMappingLookup{parserName: "Santander"}
		parser := &mockReportParser{
			parseFunc: func(acc BankAccount, data [][]string) []Transaction {
				return []Transaction{{ID: "t1", Account: acc}}
			},
		}
		parsers := map[string]ReportParser{"Santander": parser}
		processor := NewProcessor(parsers, store, budgetStore, nil, nil, mapping)

		err := processor.Process(ctx, ProcessParams{AccountID: "acc1", Data: [][]string{{"header"}}})

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(store.transactions) != 1 {
			t.Fatalf("Expected 1 transaction saved, got %d", len(store.transactions))
		}
	})

	t.Run("propagates mapping lookup errors", func(t *testing.T) {
		store := &mockTransactionStore{}
		mapping := &mockParserMappingLookup{err: errors.New("db error")}
		processor := NewProcessor(nil, store, budgetStore, nil, nil, mapping)

		err := processor.Process(ctx, ProcessParams{AccountID: "acc1", Data: [][]string{{"header"}}})

		if err == nil {
			t.Fatal("Expected error to be propagated")
		}
	})
}

func TestProcessor_Preview_ParserMapping(t *testing.T) {
	ctx := context.Background()

	budgetStore := &mockBudgetFinder{
		budget: ynab.Budget{
			Accounts: []ynab.Account{
				{ID: "acc1", Name: "My Checking"},
			},
		},
	}

	t.Run("returns validation error (no Go error) when no mapping found", func(t *testing.T) {
		store := &mockTransactionStore{}
		mapping := &mockParserMappingLookup{parserName: ""}
		processor := NewProcessor(nil, store, budgetStore, nil, nil, mapping)

		result, err := processor.Preview(ctx, ProcessParams{AccountID: "acc1", Data: [][]string{{"header"}}})

		if err != nil {
			t.Fatalf("Expected nil Go error, got: %v", err)
		}
		if len(result.ValidationErrors) == 0 {
			t.Fatal("Expected a validation error")
		}
	})

	t.Run("returns validation error (no Go error) when parser is unknown", func(t *testing.T) {
		store := &mockTransactionStore{}
		mapping := &mockParserMappingLookup{parserName: "Unknown"}
		processor := NewProcessor(map[string]ReportParser{}, store, budgetStore, nil, nil, mapping)

		result, err := processor.Preview(ctx, ProcessParams{AccountID: "acc1", Data: [][]string{{"header"}}})

		if err != nil {
			t.Fatalf("Expected nil Go error, got: %v", err)
		}
		if len(result.ValidationErrors) == 0 {
			t.Fatal("Expected a validation error")
		}
	})
}
