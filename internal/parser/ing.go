package parser

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"github.com/oneils/ynab-helper/internal/txn"
)

const ingHeaderMarker = "Data transakcji"

// INGParser parses ING Bank Śląski S.A. CSV exports.
type INGParser struct {
	cfg          Config
	hasher       Hasher
	timeProvider TimeProvider
}

// NewINGParser creates a new INGParser.
func NewINGParser(cfg Config, hasher Hasher, timeProvider TimeProvider) INGParser {
	return INGParser{
		cfg:          cfg,
		hasher:       hasher,
		timeProvider: timeProvider,
	}
}

// Parse parses ING CSV data into transactions.
func (p INGParser) Parse(acc txn.BankAccount, data [][]string) []txn.Transaction {
	var txns []txn.Transaction

	if len(data) == 0 {
		return txns
	}

	dataStart := 0
	for i, row := range data {
		if len(row) > 0 && strings.TrimSpace(row[0]) == ingHeaderMarker {
			dataStart = i + 1
			break
		}
	}
	data = data[dataStart:]

	location, _ := time.LoadLocation(warsawLocation)
	createdAt := p.timeProvider.Now()

	for lineNumber, row := range data {
		rowString := strings.Join(row, ",")

		if len(row) < p.cfg.ColumnsAmount {
			continue
		}

		if strings.TrimSpace(row[p.cfg.AmountIndex]) == "" {
			continue
		}

		amount, err := getAmount(p.cfg.AmountIndex, row)
		if err != nil {
			errMsg := fmt.Sprintf("invalid amount [%s] at line %d. Error: %s", row[p.cfg.AmountIndex], lineNumber, err.Error())
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

		payee, err := p.convertToUTF8(row[2])
		if err != nil {
			errMsg := fmt.Sprintf("can't convert to utf-8 payee[%s] at line %d: %s", row[2], lineNumber, err.Error())
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

		description, err := p.convertToUTF8(row[p.cfg.DescriptionIndex])
		if err != nil {
			errMsg := fmt.Sprintf("can't convert to utf-8 description[%s] at line %d: %s", row[p.cfg.DescriptionIndex], lineNumber, err.Error())
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
		if _, err = p.hasher.Write([]byte(buildHashInput(p.cfg, row))); err != nil {
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
			Description: description,
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

// convertToUTF8 converts a string from Windows-1250 to UTF-8.
func (p INGParser) convertToUTF8(input string) (string, error) {
	decoder := charmap.Windows1250.NewDecoder()
	utf8Bytes, _, err := transform.String(decoder, input)
	if err != nil {
		return "", err
	}
	return utf8Bytes, nil
}
