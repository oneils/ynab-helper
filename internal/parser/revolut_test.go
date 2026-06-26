package parser

import (
	"testing"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

func TestRevolutParser_Parse_ValidData(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     1,
		AmountIndex:          2,
		FeeIndex:             3,
		CurrencyIndex:        4,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        5,
		Header:               HeaderCfg{HasHeader: true},
	}

	parser := NewRevolutParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})

	account := txn.BankAccount{
		ID:   "acc-revolut-123",
		Name: "Revolut Test Account",
	}

	data := [][]string{
		{"Date", "Description", "Amount", "Fee", "Currency"},
		{"2024-01-10", "Spotify Premium", "-9.99", "0.00", "EUR"},
		{"2024-01-11", "Amazon.com", "-45.50", "-2.50", "EUR"},
		{"2024-01-12", "Salary", "3000.00", "0.00", "EUR"},
	}

	results := parser.Parse(account, data)

	if len(results) != 3 {
		t.Fatalf("Expected 3 transactions, got %d", len(results))
	}

	// Test first transaction - Spotify (no fee)
	tx1 := results[0]
	if tx1.Status != txn.TransactionDraft {
		t.Errorf("Expected status TransactionDraft, got %v", tx1.Status)
	}
	if tx1.Amount != -9.99 {
		t.Errorf("Expected amount -9.99, got %f", tx1.Amount)
	}
	if tx1.Currency != "EUR" {
		t.Errorf("Expected currency EUR, got %s", tx1.Currency)
	}
	if tx1.Payee != "Spotify Premium" {
		t.Errorf("Expected payee 'Spotify Premium', got '%s'", tx1.Payee)
	}
	if tx1.Description != "Spotify Premium" {
		t.Errorf("Expected description 'Spotify Premium', got '%s'", tx1.Description)
	}
	expectedDate := time.Date(2024, 1, 10, 0, 0, 0, 0, time.FixedZone("Europe/Warsaw", 3600))
	if !tx1.TxnTime.Equal(expectedDate) {
		t.Errorf("Expected date %v, got %v", expectedDate, tx1.TxnTime)
	}

	// Test second transaction - Amazon with fee
	tx2 := results[1]
	if tx2.Payee != "Amazon.com" {
		t.Errorf("Expected payee 'Amazon.com', got '%s'", tx2.Payee)
	}
	// Amount should be initial amount minus fee: -45.50 - (-2.50) = -48.00
	expectedAmount := -45.50 - (-2.50)
	if tx2.Amount != expectedAmount {
		t.Errorf("Expected amount %f (amount + fee), got %f", expectedAmount, tx2.Amount)
	}

	// Test third transaction - Salary (positive amount)
	tx3 := results[2]
	if tx3.Payee != "Salary" {
		t.Errorf("Expected payee 'Salary', got '%s'", tx3.Payee)
	}
	if tx3.Amount != 3000.00 {
		t.Errorf("Expected amount 3000.00, got %f", tx3.Amount)
	}
}

func TestRevolutParser_Parse_EmptyData(t *testing.T) {
	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     1,
		AmountIndex:          2,
		FeeIndex:             3,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        5,
	}

	parser := NewRevolutParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: time.Now()})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	results := parser.Parse(account, [][]string{})

	if len(results) != 0 {
		t.Errorf("Expected 0 transactions for empty data, got %d", len(results))
	}
}

func TestRevolutParser_Parse_InvalidColumns(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     1,
		AmountIndex:          2,
		FeeIndex:             3,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        5,
	}

	parser := NewRevolutParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"2024-01-10", "Description"},
	}

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Status != txn.TransactionInvalid {
		t.Errorf("Expected status TransactionInvalid, got %v", results[0].Status)
	}
	if results[0].ErrorMsg == "" {
		t.Error("Expected error message for invalid columns")
	}
}

func TestRevolutParser_Parse_InvalidAmount(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     1,
		AmountIndex:          2,
		FeeIndex:             3,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        5,
	}

	parser := NewRevolutParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"2024-01-10", "Description", "invalid", "0.00", "EUR"},
	}

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Status != txn.TransactionInvalid {
		t.Errorf("Expected status TransactionInvalid, got %v", results[0].Status)
	}
	if results[0].ErrorMsg == "" {
		t.Error("Expected error message for invalid amount")
	}
}

func TestRevolutParser_Parse_InvalidFee(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     1,
		AmountIndex:          2,
		FeeIndex:             3,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        5,
	}

	parser := NewRevolutParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"2024-01-10", "Description", "-45.50", "invalid_fee", "EUR"},
	}

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Status != txn.TransactionInvalid {
		t.Errorf("Expected status TransactionInvalid, got %v", results[0].Status)
	}
	if results[0].ErrorMsg == "" {
		t.Error("Expected error message for invalid fee")
	}
}

func TestRevolutParser_Parse_InvalidDate(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     1,
		AmountIndex:          2,
		FeeIndex:             3,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        5,
	}

	parser := NewRevolutParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"invalid-date", "Description", "-45.50", "0.00", "EUR"},
	}

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Status != txn.TransactionInvalid {
		t.Errorf("Expected status TransactionInvalid, got %v", results[0].Status)
	}
	if results[0].ErrorMsg == "" {
		t.Error("Expected error message for invalid date")
	}
}

func TestRevolutParser_Parse_FeeCalculation(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		amount         string
		fee            string
		expectedAmount float64
	}{
		{
			name:           "No fee",
			amount:         "-100.00",
			fee:            "0.00",
			expectedAmount: -100.00, // -100 - 0 = -100
		},
		{
			name:           "Negative fee (charge)",
			amount:         "-100.00",
			fee:            "-2.50",
			expectedAmount: -97.50, // -100 - (-2.50) = -97.50
		},
		{
			name:           "Positive amount no fee",
			amount:         "500.00",
			fee:            "0.00",
			expectedAmount: 500.00, // 500 - 0 = 500
		},
		{
			name:           "Positive amount with fee",
			amount:         "500.00",
			fee:            "-5.00",
			expectedAmount: 505.00, // 500 - (-5) = 505
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				TransactionDateIndex: 0,
				DescriptionIndex:     1,
				AmountIndex:          2,
				FeeIndex:             3,
				DateFormat:           "2006-01-02",
				ColumnsAmount:        5,
			}

			parser := NewRevolutParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
			account := txn.BankAccount{ID: "acc-123", Name: "Test"}

			data := [][]string{
				{"2024-01-10", "Test Transaction", tt.amount, tt.fee, "EUR"},
			}

			results := parser.Parse(account, data)

			if len(results) != 1 {
				t.Fatalf("Expected 1 transaction, got %d", len(results))
			}

			if results[0].Amount != tt.expectedAmount {
				t.Errorf("Expected amount %f, got %f", tt.expectedAmount, results[0].Amount)
			}
		})
	}
}

func TestRevolutParser_Parse_HeaderHandling(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		hasHeader      bool
		data           [][]string
		expectedCount  int
		expectedStatus txn.TransactionStatus
	}{
		{
			name:      "With header - should skip first row",
			hasHeader: true,
			data: [][]string{
				{"Date", "Description", "Amount", "Fee", "Currency"},
				{"2024-01-10", "Test", "-45.50", "0.00", "EUR"},
			},
			expectedCount:  1,
			expectedStatus: txn.TransactionDraft,
		},
		{
			name:      "Without header - should parse all rows",
			hasHeader: false,
			data: [][]string{
				{"2024-01-10", "Test", "-45.50", "0.00", "EUR"},
			},
			expectedCount:  1,
			expectedStatus: txn.TransactionDraft,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				TransactionDateIndex: 0,
				DescriptionIndex:     1,
				AmountIndex:          2,
				FeeIndex:             3,
				DateFormat:           "2006-01-02",
				ColumnsAmount:        5,
				Header:               HeaderCfg{HasHeader: tt.hasHeader},
			}

			parser := NewRevolutParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
			account := txn.BankAccount{ID: "acc-123", Name: "Test"}

			results := parser.Parse(account, tt.data)

			if len(results) != tt.expectedCount {
				t.Errorf("Expected %d transactions, got %d", tt.expectedCount, len(results))
			}

			if len(results) > 0 && results[0].Status != tt.expectedStatus {
				t.Errorf("Expected status %v, got %v", tt.expectedStatus, results[0].Status)
			}
		})
	}
}

func TestRevolutParser_UniqueIDs(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     1,
		AmountIndex:          2,
		FeeIndex:             3,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        5,
	}

	parser := NewRevolutParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"2024-01-10", "Transaction 1", "-45.50", "0.00", "EUR"},
		{"2024-01-11", "Transaction 2", "-50.00", "0.00", "EUR"},
	}

	results := parser.Parse(account, data)

	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}

	if results[0].ID == results[1].ID {
		t.Error("Expected unique IDs for different transactions")
	}
	if results[0].ID == "" || results[1].ID == "" {
		t.Error("Expected non-empty IDs")
	}
}

func TestRevolutParser_Parse_MultiCurrency(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     1,
		AmountIndex:          2,
		FeeIndex:             3,
		CurrencyIndex:        4,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        5,
	}

	parser := NewRevolutParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"2024-01-10", "EUR Transaction", "-100.00", "0.00", "EUR"},
		{"2024-01-11", "USD Transaction", "-50.00", "0.00", "USD"},
		{"2024-01-12", "PLN Transaction", "-200.00", "0.00", "PLN"},
	}

	results := parser.Parse(account, data)

	if len(results) != 3 {
		t.Fatalf("Expected 3 transactions, got %d", len(results))
	}

	expectedCurrencies := []string{"EUR", "USD", "PLN"}
	for i, expected := range expectedCurrencies {
		if results[i].Currency != expected {
			t.Errorf("Transaction %d: expected currency %s, got %s", i, expected, results[i].Currency)
		}
	}
}
