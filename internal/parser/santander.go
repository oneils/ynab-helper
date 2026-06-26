package parser

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/oneils/ynab-helper/internal/txn"
)

const santanderPattern = `PŁATNOŚĆ KARTĄ ([\d.]+) PLN ([A-ZĄĆĘŁŃÓŚŹŻa-ząćęłńóśźż0-9\s]+)|Zakup BLIK ([A-ZĄĆĘŁŃÓŚŹŻa-ząćęłńóśźż0-9\s]+)`

// SantanderParser parses Santander bank CSV exports.
type SantanderParser struct {
	cfg          Config
	hasher       Hasher
	timeProvider TimeProvider
}

// NewSantanderParser creates a new SantanderParser.
func NewSantanderParser(cfg Config, hasher Hasher, timeProvider TimeProvider) SantanderParser {
	return SantanderParser{
		cfg:          cfg,
		hasher:       hasher,
		timeProvider: timeProvider,
	}
}

// Parse parses Santander CSV data into transactions.
func (p SantanderParser) Parse(acc txn.BankAccount, data [][]string) []txn.Transaction {
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

		location, _ := time.LoadLocation(warsawLocation)
		date, err := time.ParseInLocation(p.cfg.DateFormat, row[p.cfg.TransactionDateIndex], location)
		if err != nil {
			errMsg := fmt.Sprintf("invalid transaction time [%s] at line %d: %s", row[p.cfg.AmountIndex], lineNumber, err.Error())
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

		payee := p.shopName(row[p.cfg.DescriptionIndex])

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
			Payee:       payee,
		})
	}

	return txns
}

func (p SantanderParser) shopName(text string) string {
	re := regexp.MustCompile(santanderPattern)
	matches := re.FindAllStringSubmatch(text, -1)

	if len(matches) == 0 {
		return ""
	}

	match := matches[0]

	var shopName string
	if len(match) >= 3 && match[3] != "" {
		shopName = match[3]
	} else if len(match) >= 3 {
		shopName = match[2]
		if strings.Contains(shopName, " ") && !strings.HasSuffix(shopName, " ") {
			parts := strings.Split(shopName, " ")
			shopName = parts[0]
		}
	}

	return strings.TrimSpace(shopName)
}
