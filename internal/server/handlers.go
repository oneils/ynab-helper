package server

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
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
		Txns    []txn.Transaction
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

	data := struct {
		Budgets        []ynab.Budget
		Accs           []ynab.Account
		Txns           []txn.Transaction
		SelectedBudget string
	}{
		Budgets: budgets,
	}

	// If only one budget, auto-select it and load its accounts/transactions
	if len(budgets) == 1 {
		data.SelectedBudget = budgets[0].ID
		data.Accs = budgets[0].Accounts

		// Fetch all transactions for this budget
		txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
			BudgetID:  budgets[0].ID,
			AccountID: "", // All accounts
			Status:    "", // All statuses
		})
		if err == nil {
			data.Txns = txns
		}
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", baseTmpl, data)
}

func (s *Server) syncHistoryHandler(w http.ResponseWriter, r *http.Request) {
	budget := r.URL.Query().Get("budget")

	syncHistory, err := s.Syncer.FindHistoryByBudget(r.Context(), budget)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budgets, err := s.Syncer.FetchBudgets(r.Context())
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	data := struct {
		History  []ynab.SyncHistory
		Budgets  []ynab.Budget
		ErrorMsg string
	}{
		History: syncHistory,
		Budgets: budgets,
	}

	s.render(w, http.StatusOK, "ynab-settings.tmpl.html", "sync-statuses", data)
}

func (s *Server) aboutViewHandler(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "about.tmpl.html", baseTmpl, nil)
}

func (s *Server) accountsHandler(w http.ResponseWriter, r *http.Request) {
	budgetID := r.URL.Query().Get("budget")

	budget, err := s.Syncer.FindBudgetByID(r.Context(), budgetID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	data := struct {
		Accs []ynab.Account
	}{
		Accs: budget.Accounts,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "accounts-select", data)
}

func (s *Server) bankTxnsHandler(w http.ResponseWriter, r *http.Request) {
	budgetID := r.URL.Query().Get("budget")
	accountID := r.URL.Query().Get("account")
	status := r.URL.Query().Get("status")

	txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
		BudgetID:  budgetID,
		AccountID: accountID,
		Status:    status,
	})
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	data := struct {
		Txns []txn.Transaction
	}{
		Txns: txns,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions", data)
}

func (s *Server) fetchBankTxnHandler(w http.ResponseWriter, r *http.Request) {
	txnID := chi.URLParam(r, "id")
	accID := r.URL.Query().Get("accId")

	transaction, err := s.TxnProcessor.FetchByID(r.Context(), txnID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budget, err := s.Syncer.FindBudgetByAccID(r.Context(), accID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budgets, err := s.Syncer.FetchBudgets(r.Context())
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	payees, err := s.Syncer.FetchPayeesByBudget(r.Context(), budget.ID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	suggestedPayee := s.TxnProcessor.SuggestPayee(transaction, payees)

	categories, err := s.Syncer.FetchCategoriesByBudget(r.Context(), budget.ID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	data := struct {
		Txn                 txn.Transaction
		BudgetID            string
		Budgets             []ynab.Budget
		Accs                []ynab.Account
		AccID               string
		Payee               []ynab.Payee
		SuggestedPayeeID    string
		Categories          []ynab.Category
		SuggestedCategoryID string
	}{
		Txn:                 transaction,
		BudgetID:            budget.ID,
		Budgets:             budgets,
		Accs:                budget.Accounts,
		AccID:               accID,
		Payee:               payees,
		SuggestedPayeeID:    suggestedPayee.ID,
		Categories:          categories,
		SuggestedCategoryID: suggestedPayee.LastCategoryID,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-txn", data)
}

func (s *Server) skipBankTxnHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	txnID := chi.URLParam(r, "id")
	accID := r.URL.Query().Get("accId")

	if err := s.TxnProcessor.Skip(r.Context(), txnID); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budget, err := s.Syncer.FindBudgetByAccID(r.Context(), accID)
	if err != nil {
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

	data := struct {
		Txns []txn.Transaction
	}{
		Txns: txns,
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

	if err := s.TxnProcessor.SaveToYnab(r.Context(), form); err != nil {
		slog.Error("error uploading transaction to YNAB", "error", err)
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	if err := s.Syncer.UpdatePayeeLastCategory(r.Context(), form.PayeeID, form.CategoryID); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
		BudgetID:  form.BudgetID,
		AccountID: form.AccountID,
	})
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	data := struct {
		Txns []txn.Transaction
	}{
		Txns: txns,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions", data)
}

func (s *Server) settingsViewHandler(w http.ResponseWriter, r *http.Request) {
	syncHistory, err := s.Syncer.FetchHistory(r.Context())
	if err != nil {
		slog.Error("failed to fetch sync history on settings page", "error", err)
		// Don't block page load, just show empty history
		syncHistory = []ynab.SyncHistory{}
	}

	budgets, err := s.Syncer.FetchBudgets(r.Context())
	if err != nil {
		slog.Error("failed to fetch budgets on settings page", "error", err)
		// Don't block page load, just show empty budgets
		budgets = []ynab.Budget{}
	}

	slog.Info("fetched budgets", "count", len(budgets))

	data := struct {
		History  []ynab.SyncHistory
		Budgets  []ynab.Budget
		ErrorMsg string
	}{
		History: syncHistory,
		Budgets: budgets,
	}

	s.render(w, http.StatusOK, "ynab-settings.tmpl.html", baseTmpl, data)
}

func (s *Server) syncBudgetsHandler(w http.ResponseWriter, r *http.Request) {
	if err := s.Syncer.SyncBudgets(r.Context()); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budgets, err := s.Syncer.FetchBudgets(r.Context())
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	data := struct {
		History  []ynab.SyncHistory
		Budgets  []ynab.Budget
		ErrorMsg string
	}{
		Budgets: budgets,
	}

	s.render(w, http.StatusOK, "ynab-settings.tmpl.html", "sync-statuses", data)
}

func (s *Server) syncAccountsHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budget := r.PostForm.Get("budget")

	if err := s.Syncer.SyncAccounts(r.Context(), budget); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	slog.Info("synced accounts", "budget", budget)
}

func (s *Server) syncCategoriesHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budget := r.PostForm.Get("budget")

	if err := s.Syncer.SyncCategories(r.Context(), budget); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	syncHistory, err := s.Syncer.FindHistoryByBudget(r.Context(), budget)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budgets, err := s.Syncer.FetchBudgets(r.Context())
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	data := struct {
		History  []ynab.SyncHistory
		Budgets  []ynab.Budget
		ErrorMsg string
	}{
		History: syncHistory,
		Budgets: budgets,
	}

	s.render(w, http.StatusOK, "ynab-settings.tmpl.html", "sync-statuses", data)
}

func (s *Server) syncPayeesHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budget := r.PostForm.Get("budget")

	if err := s.Syncer.SyncPayees(r.Context(), budget); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	syncHistory, err := s.Syncer.FindHistoryByBudget(r.Context(), budget)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budgets, err := s.Syncer.FetchBudgets(r.Context())
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	data := struct {
		History  []ynab.SyncHistory
		Budgets  []ynab.Budget
		ErrorMsg string
	}{
		History: syncHistory,
		Budgets: budgets,
	}

	s.render(w, http.StatusOK, "ynab-settings.tmpl.html", "sync-statuses", data)
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

	data := struct {
		Txns []txn.Transaction
	}{
		Txns: txns,
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

	// Fetch and render transactions (same as uploadBankTxnsHandler)
	txns, err := s.TxnProcessor.Fetch(r.Context(), params)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	responseData := struct {
		Txns []txn.Transaction
	}{
		Txns: txns,
	}

	s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions", responseData)
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

func (s *Server) editInlineTxnHandler(w http.ResponseWriter, r *http.Request) {
	txnID := chi.URLParam(r, "id")
	accID := r.URL.Query().Get("accId")

	transaction, err := s.TxnProcessor.FetchByID(r.Context(), txnID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	budget, err := s.Syncer.FindBudgetByAccID(r.Context(), accID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	payees, err := s.Syncer.FetchPayeesByBudget(r.Context(), budget.ID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	categories, err := s.Syncer.FetchCategoriesByBudget(r.Context(), budget.ID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	// Get the transaction's index (not critical, can default to 0)
	data := struct {
		Txn        txn.Transaction
		BudgetID   string
		Payees     []ynab.Payee
		Categories []ynab.Category
		Index      int
	}{
		Txn:        transaction,
		BudgetID:   budget.ID,
		Payees:     payees,
		Categories: categories,
		Index:      0, // Will be visible in UI context
	}

	s.render(w, http.StatusOK, "transaction-row.tmpl.html", "transaction-row-edit", data)
}

func (s *Server) viewInlineTxnHandler(w http.ResponseWriter, r *http.Request) {
	txnID := chi.URLParam(r, "id")
	accID := r.URL.Query().Get("accId")

	transaction, err := s.TxnProcessor.FetchByID(r.Context(), txnID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	// Verify account access
	_, err = s.Syncer.FindBudgetByAccID(r.Context(), accID)
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	data := struct {
		Txn   txn.Transaction
		Index int
	}{
		Txn:   transaction,
		Index: 0, // Will be visible in UI context
	}

	s.render(w, http.StatusOK, "transaction-row.tmpl.html", "transaction-row-view", data)
}

func (s *Server) saveInlineTxnHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	txnID := chi.URLParam(r, "id")

	form := txn.SaveForm{
		TxnID:      txnID,
		BudgetID:   r.PostForm.Get("budget_id"),
		AccountID:  r.PostForm.Get("account_id"),
		PayeeID:    r.PostForm.Get("payee_id"),
		CategoryID: r.PostForm.Get("category_id"),
		Memo:       r.PostForm.Get("memo"),
		Amount:     r.PostForm.Get("amount"),
		TxnDate:    r.PostForm.Get("date"),
	}

	// Save to YNAB
	if err := s.TxnProcessor.SaveToYnab(r.Context(), form); err != nil {
		slog.Error("error uploading transaction to YNAB", "error", err)
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
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
		// Fetch payees and categories to get names
		payees, err := s.Syncer.FetchPayeesByBudget(r.Context(), form.BudgetID)
		if err != nil {
			slog.Error("error fetching payees for pattern recording", "error", err)
			return
		}

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
				return
			}

			for _, c := range categories {
				if c.ID == form.CategoryID {
					categoryName = c.Name
					break
				}
			}
		}

		// Record the pattern
		if err := s.TxnProcessor.RecordPattern(r.Context(), form.BudgetID, transaction.Description, form.PayeeID, payeeName, form.CategoryID, categoryName, transaction.TxnTime); err != nil {
			slog.Error("error recording payee pattern", "error", err)
			// Don't fail the request, just log the error
		}
	}

	// Return to view mode
	data := struct {
		Txn   txn.Transaction
		Index int
	}{
		Txn:   transaction,
		Index: 0,
	}

	// Add success message header for toast notification
	w.Header().Set("HX-Trigger", `{"showToast": {"message": "Transaction saved successfully", "type": "success"}}`)

	s.render(w, http.StatusOK, "transaction-row.tmpl.html", "transaction-row-view", data)
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

	// Fetch remaining DRAFT transactions
	txns, err := s.TxnProcessor.Fetch(r.Context(), txn.ProcessParams{
		BudgetID:  budget.ID,
		AccountID: accID,
		Status:    "DRAFT",
	})
	if err != nil {
		s.render(w, http.StatusOK, "error.tmpl.html", errorTmpl, err.Error())
		return
	}

	// Re-render transaction list
	data := struct {
		Txns []txn.Transaction
	}{
		Txns: txns,
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
