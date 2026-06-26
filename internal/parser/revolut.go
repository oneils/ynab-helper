package parser

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

// RevolutParser parses Revolut bank CSV exports.
type RevolutParser struct {
	cfg          Config
	hasher       Hasher
	timeProvider TimeProvider
}

// NewRevolutParser creates a new RevolutParser.
func NewRevolutParser(cfg Config, hasher Hasher, timeProvider TimeProvider) RevolutParser {
	return RevolutParser{
		cfg:          cfg,
		hasher:       hasher,
		timeProvider: timeProvider,
	}
}

// Parse parses Revolut CSV data into transactions.
func (p RevolutParser) Parse(acc txn.BankAccount, data [][]string) []txn.Transaction {
	var txns []txn.Transaction

	if len(data) == 0 {
		return txns
	}

	if p.cfg.Header.HasHeader {
		data = data[1:]
	}

	for lineNumber, row := range data {
		rowString := strings.Join(row, ",")
		createdAt := p.timeProvider.Now()

		if err := validColumnsAmount(p.cfg.ColumnsAmount, row); err != nil {
			txns = append(txns, txn.Transaction{
				Account:       acc,
				Status:        txn.TransactionInvalid,
				RawLineNumber: lineNumber,
				RawText:       rowString,
				ErrorMsg:      err.Error(),
				CreatedAt:     createdAt,
			})
			slog.Warn("invalid line", "line", lineNumber, "error", err)
			continue
		}

		initAmount, err := getAmount(p.cfg.AmountIndex, row)
		if err != nil {
			errMsg := fmt.Sprintf("invalid amount [%s] at line %d. Error: %s", row[p.cfg.AmountIndex], lineNumber, err.Error())
			txns = append(txns, txn.Transaction{
				Account:       acc,
				RawLineNumber: lineNumber,
				Status:        txn.TransactionInvalid,
				RawText:       rowString,
				ErrorMsg:      errMsg,
				CreatedAt:     createdAt,
			})
			slog.Warn(errMsg)
			continue
		}

		fee, err := getAmount(p.cfg.FeeIndex, row)
		if err != nil {
			errMsg := fmt.Sprintf("invalid fee amount [%s] at line %d. Error: %s", row[p.cfg.FeeIndex], lineNumber, err.Error())
			txns = append(txns, txn.Transaction{
				Account:       acc,
				RawLineNumber: lineNumber,
				Status:        txn.TransactionInvalid,
				RawText:       rowString,
				ErrorMsg:      errMsg,
				CreatedAt:     createdAt,
			})
			slog.Warn(errMsg)
			continue
		}

		amount := initAmount - fee

		location, _ := time.LoadLocation(warsawLocation)
		date, err := time.ParseInLocation(p.cfg.DateFormat, row[p.cfg.TransactionDateIndex], location)
		if err != nil {
			errMsg := fmt.Sprintf("invalid transaction time [%s] at line %d: %s", row[p.cfg.TransactionDateIndex], lineNumber, err.Error())
			txns = append(txns, txn.Transaction{
				Account:       acc,
				Status:        txn.TransactionInvalid,
				RawLineNumber: lineNumber,
				RawText:       rowString,
				ErrorMsg:      errMsg,
				CreatedAt:     createdAt,
			})
			slog.Warn(errMsg)
			continue
		}

		p.hasher.Reset()
		if _, err = p.hasher.Write([]byte(rowString)); err != nil {
			errMsg := fmt.Sprintf("can't generate hash for raw string[%s] at line %d: %s", rowString, lineNumber, err.Error())
			txns = append(txns, txn.Transaction{
				Account:       acc,
				Status:        txn.TransactionInvalid,
				RawLineNumber: lineNumber,
				RawText:       rowString,
				ErrorMsg:      errMsg,
				CreatedAt:     createdAt,
			})
			slog.Warn(errMsg)
			continue
		}
		sum := hex.EncodeToString(p.hasher.Sum(nil))

		payee := row[p.cfg.DescriptionIndex]

		txns = append(txns, txn.Transaction{
			Account:     acc,
			ID:          sum,
			Description: row[p.cfg.DescriptionIndex],
			Amount:      amount,
			Currency:    getCurrency(p.cfg, row),
			CreatedAt:   createdAt,
			TxnTime:     date,
			Status:      txn.TransactionDraft,
			Payee:       payee,
			RawText:     rowString,
		})
	}

	return txns
}
