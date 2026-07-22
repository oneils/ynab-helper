package parser

import (
	"testing"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

func milleniumConfigForTest() Config {
	return Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     6,
		AmountIndex:          7,
		CurrencyIndex:        10,
		DateFormat:           "2006-01-02",
		BankName:             MilleniumBankName,
		ColumnsAmount:        11,
		Header:               HeaderCfg{HasHeader: true},
	}
}

func TestMilleniumParser_Parse_EmptyData(t *testing.T) {
	parser := NewMilleniumParser(milleniumConfigForTest(), &mockHasher{}, &mockTimeProvider{fixedTime: time.Now()})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	results := parser.Parse(account, [][]string{})

	if len(results) != 0 {
		t.Errorf("Expected 0 transactions for empty data, got %d", len(results))
	}
}

func TestMilleniumParser_Parse_ValidDebit(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	parser := NewMilleniumParser(milleniumConfigForTest(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Millennium Test"}

	data := [][]string{
		{"Numer", "Data transakcji", "Data rozliczenia", "Rodzaj", "Na konto", "Odbiorca", "Opis", "Obciążenia", "Uznania", "Saldo", "Waluta"},
		{"123", "2026-06-29", "2026-06-29", "ZAKUP - FIZ. UŻYCIE KARTY", "", "", "SHOP WARSZAWA", "-16.55", "", "1000.00", "PLN"},
	}

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	tx := results[0]
	if tx.Status != txn.TransactionDraft {
		t.Errorf("Expected status TransactionDraft, got %v", tx.Status)
	}
	if tx.Amount != -16.55 {
		t.Errorf("Expected amount -16.55, got %f", tx.Amount)
	}
	if tx.Payee != "SHOP WARSZAWA" {
		t.Errorf("Expected payee 'SHOP WARSZAWA', got '%s'", tx.Payee)
	}
	if tx.Description != "SHOP WARSZAWA" {
		t.Errorf("Expected description 'SHOP WARSZAWA', got '%s'", tx.Description)
	}
	if tx.Currency != "PLN" {
		t.Errorf("Expected currency PLN, got %s", tx.Currency)
	}
	expectedDate := time.Date(2026, 6, 29, 0, 0, 0, 0, time.FixedZone("Europe/Warsaw", 7200))
	if !tx.TxnTime.Equal(expectedDate) {
		t.Errorf("Expected date %v, got %v", expectedDate, tx.TxnTime)
	}
}

func TestMilleniumParser_Parse_ValidCredit(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	parser := NewMilleniumParser(milleniumConfigForTest(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Millennium Test"}

	data := [][]string{
		{"Numer", "Data transakcji", "Data rozliczenia", "Rodzaj", "Na konto", "Odbiorca", "Opis", "Obciążenia", "Uznania", "Saldo", "Waluta"},
		{"123", "2026-06-29", "2026-06-29", "PRZELEW", "", "", "WYPLATA", "", "87.10", "1000.00", "PLN"},
	}

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	tx := results[0]
	if tx.Amount != 87.10 {
		t.Errorf("Expected amount 87.10, got %f", tx.Amount)
	}
}

func TestMilleniumParser_Parse_DebitSignIsPreserved(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	account := txn.BankAccount{ID: "acc-123", Name: "Millennium Test"}

	data := [][]string{
		{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "-23.00", "", "1000.00", "PLN"},
	}

	// no header this time
	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Amount != -23.00 {
		t.Errorf("Expected amount -23.00 (sign preserved from column value), got %f", results[0].Amount)
	}
}

func TestMilleniumParser_Parse_InvalidColumns(t *testing.T) {
	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: time.Now()})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"123", "2026-06-29", "Description"},
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

func TestMilleniumParser_Parse_BothAmountsEmpty(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "", "", "1000.00", "PLN"},
	}

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}
	if results[0].Status != txn.TransactionInvalid {
		t.Errorf("Expected status TransactionInvalid, got %v", results[0].Status)
	}
	if results[0].ErrorMsg == "" {
		t.Error("Expected error message when both amount columns are empty")
	}
}

func TestMilleniumParser_Parse_InvalidAmount(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		row  []string
	}{
		{
			name: "bad debit value",
			row:  []string{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "abc", "", "1000.00", "PLN"},
		},
		{
			name: "bad credit value",
			row:  []string{"123", "2026-06-29", "2026-06-29", "PRZELEW", "", "", "SHOP", "", "xyz", "1000.00", "PLN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := milleniumConfigForTest()
			cfg.Header = HeaderCfg{HasHeader: false}
			parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
			account := txn.BankAccount{ID: "acc-123", Name: "Test"}

			results := parser.Parse(account, [][]string{tt.row})

			if len(results) != 1 {
				t.Fatalf("Expected 1 transaction, got %d", len(results))
			}
			if results[0].Status != txn.TransactionInvalid {
				t.Errorf("Expected status TransactionInvalid, got %v", results[0].Status)
			}
			if results[0].ErrorMsg == "" {
				t.Error("Expected error message for invalid amount")
			}
		})
	}
}

func TestMilleniumParser_Parse_InvalidDate(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"123", "invalid-date", "2026-06-29", "ZAKUP", "", "", "SHOP", "-16.55", "", "1000.00", "PLN"},
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

func TestMilleniumParser_Parse_HeaderHandling(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		hasHeader     bool
		data          [][]string
		expectedCount int
	}{
		{
			name:      "With header - should skip first row",
			hasHeader: true,
			data: [][]string{
				{"Numer", "Data transakcji", "Data rozliczenia", "Rodzaj", "Na konto", "Odbiorca", "Opis", "Obciążenia", "Uznania", "Saldo", "Waluta"},
				{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "-16.55", "", "1000.00", "PLN"},
			},
			expectedCount: 1,
		},
		{
			name:      "Without header - should parse all rows",
			hasHeader: false,
			data: [][]string{
				{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "-16.55", "", "1000.00", "PLN"},
			},
			expectedCount: 1,
		},
		{
			name:      "Header-only file - no transactions",
			hasHeader: true,
			data: [][]string{
				{"Numer", "Data transakcji", "Data rozliczenia", "Rodzaj", "Na konto", "Odbiorca", "Opis", "Obciążenia", "Uznania", "Saldo", "Waluta"},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := milleniumConfigForTest()
			cfg.Header = HeaderCfg{HasHeader: tt.hasHeader}

			parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
			account := txn.BankAccount{ID: "acc-123", Name: "Test"}

			results := parser.Parse(account, tt.data)

			if len(results) != tt.expectedCount {
				t.Errorf("Expected %d transactions, got %d", tt.expectedCount, len(results))
			}
		})
	}
}

func TestMilleniumParser_Parse_PolishCharacters(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	opis := "Żabka Sklep Ząb"
	data := [][]string{
		{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", opis, "-16.55", "", "1000.00", "PLN"},
	}

	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}
	if results[0].Payee != opis {
		t.Errorf("Expected payee %q, got %q", opis, results[0].Payee)
	}
	if results[0].Description != opis {
		t.Errorf("Expected description %q, got %q", opis, results[0].Description)
	}
}

func TestMilleniumParser_Parse_BothAmountsPopulated(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "-16.55", "87.10", "1000.00", "PLN"},
	}

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}
	if results[0].Amount != -16.55 {
		t.Errorf("Expected debit column to take precedence when both are populated, got %f", results[0].Amount)
	}
}

// TestMilleniumParser_HashColumns_SameIDForVaryingSettlementDateAndSaldo verifies that
// two rows identical in columns 0,1,3-8,10 but differing in settlement date (col 2)
// and Saldo (col 9) produce the same ID, so re-exports don't create duplicates.
func TestMilleniumParser_HashColumns_SameIDForVaryingSettlementDateAndSaldo(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	cfg.HashColumns = []int{0, 1, 3, 4, 5, 6, 7, 8, 10}

	parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "-16.55", "", "1000.00", "PLN"},
		{"123", "2026-06-29", "2026-07-01", "ZAKUP", "", "", "SHOP", "-16.55", "", "1050.00", "PLN"},
	}

	results := parser.Parse(account, data)

	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}
	if results[0].ID != results[1].ID {
		t.Errorf("Expected same ID for rows differing only in settlement date/Saldo, got %s vs %s", results[0].ID, results[1].ID)
	}
}

// TestMilleniumParser_HashColumns_DifferentIDForDifferentDescriptionOrAmount verifies that
// two rows differing in description or amount produce different IDs.
func TestMilleniumParser_HashColumns_DifferentIDForDifferentDescriptionOrAmount(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	cfg.HashColumns = []int{0, 1, 3, 4, 5, 6, 7, 8, 10}

	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	t.Run("different description", func(t *testing.T) {
		parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
		data := [][]string{
			{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP A", "-16.55", "", "1000.00", "PLN"},
			{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP B", "-16.55", "", "1000.00", "PLN"},
		}
		results := parser.Parse(account, data)
		if len(results) != 2 {
			t.Fatalf("Expected 2 transactions, got %d", len(results))
		}
		if results[0].ID == results[1].ID {
			t.Error("Expected different IDs for rows differing in description")
		}
	})

	t.Run("different amount", func(t *testing.T) {
		parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
		data := [][]string{
			{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "-16.55", "", "1000.00", "PLN"},
			{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "-20.00", "", "1000.00", "PLN"},
		}
		results := parser.Parse(account, data)
		if len(results) != 2 {
			t.Fatalf("Expected 2 transactions, got %d", len(results))
		}
		if results[0].ID == results[1].ID {
			t.Error("Expected different IDs for rows differing in amount")
		}
	})
}

// TestMilleniumParser_HashColumns_DifferentNumerProducesDifferentID verifies that
// two rows with different Numer (col 0) and otherwise identical fields produce
// different IDs, since Numer contributes to uniqueness.
func TestMilleniumParser_HashColumns_DifferentNumerProducesDifferentID(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	cfg.HashColumns = []int{0, 1, 3, 4, 5, 6, 7, 8, 10}

	parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "-16.55", "", "1000.00", "PLN"},
		{"124", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "-16.55", "", "1000.00", "PLN"},
	}

	results := parser.Parse(account, data)

	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}
	if results[0].ID == results[1].ID {
		t.Error("Expected different IDs for rows differing in Numer")
	}
}

// TestMilleniumParser_Parse_ValidTransactionHasRawText verifies RawText is populated.
func TestMilleniumParser_Parse_ValidTransactionHasRawText(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	cfg.HashColumns = []int{0, 1, 3, 4, 5, 6, 7, 8, 10}

	parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP", "-16.55", "", "1000.00", "PLN"},
	}

	results := parser.Parse(account, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}
	if results[0].RawText == "" {
		t.Error("Expected non-empty RawText for valid transaction")
	}
}

func TestMilleniumParser_UniqueIDs(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	account := txn.BankAccount{ID: "acc-123", Name: "Test"}

	data := [][]string{
		{"123", "2026-06-29", "2026-06-29", "ZAKUP", "", "", "SHOP1", "-16.55", "", "1000.00", "PLN"},
		{"124", "2026-06-30", "2026-06-30", "ZAKUP", "", "", "SHOP2", "-20.00", "", "980.00", "PLN"},
	}

	cfg := milleniumConfigForTest()
	cfg.Header = HeaderCfg{HasHeader: false}
	parser := NewMilleniumParser(cfg, &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})

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
