// Package txn contains all transaction related logic and models.
package txn

import "time"

// TransactionStatus represents the status of a bank transaction.
type TransactionStatus string

// Transaction status constants.
const (
	TransactionDraft     TransactionStatus = "DRAFT"
	TransactionSkipped   TransactionStatus = "SKIPPED"
	TransactionProcessed TransactionStatus = "PROCESSED"
	TransactionInvalid   TransactionStatus = "INVALID"
)

// Transaction represents a transaction imported from bank report files.
type Transaction struct {
	ID            string            `json:"id" bson:"_id"`
	Amount        float64           `json:"amount" bson:"amount"`
	Currency      string            `json:"currency" bson:"currency"`
	Description   string            `json:"description" bson:"description"`
	Payee         string            `json:"payee" bson:"payee"`
	Account       BankAccount       `json:"account" bson:"account"`
	Status        TransactionStatus `json:"status" bson:"status"`
	CreatedAt     time.Time         `json:"createdAt" bson:"created_at"`
	TxnTime       time.Time         `json:"txnTime" bson:"txn_time"`
	RawText       string            `json:"rawText" bson:"rawText"`
	RawLineNumber int               `json:"raw_line_number" bson:"raw_line_number"`
	ErrorMsg      string            `json:"errorMsg" bson:"error_msg"`
}

// BankAccount represents the bank account a transaction belongs to.
type BankAccount struct {
	ID   string `json:"id" bson:"_id"`
	Name string `json:"name" bson:"name"`
}
