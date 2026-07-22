package parser

import (
	"testing"
	"time"
)

func TestValidColumnsAmount(t *testing.T) {
	tests := []struct {
		name        string
		expected    int
		actual      []string
		shouldError bool
	}{
		{
			name:        "Valid column count",
			expected:    5,
			actual:      []string{"col1", "col2", "col3", "col4", "col5"},
			shouldError: false,
		},
		{
			name:        "Too few columns",
			expected:    5,
			actual:      []string{"col1", "col2"},
			shouldError: true,
		},
		{
			name:        "Too many columns",
			expected:    5,
			actual:      []string{"col1", "col2", "col3", "col4", "col5", "col6"},
			shouldError: true,
		},
		{
			name:        "Empty row",
			expected:    5,
			actual:      []string{},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validColumnsAmount(tt.expected, tt.actual)
			if (err != nil) != tt.shouldError {
				t.Errorf("validColumnsAmount() error = %v, shouldError = %v", err, tt.shouldError)
			}
		})
	}
}

func TestGetAmount(t *testing.T) {
	tests := []struct {
		name        string
		index       int
		row         []string
		expected    float64
		shouldError bool
	}{
		{
			name:        "Valid positive amount",
			index:       0,
			row:         []string{"123.45"},
			expected:    123.45,
			shouldError: false,
		},
		{
			name:        "Valid negative amount",
			index:       0,
			row:         []string{"-123.45"},
			expected:    -123.45,
			shouldError: false,
		},
		{
			name:        "Amount with comma decimal separator",
			index:       0,
			row:         []string{"123,45"},
			expected:    123.45,
			shouldError: false,
		},
		{
			name:        "Invalid amount",
			index:       0,
			row:         []string{"invalid"},
			expected:    0,
			shouldError: true,
		},
		{
			name:        "Empty string",
			index:       0,
			row:         []string{""},
			expected:    0,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getAmount(tt.index, tt.row)
			if (err != nil) != tt.shouldError {
				t.Errorf("getAmount() error = %v, shouldError = %v", err, tt.shouldError)
				return
			}
			if !tt.shouldError && result != tt.expected {
				t.Errorf("getAmount() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetCurrency(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		row      []string
		expected string
	}{
		{
			name: "Explicit currency from row",
			cfg: Config{
				CurrencyIndex: 2,
			},
			row:      []string{"col1", "col2", "EUR"},
			expected: "EUR",
		},
		{
			name: "Default currency when index is 0",
			cfg: Config{
				CurrencyIndex: 0,
			},
			row:      []string{"col1"},
			expected: defaultCurrency,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCurrency(tt.cfg, tt.row)
			if result != tt.expected {
				t.Errorf("getCurrency() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBuildHashInput(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		row      []string
		expected string
	}{
		{
			name:     "Empty HashColumns joins full row",
			cfg:      Config{},
			row:      []string{"a", "b", "c"},
			expected: "a,b,c",
		},
		{
			name:     "Non-empty HashColumns includes only selected columns",
			cfg:      Config{HashColumns: []int{0, 2}},
			row:      []string{"a", "b", "c"},
			expected: "a,c",
		},
		{
			name:     "HashColumns order determines output order",
			cfg:      Config{HashColumns: []int{2, 0}},
			row:      []string{"a", "b", "c"},
			expected: "c,a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildHashInput(tt.cfg, tt.row)
			if result != tt.expected {
				t.Errorf("buildHashInput() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRealTimeProvider_Now(t *testing.T) {
	// Arrange
	provider := RealTimeProvider{}

	// Act
	before := time.Now()
	result := provider.Now()
	after := time.Now()

	// Assert
	if result.Before(before) || result.After(after) {
		t.Errorf("RealTimeProvider.Now() returned time outside expected range")
	}
}
