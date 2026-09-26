package txn

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/oneils/ynab-helper/internal/ynab"
)

// TransactionStorer defines storage operations for transactions.
type TransactionStorer interface {
	InsertTransaction(ctx context.Context, t Transaction) error
	FetchTransactionsByAccount(ctx context.Context, accID, status, sortOrder string) ([]Transaction, error)
	FindTransactionByID(ctx context.Context, id string) (Transaction, error)
	UpdateTransactionStatus(ctx context.Context, id string, status TransactionStatus) error
	CountByStatus(ctx context.Context, accountID string) (map[TransactionStatus]int, error)
}

// BudgetFinder defines budget lookup operations.
type BudgetFinder interface {
	FindBudgetByAccountID(ctx context.Context, accID string) (ynab.Budget, error)
}

// ReportParser parses bank CSV reports into transactions.
type ReportParser interface {
	Parse(acc BankAccount, data [][]string) []Transaction
}

// YnabUploader uploads transactions to YNAB.
type YnabUploader interface {
	Upload(txn ynab.TxnReq) error
}

// ParserMappingLookup looks up the configured parser for an account.
type ParserMappingLookup interface {
	GetParserMapping(ctx context.Context, accountID string) (string, error)
}

// BudgetTxnFinder looks up cached YNAB transactions for match detection.
type BudgetTxnFinder interface {
	FindBudgetTxnMatch(ctx context.Context, accountID string, amountMilliunits int, date time.Time) (ynab.BudgetTxn, bool, error)
}

// Processor handles transaction processing and storage.
type Processor struct {
	parsers          map[string]ReportParser
	txnStore         TransactionStorer
	budgetStore      BudgetFinder
	client           YnabUploader
	suggestionEngine *SuggestionEngine
	mappingStore     ParserMappingLookup
	budgetTxnFinder  BudgetTxnFinder
}

// NewProcessor creates a new transaction processor.
func NewProcessor(parsers map[string]ReportParser, txnStore TransactionStorer, budgetStore BudgetFinder, client YnabUploader, suggestionEngine *SuggestionEngine, mappingStore ParserMappingLookup, budgetTxnFinder BudgetTxnFinder) *Processor {
	return &Processor{
		parsers:          parsers,
		txnStore:         txnStore,
		budgetStore:      budgetStore,
		client:           client,
		suggestionEngine: suggestionEngine,
		mappingStore:     mappingStore,
		budgetTxnFinder:  budgetTxnFinder,
	}
}

// ParserNames returns the names of registered report parsers.
func (p *Processor) ParserNames() []string {
	names := make([]string, 0, len(p.parsers))
	for name := range p.parsers {
		names = append(names, name)
	}
	return names
}

// ProcessParams contains parameters for processing transactions.
type ProcessParams struct {
	Data      [][]string // Pre-parsed CSV data
	BudgetID  string
	AccountID string
	Status    string
	Sort      string
}

// PreviewResult contains preview data for CSV import.
type PreviewResult struct {
	DetectedFormat     string
	TotalCount         int
	DuplicateCount     int
	NewCount           int
	AlreadyInYnabCount int
	ImportableCount    int
	ErrorCount         int
	Transactions       []Transaction // Sample only (first 10)
	ValidationErrors   []string
}

// SaveForm contains form data for saving a transaction to YNAB.
type SaveForm struct {
	TxnID      string
	BudgetID   string
	AccountID  string
	TxnDate    string
	PayeeID    string
	Amount     string
	CategoryID string
	Memo       string
}

// Process parses and stores transactions from pre-parsed CSV data.
func (p *Processor) Process(ctx context.Context, params ProcessParams) error {
	data := params.Data

	budget, err := p.budgetStore.FindBudgetByAccountID(ctx, params.AccountID)
	if err != nil {
		return fmt.Errorf("finding budget: %w", err)
	}

	var accName string
	for _, acc := range budget.Accounts {
		if acc.ID == params.AccountID {
			accName = acc.Name
			break
		}
	}

	parserName, err := p.mappingStore.GetParserMapping(ctx, params.AccountID)
	if err != nil {
		return fmt.Errorf("looking up parser mapping: %w", err)
	}
	if parserName == "" {
		return fmt.Errorf("no parser mapped for account [%s] — configure it in Settings > Parser Mappings", accName)
	}
	reportParser := p.parsers[parserName]
	if reportParser == nil {
		return fmt.Errorf("parser %q is not registered", parserName)
	}

	txns := reportParser.Parse(BankAccount{
		ID:   params.AccountID,
		Name: accName,
	}, data)

	for _, t := range txns {
		if match, found := p.matchExistingYnabTxn(ctx, t); found {
			t.Status = TransactionSkipped
			t.MatchNote = fmt.Sprintf("possible match: existing YNAB txn on %s, matched account/amount", match.Date)
		}
		if err := p.txnStore.InsertTransaction(ctx, t); err != nil {
			return fmt.Errorf("saving transaction: %w", err)
		}
	}

	return nil
}

// matchExistingYnabTxn checks whether a parsed transaction matches an existing
// cached YNAB transaction (account + amount + date window). It fails open:
// a nil finder, an invalid transaction, or a lookup error all result in no match.
func (p *Processor) matchExistingYnabTxn(ctx context.Context, t Transaction) (ynab.BudgetTxn, bool) {
	if p.budgetTxnFinder == nil || t.Status == TransactionInvalid {
		return ynab.BudgetTxn{}, false
	}

	match, found, err := p.budgetTxnFinder.FindBudgetTxnMatch(ctx, t.Account.ID, toMilliunits(t.Amount), t.TxnTime)
	if err != nil {
		slog.Warn("looking up existing YNAB transaction match", "error", err)
		return ynab.BudgetTxn{}, false
	}
	return match, found
}

// Fetch retrieves transactions for an account.
func (p *Processor) Fetch(ctx context.Context, params ProcessParams) ([]Transaction, error) {
	return p.txnStore.FetchTransactionsByAccount(ctx, params.AccountID, params.Status, params.Sort)
}

// Preview parses pre-parsed CSV data and returns preview without saving to database.
func (p *Processor) Preview(ctx context.Context, params ProcessParams) (*PreviewResult, error) {
	if params.Data == nil {
		return nil, fmt.Errorf("no data provided")
	}
	data := params.Data

	budget, err := p.budgetStore.FindBudgetByAccountID(ctx, params.AccountID)
	if err != nil {
		return nil, fmt.Errorf("finding budget: %w", err)
	}

	var accName string
	for _, acc := range budget.Accounts {
		if acc.ID == params.AccountID {
			accName = acc.Name
			break
		}
	}

	parserName, err := p.mappingStore.GetParserMapping(ctx, params.AccountID)
	if err != nil {
		return nil, fmt.Errorf("looking up parser mapping: %w", err)
	}
	if parserName == "" {
		return &PreviewResult{
			ValidationErrors: []string{fmt.Sprintf("no parser mapped for account [%s] — configure it in Settings > Parser Mappings", accName)},
		}, nil
	}
	reportParser := p.parsers[parserName]
	if reportParser == nil {
		return &PreviewResult{
			ValidationErrors: []string{fmt.Sprintf("parser %q is not registered", parserName)},
		}, nil
	}
	detectedFormat := parserName

	// Parse transactions
	txns := reportParser.Parse(BankAccount{
		ID:   params.AccountID,
		Name: accName,
	}, data)

	// Analyze transactions
	result := &PreviewResult{
		DetectedFormat:   detectedFormat,
		TotalCount:       len(txns),
		ValidationErrors: []string{},
	}

	// Check for duplicates and errors
	for i, txn := range txns {
		// Count errors
		if txn.Status == TransactionInvalid {
			result.ErrorCount++
			if txn.ErrorMsg != "" {
				result.ValidationErrors = append(result.ValidationErrors,
					fmt.Sprintf("Line %d: %s", txn.RawLineNumber, txn.ErrorMsg))
			}
		}

		// Check for duplicates by trying to find existing transaction with same ID
		isDuplicate := false
		if txn.ID != "" {
			_, err := p.txnStore.FindTransactionByID(ctx, txn.ID)
			if err == nil {
				// Transaction exists = duplicate
				isDuplicate = true
				result.DuplicateCount++
				txns[i].Status = "DUPLICATE" // Mark as duplicate for display
			}
		}

		// If not already a hash-ID duplicate, check for a field-based match
		// against the cached YNAB transactions.
		if !isDuplicate {
			if _, found := p.matchExistingYnabTxn(ctx, txn); found {
				result.AlreadyInYnabCount++
				txns[i].Status = "ALREADY_IN_YNAB"
			}
		}

		// Add to sample (first 10 transactions)
		if i < 10 {
			result.Transactions = append(result.Transactions, txns[i])
		}
	}

	result.NewCount = result.TotalCount - result.DuplicateCount - result.AlreadyInYnabCount - result.ErrorCount
	result.ImportableCount = result.NewCount + result.AlreadyInYnabCount

	return result, nil
}

// CountByStatus returns the number of transactions per status for a given account.
func (p *Processor) CountByStatus(ctx context.Context, accountID string) (map[TransactionStatus]int, error) {
	return p.txnStore.CountByStatus(ctx, accountID)
}

// FetchByID retrieves a transaction by ID.
func (p *Processor) FetchByID(ctx context.Context, id string) (Transaction, error) {
	return p.txnStore.FindTransactionByID(ctx, id)
}

// Skip marks a transaction as skipped.
func (p *Processor) Skip(ctx context.Context, id string) error {
	return p.txnStore.UpdateTransactionStatus(ctx, id, TransactionSkipped)
}

// SaveToYnab uploads a transaction to YNAB and marks it as processed.
func (p *Processor) SaveToYnab(ctx context.Context, form SaveForm) error {
	_, err := p.parseYnabTime(form.TxnDate)
	if err != nil {
		return fmt.Errorf("parsing time: %w", err)
	}

	amountFloat, err := strconv.ParseFloat(form.Amount, 64)
	if err != nil {
		return fmt.Errorf("parsing amount: %w", err)
	}

	err = p.client.Upload(ynab.TxnReq{
		BudgetID:   form.BudgetID,
		AccountID:  form.AccountID,
		Date:       form.TxnDate,
		Amount:     toMilliunits(amountFloat),
		PayeeID:    form.PayeeID,
		CategoryID: form.CategoryID,
		Memo:       form.Memo,
		Cleared:    "cleared",
		Approved:   true,
	})
	if err != nil {
		return fmt.Errorf("uploading to YNAB: %w", err)
	}

	return p.txnStore.UpdateTransactionStatus(ctx, form.TxnID, TransactionProcessed)
}

// SuggestPayee suggests a payee based on transaction description.
func (p *Processor) SuggestPayee(t Transaction, payees []ynab.Payee) ynab.Payee {
	for _, payee := range payees {
		if t.Payee == "" {
			if strings.Contains(strings.ToLower(normalize(t.Description)), strings.ToLower(normalize(payee.Name))) {
				return payee
			}
		}
		if strings.Contains(strings.ToLower(normalize(t.Payee)), strings.ToLower(normalize(payee.Name))) {
			return payee
		}
	}
	return ynab.Payee{}
}

// toMilliunits converts a decimal currency amount to YNAB's milliunit format.
func toMilliunits(amount float64) int {
	return int(math.Round(amount * 1000))
}

// parseYnabTime parses a date string in YNAB format.
func (p *Processor) parseYnabTime(dateStr string) (string, error) {
	_, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
	}
	return dateStr, nil
}

// GetSmartSuggestions returns intelligent payee suggestions based on transaction description.
func (p *Processor) GetSmartSuggestions(ctx context.Context, budgetID, description string) ([]PayeeSuggestion, error) {
	if p.suggestionEngine == nil {
		return []PayeeSuggestion{}, nil
	}
	return p.suggestionEngine.GetSuggestions(ctx, budgetID, description, 5)
}

// GetCategorySuggestions returns intelligent category suggestions.
// Prioritizes payee-based suggestions if payeeID is provided.
func (p *Processor) GetCategorySuggestions(ctx context.Context, budgetID, description, payeeID string) ([]CategorySuggestion, error) {
	if p.suggestionEngine == nil {
		return []CategorySuggestion{}, nil
	}
	return p.suggestionEngine.GetCategorySuggestions(ctx, budgetID, description, payeeID, 5)
}

// RecordPattern records a payee-category pattern for future suggestions.
func (p *Processor) RecordPattern(ctx context.Context, budgetID, description, payeeID, payeeName, categoryID, categoryName string, txnTime time.Time) error {
	if p.suggestionEngine == nil {
		return nil
	}

	// Don't record if we don't have a payee name
	if payeeName == "" {
		return nil
	}

	// Create and upsert pattern
	pattern := PayeePattern{
		BudgetID:              budgetID,
		NormalizedDescription: Fingerprint(description),
		PayeeID:               payeeID,
		PayeeName:             payeeName,
		CategoryID:            categoryID,
		CategoryName:          categoryName,
		LastSeen:              txnTime,
	}

	return p.suggestionEngine.RecordPattern(ctx, pattern)
}
