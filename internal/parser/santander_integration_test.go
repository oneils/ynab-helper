package parser

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

// TestSantanderParser_RealWorldData tests parsing with actual Santander CSV format
func TestSantanderParser_RealWorldData(t *testing.T) {
	// Arrange
	fixedTime := time.Date(2025, 12, 29, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		BankName:             SantanderBankName,
		ColumnsAmount:        9,
		Header: HeaderCfg{
			HasHeader:      false,
			ValidateHeader: false,
		},
	}

	parser := NewSantanderParser(cfg, sha256.New(), &mockTimeProvider{fixedTime: fixedTime})

	acc := txn.BankAccount{
		ID:   "santander-acc-123",
		Name: "Santander Personal Account",
	}

	// Real CSV data from Santander export
	data := [][]string{
		{"", "15-12-2025", "VISA SEL 421352******9361 PŁATNOŚĆ KARTĄ 51.91 PLN CARREFOUR EXPRESS E30 WARSZAWA", "", "", "-51,91", "", "", ""},
		{"", "15-12-2025", "VISA SEL 421352******9361 PŁATNOŚĆ KARTĄ 29.99 PLN PETSTATION WARSZAWA", "", "", "-29,99", "", "", ""},
		{"", "15-12-2025", "Zakup BLIK JMP S.A. BIEDRONKA AL. RZECZYPOSPOLITEJ . ref:92552753374", "JMP S.A. BIEDRONKA AL. RZECZYPOSPOLITEJ .", "72 1090 1489 0000 0000 4800 3393", "-2,99", "", "6717,38", "29"},
		{"", "15-12-2025", "VISA SEL 421352******9361 PŁATNOŚĆ KARTĄ 3999.00 PLN APPLE.COM/PL 801-934-999", "", "", "-3999,00", "", "", ""},
		{"", "15-12-2025", "VISA SEL 421352******9361 PŁATNOŚĆ KARTĄ 215.00 PLN CHMELI SUNELI WARSZAWA", "", "", "-215,00", "", "", ""},
	}

	// Act
	results := parser.Parse(acc, data)

	// Assert
	if len(results) != 5 {
		t.Fatalf("Expected 5 transactions, got %d", len(results))
	}

	// Test 1: CARREFOUR transaction
	t.Run("CARREFOUR transaction", func(t *testing.T) {
		transaction := results[0]

		if transaction.Status != txn.TransactionDraft {
			t.Errorf("Expected status DRAFT, got %s", transaction.Status)
		}
		if transaction.Amount != -51.91 {
			t.Errorf("Expected amount -51.91, got %f", transaction.Amount)
		}
		if transaction.Payee != "CARREFOUR" {
			t.Errorf("Expected payee 'CARREFOUR', got '%s'", transaction.Payee)
		}
		if transaction.Currency != "PLN" {
			t.Errorf("Expected currency PLN, got %s", transaction.Currency)
		}

		expectedDate := time.Date(2025, 12, 15, 0, 0, 0, 0, time.FixedZone("Europe/Warsaw", 3600))
		if !transaction.TxnTime.Equal(expectedDate) {
			t.Errorf("Expected date %v, got %v", expectedDate, transaction.TxnTime)
		}
	})

	// Test 2: PETSTATION transaction
	t.Run("PETSTATION transaction", func(t *testing.T) {
		transaction := results[1]

		if transaction.Payee != "PETSTATION" {
			t.Errorf("Expected payee 'PETSTATION', got '%s'", transaction.Payee)
		}
		if transaction.Amount != -29.99 {
			t.Errorf("Expected amount -29.99, got %f", transaction.Amount)
		}
	})

	// Test 3: BLIK BIEDRONKA transaction
	t.Run("BLIK BIEDRONKA transaction", func(t *testing.T) {
		transaction := results[2]

		// Parser extracts "JMP S" because of the space splitting logic
		if transaction.Payee != "JMP S" {
			t.Errorf("Expected payee 'JMP S', got '%s'", transaction.Payee)
		}
		if transaction.Amount != -2.99 {
			t.Errorf("Expected amount -2.99, got %f", transaction.Amount)
		}
	})

	// Test 4: APPLE.COM transaction (large amount)
	t.Run("APPLE.COM transaction", func(t *testing.T) {
		transaction := results[3]

		// Parser extracts first word, which is just "APPLE" from "APPLE.COM/PL"
		if transaction.Payee != "APPLE" {
			t.Errorf("Expected payee 'APPLE', got '%s'", transaction.Payee)
		}
		if transaction.Amount != -3999.00 {
			t.Errorf("Expected amount -3999.00, got %f", transaction.Amount)
		}
	})

	// Test 5: CHMELI SUNELI transaction (Polish characters)
	t.Run("CHMELI SUNELI transaction", func(t *testing.T) {
		transaction := results[4]

		if transaction.Payee != "CHMELI" {
			t.Errorf("Expected payee 'CHMELI', got '%s'", transaction.Payee)
		}
		if transaction.Amount != -215.00 {
			t.Errorf("Expected amount -215.00, got %f", transaction.Amount)
		}
	})
}

// TestSantanderParser_EdgeCases tests edge cases found in real data
func TestSantanderParser_EdgeCases(t *testing.T) {
	fixedTime := time.Date(2025, 12, 29, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		BankName:             SantanderBankName,
		ColumnsAmount:        9,
		Header: HeaderCfg{
			HasHeader: false,
		},
	}

	parser := NewSantanderParser(cfg, sha256.New(), &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "test-acc", Name: "Test"}

	t.Run("Comma as decimal separator", func(t *testing.T) {
		data := [][]string{
			{"", "15-12-2025", "Test transaction", "", "", "-1234,56", "", "", ""},
		}

		results := parser.Parse(acc, data)

		if len(results) != 1 {
			t.Fatalf("Expected 1 transaction, got %d", len(results))
		}

		if results[0].Amount != -1234.56 {
			t.Errorf("Expected amount -1234.56, got %f", results[0].Amount)
		}
	})

	t.Run("Transaction with long shop name", func(t *testing.T) {
		data := [][]string{
			{"", "15-12-2025", "VISA SEL 421352******9361 PŁATNOŚĆ KARTĄ 100.00 PLN LONG SHOP NAME WITH MULTIPLE WORDS WARSZAWA", "", "", "-100,00", "", "", ""},
		}

		results := parser.Parse(acc, data)

		if len(results) != 1 {
			t.Fatalf("Expected 1 transaction, got %d", len(results))
		}

		// Should extract first word after amount
		if results[0].Payee != "LONG" {
			t.Errorf("Expected payee 'LONG', got '%s'", results[0].Payee)
		}
	})

	t.Run("UBER transaction with asterisk", func(t *testing.T) {
		data := [][]string{
			{"", "25-12-2025", "VISA SEL 421352******9361 PŁATNOŚĆ KARTĄ 2.00 PLN UBER *TRIP AMSTERDAM", "", "", "-2,00", "", "", ""},
		}

		results := parser.Parse(acc, data)

		if len(results) != 1 {
			t.Fatalf("Expected 1 transaction, got %d", len(results))
		}

		if results[0].Payee != "UBER" {
			t.Errorf("Expected payee 'UBER', got '%s'", results[0].Payee)
		}
		if results[0].Amount != -2.00 {
			t.Errorf("Expected amount -2.00, got %f", results[0].Amount)
		}
	})
}

// TestSantanderParser_UniqueTransactionIDs ensures each transaction gets a unique ID
func TestSantanderParser_UniqueTransactionIDs(t *testing.T) {
	fixedTime := time.Date(2025, 12, 29, 12, 0, 0, 0, time.UTC)
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

	parser := NewSantanderParser(cfg, sha256.New(), &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "test-acc", Name: "Test"}

	data := [][]string{
		{"", "15-12-2025", "Transaction 1", "", "", "-100,00", "", "", ""},
		{"", "15-12-2025", "Transaction 2", "", "", "-200,00", "", "", ""},
		{"", "16-12-2025", "Transaction 1", "", "", "-100,00", "", "", ""}, // Same description, different date
	}

	results := parser.Parse(acc, data)

	if len(results) != 3 {
		t.Fatalf("Expected 3 transactions, got %d", len(results))
	}

	// Check all IDs are unique
	ids := make(map[string]bool)
	for i, transaction := range results {
		if transaction.ID == "" {
			t.Errorf("Transaction %d has empty ID", i)
		}
		if ids[transaction.ID] {
			t.Errorf("Duplicate ID found: %s", transaction.ID)
		}
		ids[transaction.ID] = true
	}
}

// TestSantanderParser_AllTransactionsDraft ensures all valid transactions are marked as DRAFT
func TestSantanderParser_AllTransactionsDraft(t *testing.T) {
	fixedTime := time.Date(2025, 12, 29, 12, 0, 0, 0, time.UTC)
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

	parser := NewSantanderParser(cfg, sha256.New(), &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "test-acc", Name: "Test"}

	data := [][]string{
		{"", "15-12-2025", "VISA SEL PŁATNOŚĆ KARTĄ 100.00 PLN SHOP1", "", "", "-100,00", "", "", ""},
		{"", "16-12-2025", "Zakup BLIK SHOP2", "", "", "-50,00", "", "", ""},
		{"", "17-12-2025", "VISA SEL PŁATNOŚĆ KARTĄ 75.50 PLN SHOP3", "", "", "-75,50", "", "", ""},
	}

	results := parser.Parse(acc, data)

	for i, transaction := range results {
		if transaction.Status != txn.TransactionDraft {
			t.Errorf("Transaction %d: expected status DRAFT, got %s", i, transaction.Status)
		}
	}
}
