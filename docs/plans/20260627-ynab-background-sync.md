# YNAB Background Sync Scheduler

## Overview

Add a background `Scheduler` that periodically syncs budgets, accounts, categories, and payees from YNAB without requiring manual HTTP triggers. Manual sync endpoints remain available. The scheduler runs on startup and then repeats at a configurable interval (default: 1 hour).

The sync order per cycle: budgets → then for each known budget: accounts, categories, payees.

## Context (from discovery)

- **Existing sync logic**: `internal/ynab/sync.go` — `Syncer.SyncBudgets`, `SyncAccounts`, `SyncCategories`, `SyncPayees`
- **Manual HTTP triggers**: `POST /ynab-budgets-sync`, `/ynab-accs-sync`, `/ynab-categories-sync`, `/ynab-payee-sync` in `internal/server/server.go` — kept as-is
- **Background goroutine pattern**: `server.Run()` file-cleanup ticker (lines 86–103 of `server.go`) — same `ticker + ctx.Done()` pattern used here
- **App wiring**: `internal/app/app.go` — `App.Run()` delegates to `Server.Run()`; `Scheduler` will be started from `App.Run()` before the server
- **Config**: `Config` struct in `internal/app/app.go` — add `SyncInterval` field there
- **Budget discovery**: `Syncer.FetchBudgets(ctx)` reads local DB; used after `SyncBudgets` to get IDs for per-budget syncs

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- All tests must pass before starting the next task
- Update this plan file when scope changes

## Testing Strategy

- **Unit tests**: mock `Syncer` via an interface to test `Scheduler` in isolation
- **Integration**: existing manual sync HTTP handlers are unchanged — no e2e change needed

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document blockers with ⚠️ prefix

## Implementation Steps

---

### Task 1: Add `SyncInterval` config field

**Files:**
- Modify: `internal/app/app.go`

- [ ] add `SyncInterval time.Duration` to `Config` with tag `long:"sync-interval" env:"SYNC_INTERVAL" default:"1h" description:"Interval between automatic YNAB syncs"`
- [ ] verify `go build ./...` passes

---

### Task 2: Define `BudgetSyncRunner` interface and implement `Scheduler`

**Files:**
- Create: `internal/ynab/scheduler.go`

- [ ] define interface `BudgetSyncRunner` with methods used by Scheduler:
  ```go
  type BudgetSyncRunner interface {
      SyncBudgets(ctx context.Context) error
      FetchBudgets(ctx context.Context) ([]Budget, error)
      SyncAccounts(ctx context.Context, budgetID string) error
      SyncCategories(ctx context.Context, budgetID string) error
      SyncPayees(ctx context.Context, budgetID string) error
  }
  ```
- [ ] implement `Scheduler` struct holding `runner BudgetSyncRunner` and `interval time.Duration`
- [ ] implement `NewScheduler(runner BudgetSyncRunner, interval time.Duration) *Scheduler`
- [ ] implement `Start(ctx context.Context)` — runs `syncAll` immediately, then ticks at `interval`; returns when `ctx` is cancelled
- [ ] implement private `syncAll(ctx context.Context)` — calls `SyncBudgets`, then for each budget ID calls `SyncAccounts`, `SyncCategories`, `SyncPayees`; logs errors with `slog.Error` and continues on failure (never returns error)
- [ ] verify `go build ./...` passes

---

### Task 3: Wire `Scheduler` into `App`

**Files:**
- Modify: `internal/app/app.go`

- [ ] add `Scheduler *ynab.Scheduler` field to `App` struct
- [ ] in `New()`, after creating `syncer`, create `ynab.NewScheduler(syncer, cfg.SyncInterval)` and assign to `App.Scheduler`
- [ ] in `App.Run(ctx)`, launch `go a.Scheduler.Start(ctx)` before calling `a.Server.Run(ctx, a.Config.Addr)`
- [ ] verify `go build ./...` and `go run ./cmd/ynab-helper` starts without errors

---

### Task 4: Tests for `Scheduler`

**Files:**
- Create: `internal/ynab/scheduler_test.go`

- [ ] define `mockSyncRunner` implementing `BudgetSyncRunner` with call counters and configurable error returns
- [ ] test `syncAll` happy path: after one call, `SyncBudgets` called once and per-budget methods called once per budget
- [ ] test `syncAll` when `SyncBudgets` errors: logs error, per-budget sync methods not called
- [ ] test `syncAll` when a per-budget sync errors: logs error, continues to next budget/resource
- [ ] test `Start` runs `syncAll` immediately on start (no tick needed), then again after interval
- [ ] run `go test ./internal/ynab/...` — must pass

---

### Task 5: Verify acceptance criteria

- [ ] start the app and observe logs: background sync runs immediately on startup
- [ ] wait past interval (or set `SYNC_INTERVAL=5s` temporarily) and confirm second sync fires
- [ ] manually trigger `POST /ynab-budgets-sync` — confirm manual sync still works
- [ ] confirm `sync_history` table records entries from the background job
- [ ] run full test suite: `go test ./...`

---

### Task N-1: Final cleanup

- [ ] run `go vet ./...` — no issues
- [ ] move this plan to `docs/plans/completed/`

## Technical Details

**Scheduler struct:**
```go
type Scheduler struct {
    runner   BudgetSyncRunner
    interval time.Duration
}

func (s *Scheduler) Start(ctx context.Context) {
    s.syncAll(ctx)                       // immediate run on startup
    ticker := time.NewTicker(s.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            s.syncAll(ctx)
        case <-ctx.Done():
            return
        }
    }
}
```

**Sync order within `syncAll`:**
1. `SyncBudgets(ctx)` — populates DB with current budget list
2. `FetchBudgets(ctx)` — reads IDs from DB
3. For each budget: `SyncAccounts`, `SyncCategories`, `SyncPayees` (errors logged, not fatal)

**Config addition:**
```go
SyncInterval time.Duration `long:"sync-interval" env:"SYNC_INTERVAL" default:"1h" description:"Interval between automatic YNAB syncs"`
```

**`*Syncer` already satisfies `BudgetSyncRunner`** — no changes to `Syncer` needed.

## Post-Completion

**Manual verification:**
- Monitor `sync_history` table via SQLite browser to confirm rows inserted by background job
- Check logs contain `slog` entries from `syncAll` for each resource type

**Deployment:**
- Set `SYNC_INTERVAL` env var to tune frequency (e.g., `SYNC_INTERVAL=2h`)
