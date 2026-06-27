package ynab

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type mockSyncRunner struct {
	budgets         []Budget
	syncBudgetsErr  error
	fetchBudgetsErr error
	syncAccountsErr error
	syncCatsErr     error
	syncPayeesErr   error

	syncBudgetsCalls  atomic.Int32
	fetchBudgetsCalls atomic.Int32
	syncAccountsCalls atomic.Int32
	syncCatsCalls     atomic.Int32
	syncPayeesCalls   atomic.Int32
}

func (m *mockSyncRunner) SyncBudgets(_ context.Context) error {
	m.syncBudgetsCalls.Add(1)
	return m.syncBudgetsErr
}

func (m *mockSyncRunner) FetchBudgets(_ context.Context) ([]Budget, error) {
	m.fetchBudgetsCalls.Add(1)
	return m.budgets, m.fetchBudgetsErr
}

func (m *mockSyncRunner) SyncAccounts(_ context.Context, _ string) error {
	m.syncAccountsCalls.Add(1)
	return m.syncAccountsErr
}

func (m *mockSyncRunner) SyncCategories(_ context.Context, _ string) error {
	m.syncCatsCalls.Add(1)
	return m.syncCatsErr
}

func (m *mockSyncRunner) SyncPayees(_ context.Context, _ string) error {
	m.syncPayeesCalls.Add(1)
	return m.syncPayeesErr
}

func TestSyncAll_HappyPath(t *testing.T) {
	runner := &mockSyncRunner{
		budgets: []Budget{{ID: "b1"}, {ID: "b2"}},
	}
	s := NewScheduler(runner, time.Hour)
	s.syncAll(context.Background())

	if runner.syncBudgetsCalls.Load() != 1 {
		t.Errorf("SyncBudgets called %d times, want 1", runner.syncBudgetsCalls.Load())
	}
	if runner.syncAccountsCalls.Load() != 2 {
		t.Errorf("SyncAccounts called %d times, want 2", runner.syncAccountsCalls.Load())
	}
	if runner.syncCatsCalls.Load() != 2 {
		t.Errorf("SyncCategories called %d times, want 2", runner.syncCatsCalls.Load())
	}
	if runner.syncPayeesCalls.Load() != 2 {
		t.Errorf("SyncPayees called %d times, want 2", runner.syncPayeesCalls.Load())
	}
}

func TestSyncAll_SyncBudgetsError(t *testing.T) {
	runner := &mockSyncRunner{
		budgets:        []Budget{{ID: "b1"}},
		syncBudgetsErr: errors.New("network error"),
	}
	s := NewScheduler(runner, time.Hour)
	s.syncAll(context.Background())

	if runner.syncBudgetsCalls.Load() != 1 {
		t.Errorf("SyncBudgets called %d times, want 1", runner.syncBudgetsCalls.Load())
	}
	// per-budget syncs must not be called when SyncBudgets fails
	if runner.syncAccountsCalls.Load() != 0 {
		t.Errorf("SyncAccounts called %d times, want 0", runner.syncAccountsCalls.Load())
	}
}

func TestSyncAll_PerBudgetErrorContinues(t *testing.T) {
	runner := &mockSyncRunner{
		budgets:         []Budget{{ID: "b1"}, {ID: "b2"}},
		syncAccountsErr: errors.New("accounts error"),
		syncCatsErr:     errors.New("categories error"),
	}
	s := NewScheduler(runner, time.Hour)
	s.syncAll(context.Background())

	// SyncPayees must still be called for both budgets despite errors above
	if runner.syncPayeesCalls.Load() != 2 {
		t.Errorf("SyncPayees called %d times, want 2", runner.syncPayeesCalls.Load())
	}
}

func TestStart_RunsImmediatelyThenOnTick(t *testing.T) {
	runner := &mockSyncRunner{budgets: []Budget{{ID: "b1"}}}
	interval := 50 * time.Millisecond
	s := NewScheduler(runner, interval)

	ctx, cancel := context.WithTimeout(context.Background(), 130*time.Millisecond)
	defer cancel()

	s.Start(ctx)

	// immediate run + 2 ticks within 130ms at 50ms interval
	calls := runner.syncBudgetsCalls.Load()
	if calls < 2 {
		t.Errorf("SyncBudgets called %d times, want at least 2 (immediate + 1 tick)", calls)
	}
}
