// Package ynab provides a client for the YNAB API.
package ynab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// Client is a YNAB API client.
type Client struct {
	apiKey     string
	apiURL     string
	httpClient *http.Client
}

// NewClient creates a new YNAB API client.
func NewClient(apiKey, apiURL string, httpClient *http.Client) *Client {
	return &Client{
		apiKey:     apiKey,
		apiURL:     apiURL,
		httpClient: httpClient,
	}
}

// SyncReq represents a sync request for a specific budget.
type SyncReq struct {
	BudgetID  string
	LastKnown bool
}

// BudgetResp is the API response for budgets.
type BudgetResp struct {
	Data BudgetData `json:"data"`
}

// BudgetData contains the list of budgets.
type BudgetData struct {
	Budgets []Budget `json:"budgets"`
}

// AccountResp is the API response for accounts.
type AccountResp struct {
	Data AccountData `json:"data"`
}

// AccountData contains the list of accounts.
type AccountData struct {
	Accounts        []Account `json:"accounts"`
	ServerKnowledge int64     `json:"server_knowledge"`
}

// CategoryResp is the API response for categories.
type CategoryResp struct {
	Data CategoryData `json:"data"`
}

// CategoryData contains the list of category groups.
type CategoryData struct {
	Categories      []CategoryGroup `json:"category_groups"`
	ServerKnowledge int64           `json:"server_knowledge"`
}

// PayeeResp is the API response for payees.
type PayeeResp struct {
	Data PayeeData `json:"data"`
}

// PayeeData contains the list of payees.
type PayeeData struct {
	Payees          []Payee `json:"payees"`
	ServerKnowledge int64   `json:"server_knowledge"`
}

// TxnReq is a request to create a new transaction in YNAB.
type TxnReq struct {
	BudgetID        string  `json:"budget_id"`
	AccountID       string  `json:"account_id"`
	Date            string  `json:"date"`
	Amount          int     `json:"amount"`
	PayeeID         string  `json:"payee_id"`
	PayeeName       *string `json:"payee_name,omitempty"`
	CategoryID      string  `json:"category_id"`
	Memo            string  `json:"memo"`
	Cleared         string  `json:"cleared"`
	Approved        bool    `json:"approved"`
	FlagColor       *string `json:"flag_color,omitempty"`
	Subtransactions []struct {
		Amount     int    `json:"amount"`
		PayeeID    string `json:"payee_id"`
		PayeeName  string `json:"payee_name"`
		CategoryID string `json:"category_id"`
		Memo       string `json:"memo"`
	} `json:"subtransactions,omitempty"`
}

type txnRequestBody struct {
	Txn TxnReq `json:"transaction"`
}

// FetchBudgets fetches budgets from the YNAB API.
func (c *Client) FetchBudgets() ([]Budget, error) {
	slog.Info("fetching budgets from YNAB API")

	req, err := http.NewRequest("GET", c.apiURL+"/budgets?include_accounts=true", nil)
	if err != nil {
		return nil, fmt.Errorf("creating budget request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching budgets: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("YNAB API error", "status_code", resp.StatusCode, "response_body", string(body))
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var budgetResp BudgetResp
	if err := json.NewDecoder(resp.Body).Decode(&budgetResp); err != nil {
		return nil, fmt.Errorf("decoding budgets response: %w", err)
	}

	slog.Info("fetched budgets", "count", len(budgetResp.Data.Budgets))
	return budgetResp.Data.Budgets, nil
}

// FetchAccounts fetches accounts for a budget from the YNAB API.
func (c *Client) FetchAccounts(syncReq SyncReq) (AccountData, error) {
	slog.Info("fetching accounts from YNAB API", "budget_id", syncReq.BudgetID)

	url := fmt.Sprintf("%s/budgets/%s/accounts?last_knowledge_of_server=0", c.apiURL, syncReq.BudgetID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return AccountData{}, fmt.Errorf("creating accounts request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AccountData{}, fmt.Errorf("fetching accounts: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("YNAB API error fetching accounts", "status_code", resp.StatusCode, "response_body", string(body), "budget_id", syncReq.BudgetID)
		return AccountData{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var accResp AccountResp
	if err := json.NewDecoder(resp.Body).Decode(&accResp); err != nil {
		return AccountData{}, fmt.Errorf("decoding accounts response: %w", err)
	}

	slog.Info("fetched accounts", "count", len(accResp.Data.Accounts))
	return accResp.Data, nil
}

// FetchCategories fetches categories for a budget from the YNAB API.
func (c *Client) FetchCategories(syncReq SyncReq) (CategoryData, error) {
	slog.Info("fetching categories from YNAB API", "budget_id", syncReq.BudgetID)

	url := fmt.Sprintf("%s/budgets/%s/categories?last_knowledge_of_server=0", c.apiURL, syncReq.BudgetID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return CategoryData{}, fmt.Errorf("creating categories request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CategoryData{}, fmt.Errorf("fetching categories: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("YNAB API error fetching categories", "status_code", resp.StatusCode, "response_body", string(body), "budget_id", syncReq.BudgetID)
		return CategoryData{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var catResp CategoryResp
	if err := json.NewDecoder(resp.Body).Decode(&catResp); err != nil {
		return CategoryData{}, fmt.Errorf("decoding categories response: %w", err)
	}

	slog.Info("fetched categories", "count", len(catResp.Data.Categories))
	return catResp.Data, nil
}

// FetchPayees fetches payees for a budget from the YNAB API.
func (c *Client) FetchPayees(syncReq SyncReq) (PayeeData, error) {
	slog.Info("fetching payees from YNAB API", "budget_id", syncReq.BudgetID)

	url := fmt.Sprintf("%s/budgets/%s/payees?last_knowledge_of_server=0", c.apiURL, syncReq.BudgetID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return PayeeData{}, fmt.Errorf("creating payees request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PayeeData{}, fmt.Errorf("fetching payees: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("YNAB API error fetching payees", "status_code", resp.StatusCode, "response_body", string(body), "budget_id", syncReq.BudgetID)
		return PayeeData{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var payeeResp PayeeResp
	if err := json.NewDecoder(resp.Body).Decode(&payeeResp); err != nil {
		return PayeeData{}, fmt.Errorf("decoding payees response: %w", err)
	}

	slog.Info("fetched payees", "count", len(payeeResp.Data.Payees))
	return payeeResp.Data, nil
}

// Upload uploads a transaction to YNAB.
func (c *Client) Upload(txn TxnReq) error {
	slog.Info("uploading transaction to YNAB", "budget_id", txn.BudgetID, "account_id", txn.AccountID)

	url := fmt.Sprintf("%s/budgets/%s/transactions", c.apiURL, txn.BudgetID)

	body, err := json.Marshal(txnRequestBody{Txn: txn})
	if err != nil {
		return fmt.Errorf("marshaling transaction: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("creating upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("uploading transaction: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		slog.Error("failed to upload transaction", "status", resp.StatusCode, "body", string(respBody))
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	slog.Info("transaction uploaded successfully")
	return nil
}
