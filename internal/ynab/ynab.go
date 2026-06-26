// Package ynab contains all YNAB related logic and models.
package ynab

import "time"

// Budget represents a YNAB budget.
type Budget struct {
	ID             string    `json:"id" bson:"_id"`
	Name           string    `json:"name"`
	LastModifiedOn time.Time `json:"last_modified_on"`
	FirstMonth     string    `json:"first_month"`
	LastMonth      string    `json:"last_month"`
	DateFormat     struct {
		Format string `json:"format"`
	} `json:"date_format"`
	CurrencyFormat struct {
		IsoCode          string `json:"iso_code"`
		ExampleFormat    string `json:"example_format"`
		DecimalDigits    int    `json:"decimal_digits"`
		DecimalSeparator string `json:"decimal_separator"`
		SymbolFirst      bool   `json:"symbol_first"`
		GroupSeparator   string `json:"group_separator"`
		CurrencySymbol   string `json:"currency_symbol"`
		DisplaySymbol    bool   `json:"display_symbol"`
	} `json:"currency_format"`
	Accounts []Account `json:"accounts"`
}

// Account represents a YNAB account.
type Account struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Type                string     `json:"type"`
	OnBudget            bool       `json:"on_budget"`
	Closed              bool       `json:"closed"`
	Note                string     `json:"note"`
	Balance             int        `json:"balance"`
	ClearedBalance      int        `json:"cleared_balance"`
	UnclearedBalance    int        `json:"uncleared_balance"`
	DirectImportLinked  bool       `json:"direct_import_linked"`
	DirectImportInError bool       `json:"direct_import_in_error"`
	LastReconciledAt    *time.Time `json:"last_reconciled_at"`
	DebtOriginalBalance int        `json:"debt_original_balance"`
	TransferPayeeId     string     `json:"transfer_payee_id"`
	Deleted             bool       `json:"deleted"`
}

// CategoryGroup represents a group of YNAB categories.
type CategoryGroup struct {
	ID         string     `json:"id" bson:"_id"`
	Name       string     `json:"name"`
	Hidden     bool       `json:"hidden"`
	Deleted    bool       `json:"deleted"`
	Categories []Category `json:"categories"`
	BudgetID   string     `json:"budget_id" bson:"budget_id"`
}

// Category represents a YNAB category.
type Category struct {
	ID                      string `json:"id"`
	CategoryGroupID         string `json:"category_group_id"`
	CategoryGroupName       string `json:"category_group_name"`
	Name                    string `json:"name"`
	Hidden                  bool   `json:"hidden"`
	OriginalCategoryGroupId string `json:"original_category_group_id"`
	Note                    string `json:"note"`
	Budgeted                int    `json:"budgeted"`
	Activity                int    `json:"activity"`
	Balance                 int    `json:"balance"`
	GoalType                string `json:"goal_type"`
	GoalDays                int    `json:"goal_days"`
	GoalCadence             int    `json:"goal_cadence"`
	GoalCadenceFrequency    int    `json:"goal_cadence_frequency"`
	GoalTarget              int    `json:"goal_target"`
	GoalTargetMonth         string `json:"goal_target_month"`
	GoalCreationMonth       string `json:"goal_creation_month"`
	GoalPercentageComplete  int    `json:"goal_percentage_complete"`
	GoalMonthsToBudget      int    `json:"goal_months_to_budget"`
	GoalUnderFunded         int    `json:"goal_under_funded"`
	GoalOverallFunded       int    `json:"goal_overall_funded"`
	GoalOverallLeft         int    `json:"goal_overall_left"`
	Deleted                 bool   `json:"deleted"`
}

// Payee represents a YNAB payee.
type Payee struct {
	ID                string `json:"id" bson:"_id"`
	Name              string `json:"name"`
	TransferAccountId string `json:"transfer_account_id"`
	Deleted           bool   `json:"deleted"`
	BudgetID          string `json:"budget_id" bson:"budget_id"`
	LastCategoryID    string `json:"last_category_id" bson:"last_category_id"`
}

// SyncHistory represents a YNAB sync operation history record.
type SyncHistory struct {
	ID               string    `json:"id" bson:"_id"`
	Name             string    `json:"name"`
	UpdatedAt        time.Time `json:"updatedAt"`
	LastKnownVersion int64     `json:"lastKnownVersion,omitempty"`
	AddedItems       int       `json:"addedItems"`
	Status           string    `json:"status"`
	Message          string    `json:"message,omitempty"`
	BudgetID         string    `json:"budget_id,omitempty" bson:"budget_id,omitempty"`
}
