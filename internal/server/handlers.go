package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/oneils/ynab-helper/internal/txn"
	"github.com/oneils/ynab-helper/internal/ynab"
	"github.com/oneils/ynab-helper/ui"
)

const (
	baseTmpl  = "base"
	errorTmpl = "error"
)

func (s *Server) render(w http.ResponseWriter, status int, page, tmplName string, data any) {
	ts, ok := s.TemplateCache[page]
	if !ok {
		slog.Error("template not found", "page", page)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	buf := new(bytes.Buffer)
	if tmplName == "" {
		tmplName = baseTmpl
	}

	if err := ts.ExecuteTemplate(buf, tmplName, data); err != nil {
		slog.Error("template execution failed", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(status)
	if _, err := buf.WriteTo(w); err != nil {
		slog.Error("failed to write response", "error", err)
	}
}

func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	budgets, err := s.Syncer.FetchBudgets(r.Context())
	if err != nil {
		slog.Error("failed to fetch budgets on index page", "error", err)
		// Don't block page load, just show empty budgets
		budgets = []ynab.Budget{}
	}

	data := struct {
		Budgets []ynab.Budget
		Accs    []ynab.Account
		Txns    []TxnListRow
		Account string
	}{
		Budgets: budgets,
	}

	s.render(w, http.StatusOK, "home.tmpl.html", baseTmpl, data)
}

func (s *Server) importBankTxnsHandler(w http.ResponseWriter, r *http.Request) {
	budgets, err := s.Syncer.FetchBudgets(r.Context())
	if err != nil {
		slog.Error("failed to fetch budgets on import page", "error", err)
		// Don't block page load, just show empty budgets
		budgets = []ynab.Budget{}
	}

	activeStatus := r.URL.Query().Get("status")
	budgetID := r.URL.Query().Get("budget")
	accountID := r.URL.Query().Get("account")

	page, limit := parsePagination(r)

	data := struct {
		Budgets        []ynab.Budget
		Accs           []ynab.Account
		Txns           []TxnListRow
		PageMeta       PageMeta
		SelectedBudget string
		Budget         string
		Account        string
		ActiveStatus   string
		StatusCounts   map[string]int
	}{
		Budgets:      budgets,
		ActiveStatus: activeStatus,
		Budget:       budgetID,
		Account:      accountID,
	}

	// If only one budget and no explicit selection, auto-select it
	if len(budgets) == 1 && budgetID == "" {
		budgetID = budgets[0].ID
	}

	if budgetID != "" {
		data.SelectedBudget = budgetID
		data.Budget = budgetID

		// Find the budget to get its accounts
		for _, b := range budgets {
			if b.ID == budgetID {
				data.Accs = b.Accounts
				break
			}
		}

		// Fetch all transactions for this budget
		txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
			BudgetID:  budgetID,
			AccountID: accountID,
			Status:    activeStatus,
		})
		if err == nil {
			rows := enrichTransactionList(r.Context(), txns,
				s.Syncer.FindBudgetByAccID,
				func(ctx context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
					return s.TxnProcessor.GetSmartSuggestions(ctx, budgetID, description)
				},
				func(ctx context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
					return s.TxnProcessor.GetCategorySuggestions(ctx, budgetID, description, payeeID)
				},
				s.Syncer.FetchPayeesByBudget,
				s.TxnProcessor.SuggestPayee,
			)
			pm := newPageMeta(page, limit, len(rows))
			start := (pm.Page - 1) * pm.Limit
			end := start + pm.Limit
			if start > len(rows) {
				start = len(rows)
			}
			if end > len(rows) {
				end = len(rows)
			}
			data.Txns = rows[start:end]
			data.PageMeta = pm
		}

		// Fetch status counts
		statusCounts, err := s.TxnProcessor.CountByStatus(r.Context(), accountID)
		if err != nil {
			slog.Error("failed to count transactions by status", "error", err)
		} else {
			statusCountsStr := make(map[string]int, len(statusCounts))
			for k, v := range statusCounts {
				statusCountsStr[string(k)] = v
			}
			data.StatusCounts = statusCountsStr
		}
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", baseTmpl, data)
}

func (s *Server) syncHistoryHandler(w http.ResponseWriter, r *http.Request) {
	budget := r.URL.Query().Get("budget")

	var syncHistory []ynab.SyncHistory
	var err error
	if budget == "" {
		syncHistory, err = s.Syncer.FetchHistory(r.Context())
	} else {
		syncHistory, err = s.Syncer.FindHistoryByBudget(r.Context(), budget)
	}
	var errorMsg string
	if err != nil {
		slog.Error("failed to fetch sync history", "error", err)
		errorMsg = "Failed to load sync history"
	}

	s.render(w, http.StatusOK, "ynab-settings.tmpl.html", "sync-statuses", struct {
		History  []ynab.SyncHistory
		ErrorMsg string
	}{
		History:  syncHistory,
		ErrorMsg: errorMsg,
	})
}

func (s *Server) aboutViewHandler(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "about.tmpl.html", baseTmpl, nil)
}

func (s *Server) accountsHandler(w http.ResponseWriter, r *http.Request) {
	budgetID := r.URL.Query().Get("budget")
	accountID := r.URL.Query().Get("account")

	budget, err := s.Syncer.FindBudgetByID(r.Context(), budgetID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	data := struct {
		Accs    []ynab.Account
		Account string
	}{
		Accs:    budget.Accounts,
		Account: accountID,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "accounts-select", data)
}

// TxnListRow is a view-model for transaction list rows, enriched with suggestion data.
type TxnListRow struct {
	Txn         txn.Transaction
	SugPayee    string // empty if no match
	SugCategory string // empty if no match
	AutoFilled  bool   // true if either field was pre-filled
}

// PageMeta holds pagination state for transaction list views.
type PageMeta struct {
	Page       int
	PrevPage   int // 0 when on the first page (disables Prev button)
	NextPage   int // 0 when on the last page (disables Next button)
	Limit      int
	Total      int
	TotalPages int
}

// parsePagination reads page and limit from the request, applying defaults and clamping.
func parsePagination(r *http.Request) (page, limit int) {
	page = 1
	limit = 20
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v >= 1 {
			page = v
		}
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v >= 1 {
			limit = v
		}
	}
	if limit > 200 {
		limit = 200
	}
	return page, limit
}

func newPageMeta(page, limit, total int) PageMeta {
	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	prev := 0
	if page > 1 {
		prev = page - 1
	}
	next := 0
	if page < totalPages {
		next = page + 1
	}
	return PageMeta{
		Page:       page,
		PrevPage:   prev,
		NextPage:   next,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}

// bankTxnRowsHandler returns only <tr> rows for the infinite scroll sentinel.
func (s *Server) bankTxnRowsHandler(w http.ResponseWriter, r *http.Request) {
	budgetID := r.URL.Query().Get("budget")
	accountID := r.URL.Query().Get("account")
	status := r.URL.Query().Get("status")
	page, limit := parsePagination(r)

	txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
		BudgetID:  budgetID,
		AccountID: accountID,
		Status:    status,
	})
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	rows := enrichTransactionList(r.Context(), txns,
		s.Syncer.FindBudgetByAccID,
		func(ctx context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
			return s.TxnProcessor.GetSmartSuggestions(ctx, budgetID, description)
		},
		func(ctx context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
			return s.TxnProcessor.GetCategorySuggestions(ctx, budgetID, description, payeeID)
		},
		s.Syncer.FetchPayeesByBudget,
		s.TxnProcessor.SuggestPayee,
	)

	pm := newPageMeta(page, limit, len(rows))
	start := (pm.Page - 1) * pm.Limit
	end := start + pm.Limit
	if start > len(rows) {
		start = len(rows)
	}
	if end > len(rows) {
		end = len(rows)
	}

	data := struct {
		Txns         []TxnListRow
		PageMeta     PageMeta
		Budget       string
		Account      string
		ActiveStatus string
		StatusCounts map[string]int
	}{
		Txns:         rows[start:end],
		PageMeta:     pm,
		Budget:       budgetID,
		Account:      accountID,
		ActiveStatus: status,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "txn-rows", data)
}

func (s *Server) bankTxnsHandler(w http.ResponseWriter, r *http.Request) {
	budgetID := r.URL.Query().Get("budget")
	accountID := r.URL.Query().Get("account")
	status := r.URL.Query().Get("status")
	page, limit := parsePagination(r)

	txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
		BudgetID:  budgetID,
		AccountID: accountID,
		Status:    status,
	})
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	rows := enrichTransactionList(r.Context(), txns,
		s.Syncer.FindBudgetByAccID,
		func(ctx context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
			return s.TxnProcessor.GetSmartSuggestions(ctx, budgetID, description)
		},
		func(ctx context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
			return s.TxnProcessor.GetCategorySuggestions(ctx, budgetID, description, payeeID)
		},
		s.Syncer.FetchPayeesByBudget,
		s.TxnProcessor.SuggestPayee,
	)

	pm := newPageMeta(page, limit, len(rows))
	start := (pm.Page - 1) * pm.Limit
	end := start + pm.Limit
	if start > len(rows) {
		start = len(rows)
	}
	if end > len(rows) {
		end = len(rows)
	}
	rows = rows[start:end]

	statusCounts, err := s.TxnProcessor.CountByStatus(r.Context(), accountID)
	if err != nil {
		slog.Error("failed to count transactions by status", "error", err)
		statusCounts = make(map[txn.TransactionStatus]int)
	}
	statusCountsStr := make(map[string]int, len(statusCounts))
	for k, v := range statusCounts {
		statusCountsStr[string(k)] = v
	}

	data := struct {
		Txns         []TxnListRow
		PageMeta     PageMeta
		Budget       string
		Account      string
		ActiveStatus string
		StatusCounts map[string]int
	}{
		Txns:         rows,
		PageMeta:     pm,
		Budget:       budgetID,
		Account:      accountID,
		ActiveStatus: status,
		StatusCounts: statusCountsStr,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions", data)
}

// enrichTransactionList enriches each transaction with payee and category suggestions.
// Budget and payee lookups are cached per account/budget ID. Suggestion failures are non-fatal.
// When pattern-based suggestions are absent, falls back to direct YNAB payee name matching.
func enrichTransactionList(
	ctx context.Context,
	txns []txn.Transaction,
	budgetByAccID func(context.Context, string) (ynab.Budget, error),
	getSuggestions func(context.Context, string, string) ([]txn.PayeeSuggestion, error),
	getCategorySuggestions func(context.Context, string, string, string) ([]txn.CategorySuggestion, error),
	getPayeesByBudget func(context.Context, string) ([]ynab.Payee, error),
	suggestPayee func(txn.Transaction, []ynab.Payee) ynab.Payee,
) []TxnListRow {
	rows := make([]TxnListRow, len(txns))
	budgetCache := make(map[string]ynab.Budget)
	payeeCache := make(map[string][]ynab.Payee)
	failedAccounts := make(map[string]bool)

	for i, t := range txns {
		rows[i].Txn = t

		if failedAccounts[t.Account.ID] {
			continue
		}

		budget, ok := budgetCache[t.Account.ID]
		if !ok {
			var err error
			budget, err = budgetByAccID(ctx, t.Account.ID)
			if err != nil {
				slog.Warn("failed to find budget for account", "accID", t.Account.ID, "error", err)
				failedAccounts[t.Account.ID] = true
				continue
			}
			budgetCache[t.Account.ID] = budget
		}

		payeeSuggestions, err := getSuggestions(ctx, budget.ID, t.Description)
		var sugPayeeID string
		if err == nil && len(payeeSuggestions) > 0 {
			sugPayeeID = payeeSuggestions[0].PayeeID
			rows[i].SugPayee = payeeSuggestions[0].PayeeName
			rows[i].AutoFilled = true
		} else if suggestPayee != nil && getPayeesByBudget != nil {
			// Fallback: match against YNAB payee names when no learned patterns exist
			payees, cached := payeeCache[budget.ID]
			if !cached {
				payees, _ = getPayeesByBudget(ctx, budget.ID)
				payeeCache[budget.ID] = payees
			}
			if matched := suggestPayee(t, payees); matched.ID != "" {
				sugPayeeID = matched.ID
				rows[i].SugPayee = matched.Name
				rows[i].AutoFilled = true
			}
		}

		catSuggestions, err := getCategorySuggestions(ctx, budget.ID, t.Description, sugPayeeID)
		if err == nil && len(catSuggestions) > 0 {
			rows[i].SugCategory = catSuggestions[0].CategoryName
			rows[i].AutoFilled = true
		}
	}

	return rows
}

// wrapTransactions converts a slice of transactions to TxnListRow wrappers without enrichment.
func wrapTransactions(txns []txn.Transaction) []TxnListRow {
	rows := make([]TxnListRow, len(txns))
	for i, t := range txns {
		rows[i] = TxnListRow{Txn: t}
	}
	return rows
}

func (s *Server) detailBankTxnHandler(w http.ResponseWriter, r *http.Request) {
	txnID := chi.URLParam(r, "id")
	activeStatus := r.URL.Query().Get("status")

	transaction, err := s.TxnProcessor.FetchByID(r.Context(), txnID)
	if err != nil {
		s.render(w, http.StatusNotFound, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budget, err := s.Syncer.FindBudgetByAccID(r.Context(), transaction.Account.ID)
	if err != nil {
		s.render(w, http.StatusInternalServerError, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	payees, err := s.Syncer.FetchPayeesByBudget(r.Context(), budget.ID)
	if err != nil {
		s.render(w, http.StatusInternalServerError, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	categories, err := s.Syncer.FetchCategoriesByBudget(r.Context(), budget.ID)
	if err != nil {
		s.render(w, http.StatusInternalServerError, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	slog.Info("detail panel data", "budgetID", budget.ID, "payeeCount", len(payees), "categoryCount", len(categories))

	payeeSugs, err := s.TxnProcessor.GetSmartSuggestions(r.Context(), budget.ID, transaction.Description)
	if err != nil {
		slog.Warn("failed to get payee suggestions for detail", "txnID", txnID, "error", err)
	}
	var patternPayeeID string
	if len(payeeSugs) > 0 {
		patternPayeeID = payeeSugs[0].PayeeID
	}

	catSugs, catErr := s.TxnProcessor.GetCategorySuggestions(r.Context(), budget.ID, transaction.Description, patternPayeeID)
	if catErr != nil {
		slog.Warn("failed to get category suggestions for detail", "txnID", txnID, "error", catErr)
	}
	var patternCatID string
	if len(catSugs) > 0 {
		patternCatID = catSugs[0].CategoryID
	}

	sugPayeeID, sugCatID := applyYnabPayeeFallback(transaction, payees, s.TxnProcessor.SuggestPayee, patternPayeeID, patternCatID)

	data := struct {
		Txn              txn.Transaction
		BudgetID         string
		Payees           []ynab.Payee
		Categories       []ynab.Category
		SugPayeeID       string
		SugPayeeName     string
		SugCategoryID    string
		SugCategoryName  string
		ActiveStatus     string
	}{
		Txn:              transaction,
		BudgetID:         budget.ID,
		Payees:           payees,
		Categories:       categories,
		SugPayeeID:       sugPayeeID,
		SugPayeeName:     payeeNameByID(payees, sugPayeeID),
		SugCategoryID:    sugCatID,
		SugCategoryName:  categoryNameByID(categories, sugCatID),
		ActiveStatus:     activeStatus,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "txn-detail-panel", data)
}

func payeeNameByID(payees []ynab.Payee, id string) string {
	for _, p := range payees {
		if p.ID == id {
			return p.Name
		}
	}
	return ""
}

func categoryNameByID(categories []ynab.Category, id string) string {
	for _, c := range categories {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}

// applyYnabPayeeFallback returns suggested payee/category IDs using YNAB payee name matching
// when pattern-based suggestions are absent. Pattern results win when non-empty.
func applyYnabPayeeFallback(
	t txn.Transaction,
	payees []ynab.Payee,
	suggestFn func(txn.Transaction, []ynab.Payee) ynab.Payee,
	patternPayeeID, patternCatID string,
) (payeeID, catID string) {
	if patternPayeeID != "" {
		return patternPayeeID, patternCatID
	}
	matched := suggestFn(t, payees)
	if matched.ID == "" {
		return "", patternCatID
	}
	catID = patternCatID
	if catID == "" {
		catID = matched.LastCategoryID
	}
	return matched.ID, catID
}

func (s *Server) skipBankTxnHandler(w http.ResponseWriter, r *http.Request) {
	txnID := chi.URLParam(r, "id")
	accID := r.URL.Query().Get("accId")
	activeStatus := r.URL.Query().Get("status")

	if accID == "" {
		s.render(w, http.StatusBadRequest, "error.tmpl.html", errorTmpl, "account ID is required")
		return
	}

	budget, err := s.Syncer.FindBudgetByAccID(r.Context(), accID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	if err := s.TxnProcessor.Skip(r.Context(), txnID); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
		BudgetID:  budget.ID,
		AccountID: accID,
	})
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	statusCounts, err := s.TxnProcessor.CountByStatus(r.Context(), accID)
	if err != nil {
		slog.Error("failed to count transactions by status", "error", err)
		statusCounts = make(map[txn.TransactionStatus]int)
	}
	statusCountsStr := make(map[string]int, len(statusCounts))
	for k, v := range statusCounts {
		statusCountsStr[string(k)] = v
	}

	data := struct {
		Txns         []TxnListRow
		Budget       string
		Account      string
		ActiveStatus string
		StatusCounts map[string]int
	}{
		Txns: enrichTransactionList(r.Context(), txns,
			s.Syncer.FindBudgetByAccID,
			func(ctx context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
				return s.TxnProcessor.GetSmartSuggestions(ctx, budgetID, description)
			},
			func(ctx context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
				return s.TxnProcessor.GetCategorySuggestions(ctx, budgetID, description, payeeID)
			},
			s.Syncer.FetchPayeesByBudget,
			s.TxnProcessor.SuggestPayee,
		),
		Budget:       budget.ID,
		Account:      accID,
		ActiveStatus: activeStatus,
		StatusCounts: statusCountsStr,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions", data)
}

func (s *Server) uploadTxnToYnabHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	form := txn.SaveForm{
		BudgetID:   r.PostForm.Get("budget"),
		AccountID:  r.PostForm.Get("account"),
		PayeeID:    r.PostForm.Get("payee"),
		CategoryID: r.PostForm.Get("category"),
		Memo:       r.PostForm.Get("memo"),
		Amount:     r.PostForm.Get("amount"),
		TxnDate:    r.PostForm.Get("txnDate"),
		TxnID:      r.PostForm.Get("txnID"),
	}
	activeStatus := r.URL.Query().Get("status")

	if err := s.TxnProcessor.SaveToYnab(r.Context(), form); err != nil {
		slog.Error("error uploading transaction to YNAB", "error", err)
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	if err := s.Syncer.UpdatePayeeLastCategory(r.Context(), form.PayeeID, form.CategoryID); err != nil {
		slog.Error("error updating payee last category", "error", err)
	}

	if form.PayeeID != "" {
		transaction, fetchErr := s.TxnProcessor.FetchByID(r.Context(), form.TxnID)
		if fetchErr != nil {
			slog.Error("error fetching transaction for pattern recording", "error", fetchErr)
		} else {
			payees, payeeErr := s.Syncer.FetchPayeesByBudget(r.Context(), form.BudgetID)
			if payeeErr != nil {
				slog.Error("error fetching payees for pattern recording", "error", payeeErr)
			} else {
				var payeeName string
				for _, p := range payees {
					if p.ID == form.PayeeID {
						payeeName = p.Name
						break
					}
				}
				var categoryName string
				if form.CategoryID != "" {
					categories, catErr := s.Syncer.FetchCategoriesByBudget(r.Context(), form.BudgetID)
					if catErr != nil {
						slog.Error("error fetching categories for pattern recording", "error", catErr)
					} else {
						for _, c := range categories {
							if c.ID == form.CategoryID {
								categoryName = c.Name
								break
							}
						}
					}
				}
				if err := s.TxnProcessor.RecordPattern(r.Context(), form.BudgetID, transaction.Description, form.PayeeID, payeeName, form.CategoryID, categoryName, transaction.TxnTime); err != nil {
					slog.Error("error recording payee pattern", "error", err)
				}
			}
		}
	}

	txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
		BudgetID:  form.BudgetID,
		AccountID: form.AccountID,
	})
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	statusCounts, err := s.TxnProcessor.CountByStatus(r.Context(), form.AccountID)
	if err != nil {
		slog.Error("failed to count transactions by status", "error", err)
		statusCounts = make(map[txn.TransactionStatus]int)
	}
	statusCountsStr := make(map[string]int, len(statusCounts))
	for k, v := range statusCounts {
		statusCountsStr[string(k)] = v
	}

	data := struct {
		Txns         []TxnListRow
		Budget       string
		Account      string
		ActiveStatus string
		StatusCounts map[string]int
	}{
		Txns: enrichTransactionList(r.Context(), txns,
			s.Syncer.FindBudgetByAccID,
			func(ctx context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
				return s.TxnProcessor.GetSmartSuggestions(ctx, budgetID, description)
			},
			func(ctx context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
				return s.TxnProcessor.GetCategorySuggestions(ctx, budgetID, description, payeeID)
			},
			s.Syncer.FetchPayeesByBudget,
			s.TxnProcessor.SuggestPayee,
		),
		Budget:       form.BudgetID,
		Account:      form.AccountID,
		ActiveStatus: activeStatus,
		StatusCounts: statusCountsStr,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions", data)
}

func (s *Server) settingsViewHandler(w http.ResponseWriter, r *http.Request) {
	syncHistory, err := s.Syncer.FetchHistory(r.Context())
	var historyErrMsg string
	if err != nil {
		slog.Error("failed to fetch sync history on settings page", "error", err)
		historyErrMsg = "Failed to load sync history"
	}

	budgets, err := s.Syncer.FetchBudgets(r.Context())
	if err != nil {
		slog.Error("failed to fetch budgets on settings page", "error", err)
		budgets = []ynab.Budget{}
	}

	slog.Debug("fetched budgets", "count", len(budgets))

	data := struct {
		History  []ynab.SyncHistory
		Budgets  []ynab.Budget
		ErrorMsg string
	}{
		History:  syncHistory,
		Budgets:  budgets,
		ErrorMsg: historyErrMsg,
	}

	s.render(w, http.StatusOK, "ynab-settings.tmpl.html", baseTmpl, data)
}

func (s *Server) syncBudgetsHandler(w http.ResponseWriter, r *http.Request) {
	if err := s.Syncer.SyncBudgets(r.Context()); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	w.Header().Set("HX-Refresh", "true")

	syncHistory, err := s.Syncer.FetchHistory(r.Context())
	var errorMsg string
	if err != nil {
		slog.Error("failed to fetch sync history after budget sync", "error", err)
		errorMsg = "Failed to load sync history"
	}

	s.render(w, http.StatusOK, "ynab-settings.tmpl.html", "sync-statuses", struct {
		History  []ynab.SyncHistory
		ErrorMsg string
	}{
		History:  syncHistory,
		ErrorMsg: errorMsg,
	})
}

func (s *Server) syncAccountsHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budget := r.PostForm.Get("budget")
	if budget == "" {
		s.render(w, http.StatusBadRequest, "error.tmpl.html", errorTmpl, "a budget must be selected")
		return
	}

	if err := s.Syncer.SyncAccounts(r.Context(), budget); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	slog.Info("synced accounts", "budget", budget)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) syncCategoriesHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budget := r.PostForm.Get("budget")
	if budget == "" {
		s.render(w, http.StatusBadRequest, "error.tmpl.html", errorTmpl, "a budget must be selected")
		return
	}

	if err := s.Syncer.SyncCategories(r.Context(), budget); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	syncHistory, err := s.Syncer.FindHistoryByBudget(r.Context(), budget)
	var errorMsg string
	if err != nil {
		slog.Error("failed to fetch sync history after categories sync", "error", err)
		errorMsg = "Failed to load sync history"
	}

	s.render(w, http.StatusOK, "ynab-settings.tmpl.html", "sync-statuses", struct {
		History  []ynab.SyncHistory
		ErrorMsg string
	}{
		History:  syncHistory,
		ErrorMsg: errorMsg,
	})
}

func (s *Server) syncPayeesHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budget := r.PostForm.Get("budget")
	if budget == "" {
		s.render(w, http.StatusBadRequest, "error.tmpl.html", errorTmpl, "a budget must be selected")
		return
	}

	if err := s.Syncer.SyncPayees(r.Context(), budget); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	syncHistory, err := s.Syncer.FindHistoryByBudget(r.Context(), budget)
	var errorMsg string
	if err != nil {
		slog.Error("failed to fetch sync history after payees sync", "error", err)
		errorMsg = "Failed to load sync history"
	}

	s.render(w, http.StatusOK, "ynab-settings.tmpl.html", "sync-statuses", struct {
		History  []ynab.SyncHistory
		ErrorMsg string
	}{
		History:  syncHistory,
		ErrorMsg: errorMsg,
	})
}

func (s *Server) uploadBankTxnsHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	file, handler, err := r.FormFile("txn-file")
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}
	defer file.Close() //nolint:errcheck

	budgetID := r.FormValue("budget")
	accID := r.FormValue("account")

	if budgetID == "" || accID == "" {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, "budget and account are required")
		return
	}

	params := txn.ProcessParams{
		File:        file,
		FileHandler: handler,
		BudgetID:    budgetID,
		AccountID:   accID,
	}

	if err := s.TxnProcessor.Process(r.Context(), params); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	txns, err := s.TxnProcessor.Fetch(r.Context(), params)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	activeStatus := r.URL.Query().Get("status")

	statusCounts, err := s.TxnProcessor.CountByStatus(r.Context(), accID)
	if err != nil {
		slog.Error("failed to count transactions by status", "error", err)
		statusCounts = make(map[txn.TransactionStatus]int)
	}
	statusCountsStr := make(map[string]int, len(statusCounts))
	for k, v := range statusCounts {
		statusCountsStr[string(k)] = v
	}

	data := struct {
		Txns         []TxnListRow
		Budget       string
		Account      string
		ActiveStatus string
		StatusCounts map[string]int
	}{
		Txns:         wrapTransactions(txns),
		Budget:       budgetID,
		Account:      accID,
		ActiveStatus: activeStatus,
		StatusCounts: statusCountsStr,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions", data)
}

func (s *Server) previewBankTxnsHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	file, handler, err := r.FormFile("txn-file")
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}
	defer file.Close() //nolint:errcheck

	budgetID := r.FormValue("budget")
	accID := r.FormValue("account")

	if budgetID == "" || accID == "" {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, "budget and account are required")
		return
	}

	// Read file bytes
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, fmt.Sprintf("reading file: %v", err))
		return
	}

	// Save to temporary storage
	fileUUID, err := s.FileStore.SaveUpload(fileBytes, handler.Filename)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, fmt.Sprintf("saving temp file: %v", err))
		return
	}

	// Parse CSV for preview
	csvReader := csv.NewReader(bytes.NewReader(fileBytes))
	csvReader.LazyQuotes = true // Allow malformed quotes in fields (common in bank exports)
	csvReader.TrimLeadingSpace = true
	data, err := csvReader.ReadAll()
	if err != nil {
		_ = s.FileStore.DeleteUpload(fileUUID) // Clean up temp file
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, fmt.Sprintf("parsing CSV: %v", err))
		return
	}

	// Get preview
	preview, err := s.TxnProcessor.Preview(r.Context(), txn.ProcessParams{
		Data:      data,
		BudgetID:  budgetID,
		AccountID: accID,
	})
	if err != nil {
		_ = s.FileStore.DeleteUpload(fileUUID) // Clean up temp file
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	// Render preview template
	previewData := struct {
		Preview   *txn.PreviewResult
		FileUUID  string
		BudgetID  string
		AccountID string
		Filename  string
	}{
		Preview:   preview,
		FileUUID:  fileUUID,
		BudgetID:  budgetID,
		AccountID: accID,
		Filename:  handler.Filename,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions-preview", previewData)
}

func (s *Server) confirmBankTxnsHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	fileUUID := r.FormValue("file_uuid")
	budgetID := r.FormValue("budget")
	accID := r.FormValue("account")

	if fileUUID == "" || budgetID == "" || accID == "" {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, "missing required parameters")
		return
	}

	// Retrieve file from temp storage
	fileBytes, err := s.FileStore.GetUpload(fileUUID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, "Preview expired or not found. Please upload the file again.")
		return
	}

	// Parse CSV
	csvReader := csv.NewReader(bytes.NewReader(fileBytes))
	csvReader.LazyQuotes = true // Allow malformed quotes in fields (common in bank exports)
	csvReader.TrimLeadingSpace = true
	data, err := csvReader.ReadAll()
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, fmt.Sprintf("parsing CSV: %v", err))
		return
	}

	// Process and save to database
	params := txn.ProcessParams{
		Data:      data,
		BudgetID:  budgetID,
		AccountID: accID,
	}

	if err := s.TxnProcessor.Process(r.Context(), params); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	// Delete temp file
	_ = s.FileStore.DeleteUpload(fileUUID)

	// Redirect to the split-panel Transactions page so the user can review newly imported txns
	w.Header().Set("HX-Redirect", fmt.Sprintf("/import-bank-txns?budget=%s&account=%s", url.QueryEscape(budgetID), url.QueryEscape(accID)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cancelPreviewHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fileUUID := r.FormValue("file_uuid")
	if fileUUID != "" {
		_ = s.FileStore.DeleteUpload(fileUUID)
	}

	w.WriteHeader(http.StatusOK)
}

// Inline editing handlers

func (s *Server) saveInlineTxnHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	txnID := chi.URLParam(r, "id")

	form := txn.SaveForm{
		TxnID:      txnID,
		BudgetID:   r.PostForm.Get("budget"),
		AccountID:  r.PostForm.Get("account"),
		PayeeID:    r.PostForm.Get("payee"),
		CategoryID: r.PostForm.Get("category"),
		Memo:       r.PostForm.Get("memo"),
		Amount:     r.PostForm.Get("amount"),
		TxnDate:    r.PostForm.Get("txnDate"),
	}

	if form.PayeeID == "" || form.CategoryID == "" {
		w.Header().Set("HX-Trigger", `{"showToast": {"message": "Select both a payee and category to remember selections", "type": "warning"}}`)
		w.Header().Set("HX-Reswap", "none")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Update payee's last category for future suggestions
	if form.PayeeID != "" && form.CategoryID != "" {
		if err := s.Syncer.UpdatePayeeLastCategory(r.Context(), form.PayeeID, form.CategoryID); err != nil {
			slog.Error("error updating payee last category", "error", err)
			// Don't fail the request, just log the error
		}
	}

	// Fetch the updated transaction
	transaction, err := s.TxnProcessor.FetchByID(r.Context(), txnID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	// Record pattern for future smart suggestions
	if form.PayeeID != "" {
		// Fetch payees to get the payee name for pattern recording
		payees, err := s.Syncer.FetchPayeesByBudget(r.Context(), form.BudgetID)
		if err != nil {
			slog.Error("error fetching payees for pattern recording", "error", err)
		} else {
			// Look up payee name
			var payeeName string
			for _, p := range payees {
				if p.ID == form.PayeeID {
					payeeName = p.Name
					break
				}
			}

			// Look up category name if provided
			var categoryName string
			if form.CategoryID != "" {
				categories, err := s.Syncer.FetchCategoriesByBudget(r.Context(), form.BudgetID)
				if err != nil {
					slog.Error("error fetching categories for pattern recording", "error", err)
				} else {
					for _, c := range categories {
						if c.ID == form.CategoryID {
							categoryName = c.Name
							break
						}
					}
				}
			}

			// Record the pattern
			if err := s.TxnProcessor.RecordPattern(r.Context(), form.BudgetID, transaction.Description, form.PayeeID, payeeName, form.CategoryID, categoryName, transaction.TxnTime); err != nil {
				slog.Error("error recording payee pattern", "error", err)
			}
		}
	}

	// Fetch updated transaction list for the account
	txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
		BudgetID:  form.BudgetID,
		AccountID: form.AccountID,
	})
	if err != nil {
		slog.Error("failed to fetch transactions after save", "error", err)
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	statusCounts, err := s.TxnProcessor.CountByStatus(r.Context(), form.AccountID)
	if err != nil {
		slog.Error("failed to count transactions by status", "error", err)
		statusCounts = make(map[txn.TransactionStatus]int)
	}
	statusCountsStr := make(map[string]int, len(statusCounts))
	for k, v := range statusCounts {
		statusCountsStr[string(k)] = v
	}

	// Add success message header for toast notification
	w.Header().Set("HX-Trigger", `{"showToast": {"message": "Transaction saved successfully", "type": "success"}}`)

	responseData := struct {
		Txns         []TxnListRow
		Budget       string
		Account      string
		ActiveStatus string
		StatusCounts map[string]int
	}{
		Txns: enrichTransactionList(r.Context(), txns,
			s.Syncer.FindBudgetByAccID,
			func(ctx context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
				return s.TxnProcessor.GetSmartSuggestions(ctx, budgetID, description)
			},
			func(ctx context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
				return s.TxnProcessor.GetCategorySuggestions(ctx, budgetID, description, payeeID)
			},
			s.Syncer.FetchPayeesByBudget,
			s.TxnProcessor.SuggestPayee,
		),
		Budget:       form.BudgetID,
		Account:      form.AccountID,
		ActiveStatus: r.URL.Query().Get("status"),
		StatusCounts: statusCountsStr,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions", responseData)
}

// Bulk operations handlers

func (s *Server) bulkSkipTxnsHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	// Get transaction IDs from form (sent as array)
	txnIDs := r.Form["txn_ids[]"]
	if len(txnIDs) == 0 {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, "No transactions selected")
		return
	}

	// Get account ID for fetching budget
	accID := r.FormValue("account_id")
	if accID == "" {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, "Account ID is required")
		return
	}

	// Skip each transaction
	for _, txnID := range txnIDs {
		if err := s.TxnProcessor.Skip(r.Context(), txnID); err != nil {
			slog.Error("error skipping transaction", "txn_id", txnID, "error", err)
			s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, fmt.Sprintf("Failed to skip transaction %s: %v", txnID, err))
			return
		}
	}

	// Get budget for re-fetching transactions
	budget, err := s.Syncer.FindBudgetByAccID(r.Context(), accID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	// Fetch all transactions for the account
	txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
		BudgetID:  budget.ID,
		AccountID: accID,
	})
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	statusCounts, err := s.TxnProcessor.CountByStatus(r.Context(), accID)
	if err != nil {
		slog.Error("failed to count transactions by status", "error", err)
		statusCounts = make(map[txn.TransactionStatus]int)
	}
	statusCountsStr := make(map[string]int, len(statusCounts))
	for k, v := range statusCounts {
		statusCountsStr[string(k)] = v
	}

	// Re-render transaction list
	data := struct {
		Txns         []TxnListRow
		Budget       string
		Account      string
		ActiveStatus string
		StatusCounts map[string]int
	}{
		Txns: enrichTransactionList(r.Context(), txns,
			s.Syncer.FindBudgetByAccID,
			func(ctx context.Context, budgetID, description string) ([]txn.PayeeSuggestion, error) {
				return s.TxnProcessor.GetSmartSuggestions(ctx, budgetID, description)
			},
			func(ctx context.Context, budgetID, description, payeeID string) ([]txn.CategorySuggestion, error) {
				return s.TxnProcessor.GetCategorySuggestions(ctx, budgetID, description, payeeID)
			},
			s.Syncer.FetchPayeesByBudget,
			s.TxnProcessor.SuggestPayee,
		),
		Budget:       budget.ID,
		Account:      accID,
		ActiveStatus: "",
		StatusCounts: statusCountsStr,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions", data)
}

func (s *Server) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   "ynab-helper",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(health); err != nil {
		slog.Error("failed to encode health check response", "error", err)
	}
}

func (s *Server) readinessCheckHandler(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := s.DB.Ping(r.Context()); err != nil {
		dbStatus = "error"
	}

	readiness := map[string]interface{}{
		"status":    "ready",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   "ynab-helper",
		"checks": map[string]string{
			"database": dbStatus,
			"server":   "ok",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(readiness); err != nil {
		slog.Error("failed to encode readiness check response", "error", err)
	}
}

// payeeSuggestionsHandler returns JSON suggestions for a transaction.
func (s *Server) payeeSuggestionsHandler(w http.ResponseWriter, r *http.Request) {
	budgetID := r.URL.Query().Get("budget_id")
	description := r.URL.Query().Get("description")

	if budgetID == "" || description == "" {
		http.Error(w, "budget_id and description required", http.StatusBadRequest)
		return
	}

	suggestions, err := s.TxnProcessor.GetSmartSuggestions(r.Context(), budgetID, description)
	if err != nil {
		slog.Error("failed to get suggestions", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"suggestions": suggestions,
	}); err != nil {
		slog.Error("failed to encode suggestions response", "error", err)
	}
}

func (s *Server) categorySuggestionsHandler(w http.ResponseWriter, r *http.Request) {
	budgetID := r.URL.Query().Get("budget_id")
	description := r.URL.Query().Get("description")
	payeeID := r.URL.Query().Get("payee_id")

	if budgetID == "" {
		http.Error(w, "budget_id required", http.StatusBadRequest)
		return
	}

	suggestions, err := s.TxnProcessor.GetCategorySuggestions(r.Context(), budgetID, description, payeeID)
	if err != nil {
		slog.Error("failed to get category suggestions", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"suggestions": suggestions,
	}); err != nil {
		slog.Error("failed to encode category suggestions response", "error", err)
	}
}

// NewTemplateCache creates a template cache.
func NewTemplateCache() (map[string]*template.Template, error) {
	cache := map[string]*template.Template{}

	pages, err := fs.Glob(ui.Files, "html/*/*.tmpl.html")
	if err != nil {
		return nil, fmt.Errorf("globbing templates: %w", err)
	}

	for _, page := range pages {
		name := filepath.Base(page)

		patterns := []string{
			"html/index.tmpl.html",
			"html/partials/*.tmpl.html",
			page,
		}

		ts, err := template.New(name).ParseFS(ui.Files, patterns...)
		if err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", name, err)
		}
		cache[name] = ts
	}

	return cache, nil
}
