package parser

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"github.com/oneils/ynab-helper/internal/txn"
)

const pkoPattern = `Lokalizacja:\s*Adres:\s*(.+?)(?:\s+Miasto:|$)|Nazwa odbiorcy:\s*([^,\n]+)|Nazwa nadawcy:\s*([^\n]+?)(?:\s+Adres:|$)`

// PKOParser parses PKO bank CSV exports.
type PKOParser struct {
	cfg          Config
	hasher       Hasher
	timeProvider TimeProvider
}

// NewPKOParser creates a new PKOParser.
func NewPKOParser(cfg Config, hasher Hasher, timeProvider TimeProvider) PKOParser {
	return PKOParser{
		cfg:          cfg,
		hasher:       hasher,
		timeProvider: timeProvider,
	}
}

// Parse parses PKO CSV data into transactions.
func (p PKOParser) Parse(acc txn.BankAccount, data [][]string) []txn.Transaction {
	var txns []txn.Transaction

	if len(data) == 0 {
		return txns
	}

	if p.cfg.Header.HasHeader {
		data = data[1:]
	}

	for lineNumber, row := range data {
		rowString := strings.Join(row, ",")

		rowString, err := p.convertToUTF8(rowString)
		if err != nil {
			errMsg := fmt.Sprintf("can't convert to utf-8 raw string[%s] at line %d: %s", rowString, lineNumber, err.Error())
			txns = append(txns, txn.Transaction{
				Account:       acc,
				Status:        txn.TransactionInvalid,
				RawLineNumber: lineNumber,
				RawText:       rowString,
				ErrorMsg:      errMsg,
				CreatedAt:     p.timeProvider.Now(),
			})
			slog.Warn(errMsg)
			continue
		}

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

		amount, err := getAmount(p.cfg.AmountIndex, row)
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

		// Try column 7 first (DescriptionIndex - for card payments "Lokalizacja: Adres: SHOP...")
		payee := p.shopName(row[p.cfg.DescriptionIndex])
		// If no match and we have more columns, try column 8 (DescriptionIndex+1 - for transfers "Nazwa odbiorcy/nadawcy:")
		if payee == "" && len(row) > p.cfg.DescriptionIndex+1 {
			payee = p.shopName(row[p.cfg.DescriptionIndex+1])
		}
		payee, err = p.convertToUTF8(payee)
		if err != nil {
			errMsg := fmt.Sprintf("can't convert to utf-8 payee[%s] at line %d: %s", payee, lineNumber, err.Error())
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

		description := row[p.cfg.DescriptionIndex+1]
		description, err = p.convertToUTF8(description)
		if err != nil {
			errMsg := fmt.Sprintf("can't convert to utf-8 description[%s] at line %d: %s", description, lineNumber, err.Error())
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

func (p PKOParser) shopName(text string) string {
	re := regexp.MustCompile(pkoPattern)
	matches := re.FindAllStringSubmatch(text, -1)

	if len(matches) == 0 {
		return ""
	}

	match := re.FindStringSubmatch(text)

	var shopName string
	if len(match) >= 3 {
		for i := 1; i < len(match); i++ {
			if match[i] != "" {
				shopName = strings.TrimSpace(match[i])
			}
		}
	}

	return strings.TrimSpace(shopName)
}

// convertToUTF8 converts a string from Windows-1250 to UTF-8.
func (p PKOParser) convertToUTF8(input string) (string, error) {
	decoder := charmap.Windows1250.NewDecoder()
	utf8Bytes, _, err := transform.String(decoder, input)
	if err != nil {
		return "", err
	}
	return utf8Bytes, nil
}
