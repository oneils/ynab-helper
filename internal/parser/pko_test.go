package parser

import (
	"testing"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

func TestPKOParser_Parse_ValidData(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     2,
		AmountIndex:          4,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        9,
		Header:               HeaderCfg{HasHeader: true},
	}

	parser := NewPKOParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})

	account := txn.BankAccount{
		ID:   "acc-123",
		Name: "PKO Test Account",
	}

	data := [][]string{
		{"Date", "Type", "Description", "Details", "Amount", "Currency", "Balance", "BalanceCurrency", "Reference"},
		{"2024-01-10", "Card payment", "Lokalizacja: Adres: BIEDRONKA POZNAN", "Purchase", "-45.50", "PLN", "1500.00", "PLN", "REF123"},
		{"2024-01-11", "Transfer", "Nazwa odbiorcy: ORANGE POLSKA, Address", "Payment", "-89.99", "PLN", "1410.01", "PLN", "REF124"},
		{"2024-01-12", "Transfer", "Nazwa nadawcy: Jan Kowalski", "Income", "2000.00", "PLN", "3410.01", "PLN", "REF125"},
	}

	results := parser.Parse(account, data)

	if len(results) != 3 {
		t.Fatalf("Expected 3 transactions, got %d", len(results))
	}

	// Test first transaction - BIEDRONKA
	tx1 := results[0]
	if tx1.Status != txn.TransactionDraft {
		t.Errorf("Expected status TransactionDraft, got %v", tx1.Status)
	}
	if tx1.Amount != -45.50 {
		t.Errorf("Expected amount -45.50, got %f", tx1.Amount)
	}
	if tx1.Currency != "PLN" {
		t.Errorf("Expected currency PLN, got %s", tx1.Currency)
	}
	if tx1.Payee != "BIEDRONKA POZNAN" {
		t.Errorf("Expected payee 'BIEDRONKA POZNAN', got '%s'", tx1.Payee)
	}
	expectedDate := time.Date(2024, 1, 10, 0, 0, 0, 0, time.FixedZone("Europe/Warsaw", 3600))
	if !tx1.TxnTime.Equal(expectedDate) {
		t.Errorf("Expected date %v, got %v", expectedDate, tx1.TxnTime)
	}

	// Test second transaction - ORANGE POLSKA
	tx2 := results[1]
	if tx2.Payee != "ORANGE POLSKA" {
		t.Errorf("Expected payee 'ORANGE POLSKA', got '%s'", tx2.Payee)
	}
	if tx2.Amount != -89.99 {
		t.Errorf("Expected amount -89.99, got %f", tx2.Amount)
	}

	// Test third transaction - Jan Kowalski
	tx3 := results[2]
	if tx3.Payee != "Jan Kowalski" {
		t.Errorf("Expected payee 'Jan Kowalski', got '%s'", tx3.Payee)
	}
	if tx3.Amount != 2000.00 {
		t.Errorf("Expected amount 2000.00, got %f", tx3.Amount)
	}
}

func TestPKOParser_Parse_EmptyData(t *testing.T) {
	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     2,
		AmountIndex:          4,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        9,
	}

	parser := NewPKOParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: time.Now()})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	results := parser.Parse(account, [][]string{})

	if len(results) != 0 {
		t.Errorf("Expected 0 transactions for empty data, got %d", len(results))
	}
}

func TestPKOParser_Parse_InvalidColumns(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     2,
		AmountIndex:          4,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        9,
	}

	parser := NewPKOParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"2024-01-10", "Card payment", "Description"},
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

func TestPKOParser_Parse_InvalidAmount(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     2,
		AmountIndex:          4,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        9,
	}

	parser := NewPKOParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"2024-01-10", "Card payment", "Description", "Details", "invalid", "PLN", "1500.00", "PLN", "REF123"},
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

func TestPKOParser_Parse_InvalidDate(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     2,
		AmountIndex:          4,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        9,
	}

	parser := NewPKOParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"invalid-date", "Card payment", "Description", "Details", "-45.50", "PLN", "1500.00", "PLN", "REF123"},
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

func TestPKOParser_shopName(t *testing.T) {
	tests := []struct {
		name        string
		description string
		expected    string
	}{
		{
			name:        "Location pattern",
			description: "Lokalizacja: Adres: BIEDRONKA POZNAN",
			expected:    "BIEDRONKA POZNAN",
		},
		{
			name:        "Receiver name pattern",
			description: "Nazwa odbiorcy: ORANGE POLSKA, ul. Marynarska 12",
			expected:    "ORANGE POLSKA",
		},
		{
			name:        "Sender name pattern",
			description: "Nazwa nadawcy: Jan Kowalski",
			expected:    "Jan Kowalski",
		},
		{
			name:        "Multiple patterns - location wins",
			description: "Lokalizacja: Adres: LIDL WARSZAWA Miasto: TEST",
			expected:    "LIDL WARSZAWA",
		},
		{
			name:        "No matching pattern",
			description: "Some other description format",
			expected:    "",
		},
		{
			name:        "Empty description",
			description: "",
			expected:    "",
		},
	}

	parser := NewPKOParser(Config{}, &mockHasher{}, &mockTimeProvider{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.shopName(tt.description)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestPKOParser_Parse_HeaderHandling(t *testing.T) {
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
				{"Date", "Type", "Description", "Details", "Amount", "Currency", "Balance", "BalanceCurrency", "Reference"},
				{"2024-01-10", "Card payment", "Lokalizacja : Adres : SHOP", "Purchase", "-45.50", "PLN", "1500.00", "PLN", "REF123"},
			},
			expectedCount:  1,
			expectedStatus: txn.TransactionDraft,
		},
		{
			name:      "Without header - should parse all rows",
			hasHeader: false,
			data: [][]string{
				{"2024-01-10", "Card payment", "Lokalizacja : Adres : SHOP", "Purchase", "-45.50", "PLN", "1500.00", "PLN", "REF123"},
			},
			expectedCount:  1,
			expectedStatus: txn.TransactionDraft,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				TransactionDateIndex: 0,
				DescriptionIndex:     2,
				AmountIndex:          4,
				DateFormat:           "2006-01-02",
				ColumnsAmount:        9,
				Header:               HeaderCfg{HasHeader: tt.hasHeader},
			}

			parser := NewPKOParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
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

func TestPKOParser_UniqueIDs(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	cfg := Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     2,
		AmountIndex:          4,
		DateFormat:           "2006-01-02",
		ColumnsAmount:        9,
	}

	parser := NewPKOParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"2024-01-10", "Card payment", "Lokalizacja : Adres : SHOP1", "Purchase", "-45.50", "PLN", "1500.00", "PLN", "REF123"},
		{"2024-01-11", "Card payment", "Lokalizacja : Adres : SHOP2", "Purchase", "-50.00", "PLN", "1450.00", "PLN", "REF124"},
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
