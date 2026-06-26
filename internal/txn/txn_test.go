package txn

import "testing"

func TestTransactionStatus_DisplayName(t *testing.T) {
	tests := []struct {
		status   TransactionStatus
		expected string
	}{
		{TransactionDraft, "Needs Review"},
		{TransactionProcessed, "Accepted"},
		{TransactionSkipped, "Skipped"},
		{TransactionInvalid, "Invalid"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := tt.status.DisplayName()
			if got != tt.expected {
				t.Errorf("DisplayName() = %q, want %q", got, tt.expected)
			}
		})
	}
}
