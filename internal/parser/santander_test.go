package parser

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

// mockHasher is a test double for Hasher interface
type mockHasher struct {
	data []byte
}

func (m *mockHasher) Write(p []byte) (n int, err error) {
	m.data = append(m.data, p...)
	return len(p), nil
}

func (m *mockHasher) Sum(b []byte) []byte {
	h := sha256.New()
	h.Write(m.data)
	return h.Sum(b)
}

func (m *mockHasher) Reset() {
	m.data = nil
}

// mockTimeProvider is a test double for TimeProvider interface
type mockTimeProvider struct {
	fixedTime time.Time
}

func (m *mockTimeProvider) Now() time.Time {
	return m.fixedTime
}

func TestSantanderParser_Parse_ValidData(t *testing.T) {
	// Arrange
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		BankName:             SantanderBankName,
		ColumnsAmount:        9,
		Header: HeaderCfg{
			HasHeader:      true,
			ValidateHeader: false,
		},
	}

	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})

	acc := txn.BankAccount{
		ID:   "acc123",
		Name: "Santander Account",
	}

	data := [][]string{
		{"Header1", "Header2", "Header3", "Header4", "Header5", "Header6", "Header7", "Header8", "Header9"},
		{"", "15-01-2025", "PŁATNOŚĆ KARTĄ 123.45 PLN BIEDRONKA", "", "", "-123.45", "", "", ""},
		{"", "16-01-2025", "Zakup BLIK LIDL", "", "", "-50.00", "", "", ""},
	}

	// Act
	results := parser.Parse(acc, data)

	// Assert
	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}

	// Test first transaction
	txn1 := results[0]
	if txn1.Status != txn.TransactionDraft {
		t.Errorf("Expected status DRAFT, got %s", txn1.Status)
	}
	if txn1.Amount != -123.45 {
		t.Errorf("Expected amount -123.45, got %f", txn1.Amount)
	}
	if txn1.Payee != "BIEDRONKA" {
		t.Errorf("Expected payee 'BIEDRONKA', got '%s'", txn1.Payee)
	}
	if txn1.Currency != "PLN" {
		t.Errorf("Expected currency PLN, got %s", txn1.Currency)
	}
	if txn1.Account.ID != "acc123" {
		t.Errorf("Expected account ID acc123, got %s", txn1.Account.ID)
	}

	// Test second transaction
	txn2 := results[1]
	if txn2.Payee != "LIDL" {
		t.Errorf("Expected payee 'LIDL', got '%s'", txn2.Payee)
	}
	if txn2.Amount != -50.00 {
		t.Errorf("Expected amount -50.00, got %f", txn2.Amount)
	}
}

func TestSantanderParser_Parse_EmptyData(t *testing.T) {
	// Arrange
	cfg := Config{ColumnsAmount: 9}
	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{})
	acc := txn.BankAccount{ID: "acc123", Name: "Test"}

	// Act
	results := parser.Parse(acc, [][]string{})

	// Assert
	if len(results) != 0 {
		t.Errorf("Expected 0 transactions for empty data, got %d", len(results))
	}
}

func TestSantanderParser_Parse_InvalidColumnCount(t *testing.T) {
	// Arrange
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		ColumnsAmount: 9,
		Header: HeaderCfg{
			HasHeader: false,
		},
	}
	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "Test"}

	data := [][]string{
		{"col1", "col2", "col3"}, // Only 3 columns, expected 9
	}

	// Act
	results := parser.Parse(acc, data)

	// Assert
	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Status != txn.TransactionInvalid {
		t.Errorf("Expected status INVALID, got %s", results[0].Status)
	}

	if results[0].ErrorMsg == "" {
		t.Error("Expected error message for invalid column count")
	}
}

func TestSantanderParser_Parse_InvalidAmount(t *testing.T) {
	// Arrange
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		ColumnsAmount:        9,
		Header: HeaderCfg{
			HasHeader: false,
		},
	}
	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "Test"}

	data := [][]string{
		{"", "15-01-2025", "Description", "", "", "INVALID_AMOUNT", "", "", ""},
	}

	// Act
	results := parser.Parse(acc, data)

	// Assert
	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Status != txn.TransactionInvalid {
		t.Errorf("Expected status INVALID, got %s", results[0].Status)
	}

	if results[0].ErrorMsg == "" {
		t.Error("Expected error message for invalid amount")
	}
}

func TestSantanderParser_Parse_InvalidDate(t *testing.T) {
	// Arrange
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		ColumnsAmount:        9,
		Header: HeaderCfg{
			HasHeader: false,
		},
	}
	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "Test"}

	data := [][]string{
		{"", "INVALID_DATE", "Description", "", "", "-100.00", "", "", ""},
	}

	// Act
	results := parser.Parse(acc, data)

	// Assert
	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Status != txn.TransactionInvalid {
		t.Errorf("Expected status INVALID, got %s", results[0].Status)
	}

	if results[0].ErrorMsg == "" {
		t.Error("Expected error message for invalid date")
	}
}

func TestSantanderParser_shopName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Card payment with amount",
			input:    "PŁATNOŚĆ KARTĄ 123.45 PLN BIEDRONKA WARSZAWA",
			expected: "BIEDRONKA",
		},
		{
			name:     "BLIK purchase",
			input:    "Zakup BLIK LIDL POZNAŃ",
			expected: "LIDL POZNAŃ",
		},
		{
			name:     "Card payment single word shop",
			input:    "PŁATNOŚĆ KARTĄ 50.00 PLN ŻABKA",
			expected: "ŻABKA",
		},
		{
			name:     "No match",
			input:    "Some random text",
			expected: "",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	cfg := Config{}
	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.shopName(tt.input)
			if result != tt.expected {
				t.Errorf("shopName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSantanderParser_Parse_WithHeader(t *testing.T) {
	// Arrange
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		ColumnsAmount:        9,
		Header: HeaderCfg{
			HasHeader: true,
		},
	}
	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "Test"}

	data := [][]string{
		{"Header1", "Header2", "Header3", "Header4", "Header5", "Header6", "Header7", "Header8", "Header9"},
		{"", "15-01-2025", "PŁATNOŚĆ KARTĄ 100 PLN SHOP", "", "", "-100.00", "", "", ""},
	}

	// Act
	results := parser.Parse(acc, data)

	// Assert
	if len(results) != 1 {
		t.Errorf("Expected 1 transaction (header should be skipped), got %d", len(results))
	}
}

func TestSantanderParser_Parse_GeneratesUniqueIDs(t *testing.T) {
	// Arrange
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		ColumnsAmount:        9,
		Header: HeaderCfg{
			HasHeader: false,
		},
	}
	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "Test"}

	data := [][]string{
		{"", "15-01-2025", "Transaction 1", "", "", "-100.00", "", "", ""},
		{"", "16-01-2025", "Transaction 2", "", "", "-200.00", "", "", ""},
	}

	// Act
	results := parser.Parse(acc, data)

	// Assert
	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}

	// IDs should be different
	if results[0].ID == results[1].ID {
		t.Error("Expected different IDs for different transactions")
	}

	// IDs should not be empty
	if results[0].ID == "" || results[1].ID == "" {
		t.Error("Expected non-empty IDs")
	}
}

// TestSantanderParser_HashColumns_SameIDForVaryingBalanceAndCounter verifies that
// two rows identical in columns 1-5 but differing in columns 7-8 (balance/counter)
// produce the same ID, so re-exports don't create duplicates.
func TestSantanderParser_HashColumns_SameIDForVaryingBalanceAndCounter(t *testing.T) {
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		ColumnsAmount:        9,
		Header: HeaderCfg{
			HasHeader: false,
		},
		HashColumns: []int{1, 2, 3, 4, 5},
	}

	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "test-acc", Name: "Test"}

	data := [][]string{
		{"", "15-01-2025", "Zakup BLIK SHOP", "Recipient", "Account123", "-2.99", "", "1000,00", "29"},
		{"", "15-01-2025", "Zakup BLIK SHOP", "Recipient", "Account123", "-2.99", "", "2000,00", "45"},
	}

	results := parser.Parse(acc, data)

	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}

	if results[0].ID != results[1].ID {
		t.Errorf("Expected same ID for rows differing only in balance/counter, got %s vs %s", results[0].ID, results[1].ID)
	}
}

// TestSantanderParser_HashColumns_DifferentIDForDifferentDescription verifies that
// two rows differing in description produce different IDs.
func TestSantanderParser_HashColumns_DifferentIDForDifferentDescription(t *testing.T) {
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		ColumnsAmount:        9,
		Header: HeaderCfg{
			HasHeader: false,
		},
		HashColumns: []int{1, 2, 3, 4, 5},
	}

	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "test-acc", Name: "Test"}

	data := [][]string{
		{"", "15-01-2025", "Zakup BLIK SHOP A", "", "", "-2.99", "", "1000,00", "29"},
		{"", "15-01-2025", "Zakup BLIK SHOP B", "", "", "-2.99", "", "1000,00", "29"},
	}

	results := parser.Parse(acc, data)

	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}

	if results[0].ID == results[1].ID {
		t.Error("Expected different IDs for rows differing in description")
	}
}

// TestSantanderParser_Parse_ValidTransactionHasRawText verifies RawText is populated.
func TestSantanderParser_Parse_ValidTransactionHasRawText(t *testing.T) {
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		ColumnsAmount:        9,
		Header: HeaderCfg{
			HasHeader: false,
		},
		HashColumns: []int{1, 2, 3, 4, 5},
	}

	parser := NewSantanderParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "test-acc", Name: "Test"}

	data := [][]string{
		{"", "15-01-2025", "Zakup BLIK SHOP", "", "", "-2.99", "", "1000,00", "29"},
	}

	results := parser.Parse(acc, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].RawText == "" {
		t.Error("Expected non-empty RawText")
	}
}
