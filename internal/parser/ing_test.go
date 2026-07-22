package parser

import (
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"github.com/oneils/ynab-helper/internal/txn"
)

func ingConfig() Config {
	return Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     3,
		AmountIndex:          8,
		CurrencyIndex:        9,
		DateFormat:           "2006-01-02",
		BankName:             INGBankName,
		ColumnsAmount:        21,
		Header:               HeaderCfg{HasHeader: false},
		HashColumns:          []int{0, 2, 3, 7, 8, 9},
	}
}

// ingRow builds a 21-column ING transaction row, overriding only the fields
// that matter for a given test via the positional arguments.
func ingRow(date, counterparty, description, txnType, txnID, amount, currency, balance string) []string {
	return []string{
		date,           // 0 Data transakcji
		date,           // 1 Data księgowania
		counterparty,   // 2 Dane kontrahenta
		description,    // 3 Tytuł
		"12345678",     // 4 Nr rachunku
		"ING Bank",     // 5 Nazwa banku
		txnType,        // 6 Szczegóły
		txnID,          // 7 Nr transakcji
		amount,         // 8 Kwota transakcji
		currency,       // 9 Waluta
		"",             // 10 Kwota blokady
		"",             // 11 Waluta
		"",             // 12 Kwota płatności w walucie
		"",             // 13 Waluta
		balance,        // 14 Saldo po transakcji
		currency,       // 15 Waluta
		"", "", "", "", "", // 16-20 padding
	}
}

func TestINGParser_Parse_EmptyData(t *testing.T) {
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	results := parser.Parse(acc, [][]string{})

	if len(results) != 0 {
		t.Errorf("Expected 0 transactions for empty data, got %d", len(results))
	}
}

func TestINGParser_Parse_DirectData_NoHeader(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		ingRow("2026-01-15", "Shop A", "TR.KART Shop A", "TR.KART", "1", "-100,00", "PLN", "900,00"),
		ingRow("2026-01-16", "Shop B", "TR.KART Shop B", "TR.KART", "2", "-50,00", "PLN", "850,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}
}

func TestINGParser_Parse_WithPreambleAndHeader(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		{"Klient:", "Jan Kowalski"},
		{"Numer rachunku:", "12345678"},
		{"Data transakcji", "Data księgowania", "Dane kontrahenta", "Tytuł"},
		ingRow("2026-01-15", "Shop A", "TR.KART Shop A", "TR.KART", "1", "-100,00", "PLN", "900,00"),
		ingRow("2026-01-16", "Shop B", "TR.KART Shop B", "TR.KART", "2", "-50,00", "PLN", "850,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}
}

func TestINGParser_Parse_ValidCardPayment(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		ingRow("2026-01-15", "Shop Name", "TR.KART Shop Name", "TR.KART", "1", "-123,45", "PLN", "900,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	got := results[0]
	if got.Status != txn.TransactionDraft {
		t.Errorf("Expected status DRAFT, got %s", got.Status)
	}
	if got.Amount != -123.45 {
		t.Errorf("Expected amount -123.45, got %f", got.Amount)
	}
	if got.Payee != "Shop Name" {
		t.Errorf("Expected payee 'Shop Name', got %q", got.Payee)
	}
	if got.Description != "TR.KART Shop Name" {
		t.Errorf("Expected description 'TR.KART Shop Name', got %q", got.Description)
	}
	if got.Currency != "PLN" {
		t.Errorf("Expected currency PLN, got %s", got.Currency)
	}
	expectedTime := time.Date(2026, 1, 15, 0, 0, 0, 0, got.TxnTime.Location())
	if !got.TxnTime.Equal(expectedTime) {
		t.Errorf("Expected TxnTime %v, got %v", expectedTime, got.TxnTime)
	}
}

func TestINGParser_Parse_Windows1250Diacritics(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	payee := "Żółć Śpiewająca Łąka"
	description := "Zażółć gęślą jaźń"

	encodedPayee, _, err := transform.String(charmap.Windows1250.NewEncoder(), payee)
	if err != nil {
		t.Fatalf("encoding payee to Windows-1250: %s", err)
	}
	encodedDescription, _, err := transform.String(charmap.Windows1250.NewEncoder(), description)
	if err != nil {
		t.Fatalf("encoding description to Windows-1250: %s", err)
	}

	data := [][]string{
		ingRow("2026-01-15", encodedPayee, encodedDescription, "TR.KART", "1", "-100,00", "PLN", "900,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	got := results[0]
	if got.Payee != payee {
		t.Errorf("Expected payee %q, got %q", payee, got.Payee)
	}
	if got.Description != description {
		t.Errorf("Expected description %q, got %q", description, got.Description)
	}
}

func TestINGParser_Parse_ValidWireTransfer(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		ingRow("2026-01-15", "John Doe", "Salary payment", "PRZELEW", "1", "5000,00", "PLN", "5900,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	got := results[0]
	if got.Amount != 5000.00 {
		t.Errorf("Expected amount 5000.00, got %f", got.Amount)
	}
	if got.Payee != "John Doe" {
		t.Errorf("Expected payee 'John Doe', got %q", got.Payee)
	}
}

func TestINGParser_Parse_ValidBLIKPayment(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		ingRow("2026-01-15", "Store", "TR.BLIK Store", "TR.BLIK", "1", "-30,00", "PLN", "870,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Amount != -30.00 {
		t.Errorf("Expected amount -30.00, got %f", results[0].Amount)
	}
}

func TestINGParser_Parse_BlockEntrySkipped(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	row := ingRow("2026-01-15", "Shop", "TR.KART Shop", "TR.KART", "1", "", "PLN", "900,00")
	row[10] = "-100,00" // block amount populated, col 8 empty

	results := parser.Parse(acc, [][]string{row})

	if len(results) != 0 {
		t.Errorf("Expected 0 transactions for block entry, got %d", len(results))
	}
}

func TestINGParser_Parse_FooterRowSkipped(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		{"Dokument ma charakter informacyjny i nie stanowi potwierdzenia wykonania transakcji"},
	}

	results := parser.Parse(acc, data)

	if len(results) != 0 {
		t.Errorf("Expected 0 transactions for footer row, got %d", len(results))
	}
}

func TestINGParser_Parse_InvalidDate(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		ingRow("INVALID_DATE", "Shop", "TR.KART Shop", "TR.KART", "1", "-100,00", "PLN", "900,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Status != txn.TransactionInvalid {
		t.Errorf("Expected status INVALID, got %s", results[0].Status)
	}
	if results[0].ErrorMsg == "" {
		t.Error("Expected non-empty ErrorMsg for invalid date")
	}
}

func TestINGParser_Parse_InvalidAmount(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		ingRow("2026-01-15", "Shop", "TR.KART Shop", "TR.KART", "1", "INVALID_AMOUNT", "PLN", "900,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}

	if results[0].Status != txn.TransactionInvalid {
		t.Errorf("Expected status INVALID, got %s", results[0].Status)
	}
	if results[0].ErrorMsg == "" {
		t.Error("Expected non-empty ErrorMsg for invalid amount")
	}
}

func TestINGParser_Parse_RawTextPopulated(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		ingRow("2026-01-15", "Shop", "TR.KART Shop", "TR.KART", "1", "-100,00", "PLN", "900,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(results))
	}
	if results[0].RawText == "" {
		t.Error("Expected non-empty RawText")
	}
}

func TestINGParser_HashColumns_StableAcrossReExport(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		ingRow("2026-01-15", "Shop", "TR.KART Shop", "TR.KART", "1", "-100,00", "PLN", "900,00"),
		ingRow("2026-01-15", "Shop", "TR.KART Shop", "TR.KART", "1", "-100,00", "PLN", "1900,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}
	if results[0].ID != results[1].ID {
		t.Errorf("Expected same ID for rows differing only in balance, got %s vs %s", results[0].ID, results[1].ID)
	}
}

func TestINGParser_HashColumns_DifferentForDifferentAmount(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		ingRow("2026-01-15", "Shop", "TR.KART Shop", "TR.KART", "1", "-100,00", "PLN", "900,00"),
		ingRow("2026-01-15", "Shop", "TR.KART Shop", "TR.KART", "1", "-200,00", "PLN", "900,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}
	if results[0].ID == results[1].ID {
		t.Error("Expected different IDs for rows differing in amount")
	}
}

func TestINGParser_UniqueIDs(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	parser := NewINGParser(ingConfig(), &mockHasher{}, &mockTimeProvider{fixedTime: fixedTime})
	acc := txn.BankAccount{ID: "acc123", Name: "ING Account"}

	data := [][]string{
		ingRow("2026-01-15", "Shop A", "TR.KART Shop A", "TR.KART", "1", "-100,00", "PLN", "900,00"),
		ingRow("2026-01-16", "Shop B", "TR.KART Shop B", "TR.KART", "2", "-50,00", "PLN", "850,00"),
	}

	results := parser.Parse(acc, data)

	if len(results) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(results))
	}
	if results[0].ID == "" || results[1].ID == "" {
		t.Error("Expected non-empty IDs")
	}
	if results[0].ID == results[1].ID {
		t.Error("Expected different IDs for different transactions")
	}
}
