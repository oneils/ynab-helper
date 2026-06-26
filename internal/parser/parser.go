// Package parser provides parsers for bank CSV exports.
package parser

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	warsawLocation  = "Europe/Warsaw"
	defaultCurrency = "PLN"

	SantanderBankName = "Santander"
	RevolutBankName   = "Revolut"
	PKOBankName       = "PKO"
)

//go:generate moq -out hasher_mock.go -fmt goimports . Hasher
//go:generate moq -out time_provider_mock.go -fmt goimports . TimeProvider

// Config represents configuration for a parser.
type Config struct {
	TransactionDateIndex int
	DescriptionIndex     int
	AmountIndex          int
	FeeIndex             int
	CurrencyIndex        int
	DateFormat           string
	BankName             string
	ColumnsAmount        int
	Header               HeaderCfg
}

// HeaderCfg represents configuration for a header.
type HeaderCfg struct {
	HasHeader      bool
	ValidateHeader bool
	ExpectedHeader string
}

// Hasher represents a hasher interface for hashing report data.
type Hasher interface {
	Write(p []byte) (n int, err error)
	Sum(b []byte) []byte
	Reset()
}

// TimeProvider provides current time. Used for testing.
type TimeProvider interface {
	Now() time.Time
}

// RealTimeProvider is the production time provider.
type RealTimeProvider struct{}

// Now returns current time.
func (RealTimeProvider) Now() time.Time {
	return time.Now()
}

func getCurrency(cfg Config, row []string) string {
	if cfg.CurrencyIndex <= 0 {
		return defaultCurrency
	}
	return row[cfg.CurrencyIndex]
}

func getAmount(amountIndex int, row []string) (float64, error) {
	amountStr := strings.ReplaceAll(row[amountIndex], ",", ".")
	return strconv.ParseFloat(amountStr, 64)
}

func validColumnsAmount(expectedAmount int, row []string) error {
	if len(row) != expectedAmount {
		return fmt.Errorf("invalid columns amount. Expected %d, got %d", expectedAmount, len(row))
	}
	return nil
}
