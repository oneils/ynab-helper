package ynab

import (
	"context"
	"log/slog"
	"time"
)

// BudgetSyncRunner is the subset of Syncer used by the Scheduler.
type BudgetSyncRunner interface {
	SyncBudgets(ctx context.Context) error
	FetchBudgets(ctx context.Context) ([]Budget, error)
	SyncAccounts(ctx context.Context, budgetID string) error
	SyncCategories(ctx context.Context, budgetID string) error
	SyncPayees(ctx context.Context, budgetID string) error
}

// Scheduler runs a full YNAB sync on startup and then on a fixed interval.
type Scheduler struct {
	runner   BudgetSyncRunner
	interval time.Duration
}

// NewScheduler creates a Scheduler that uses runner and repeats every interval.
func NewScheduler(runner BudgetSyncRunner, interval time.Duration) *Scheduler {
	return &Scheduler{runner: runner, interval: interval}
}

// Start runs an immediate sync then repeats every s.interval until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	s.syncAll(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.syncAll(ctx)
		case <-ctx.Done():
			slog.Info("stopping background YNAB sync scheduler")
			return
		}
	}
}

func (s *Scheduler) syncAll(ctx context.Context) {
	slog.Info("background YNAB sync started")

	if err := s.runner.SyncBudgets(ctx); err != nil {
		slog.Error("background sync: SyncBudgets failed", "error", err)
		return
	}

	budgets, err := s.runner.FetchBudgets(ctx)
	if err != nil {
		slog.Error("background sync: FetchBudgets failed", "error", err)
		return
	}

	for _, b := range budgets {
		if err := s.runner.SyncAccounts(ctx, b.ID); err != nil {
			slog.Error("background sync: SyncAccounts failed", "budget_id", b.ID, "error", err)
		}
		if err := s.runner.SyncCategories(ctx, b.ID); err != nil {
			slog.Error("background sync: SyncCategories failed", "budget_id", b.ID, "error", err)
		}
		if err := s.runner.SyncPayees(ctx, b.ID); err != nil {
			slog.Error("background sync: SyncPayees failed", "budget_id", b.ID, "error", err)
		}
	}

	slog.Info("background YNAB sync completed", "budgets", len(budgets))
}
