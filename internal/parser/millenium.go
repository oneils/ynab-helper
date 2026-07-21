package parser

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

// MilleniumParser parses Bank Millennium CSV exports.
type MilleniumParser struct {
	cfg          Config
	hasher       Hasher
	timeProvider TimeProvider
}

// NewMilleniumParser creates a new MilleniumParser.
func NewMilleniumParser(cfg Config, hasher Hasher, timeProvider TimeProvider) MilleniumParser {
	return MilleniumParser{
		cfg:          cfg,
		hasher:       hasher,
		timeProvider: timeProvider,
	}
}

// Parse parses Millennium CSV data into transactions.
func (p MilleniumParser) Parse(acc txn.BankAccount, data [][]string) []txn.Transaction {
	var txns []txn.Transaction

	if len(data) == 0 {
		return txns
	}

	if p.cfg.Header.HasHeader {
		data = data[1:]
	}

	createdAt := p.timeProvider.Now()

	for lineNumber, row := range data {
		rowString := strings.Join(row, ",")

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

		amount, err := getMilleniumAmount(row, p.cfg.AmountIndex, p.cfg.AmountIndex+1)
		if err != nil {
			errMsg := fmt.Sprintf("invalid amount at line %d. Error: %s", lineNumber, err.Error())
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

		txns = append(txns, txn.Transaction{
			Account:     acc,
			ID:          sum,
			Description: row[p.cfg.DescriptionIndex],
			Amount:      amount,
			Currency:    getCurrency(p.cfg, row),
			CreatedAt:   createdAt,
			TxnTime:     date,
			Status:      txn.TransactionDraft,
			Payee:       row[p.cfg.DescriptionIndex],
		})
	}

	return txns
}

// getMilleniumAmount extracts the amount from Millennium's two-column debit/credit layout.
// Exactly one of the debit or credit columns is expected to be non-empty; the debit
// value is already pre-signed negative and must not be negated.
func getMilleniumAmount(row []string, debitIdx, creditIdx int) (float64, error) {
	if row[debitIdx] != "" {
		amount, err := getAmount(debitIdx, row)
		if err != nil {
			return 0, fmt.Errorf("invalid debit amount [%s]: %w", row[debitIdx], err)
		}
		return amount, nil
	}

	if row[creditIdx] != "" {
		amount, err := getAmount(creditIdx, row)
		if err != nil {
			return 0, fmt.Errorf("invalid credit amount [%s]: %w", row[creditIdx], err)
		}
		return amount, nil
	}

	return 0, fmt.Errorf("both debit and credit columns are empty")
}
