# Exclude Manually-Entered YNAB Transactions from CSV Import

## Overview

Today, duplicate detection during CSV import only catches transactions this
app has already pushed to YNAB (matched by an internal hash-ID computed from
stable CSV columns — see `internal/parser/*.go` and
`docs/plans/completed/20260722-fix-duplicate-transactions.md`). It does not
catch transactions the user typed directly into YNAB by hand: those never
got our hash ID, so the same real-world transaction re-appears in a later
CSV import as "new," and if accepted gets pushed to YNAB a second time.

This plan adds a second detection path: a local cache of the budget's actual
YNAB transactions (kept fresh by the existing hourly background sync), and a
field-based match (account + amount + a ±3-day date window, to absorb bank
settlement-date drift) applied during CSV preview/import. A match is
auto-skipped (not silently dropped — inserted with status `SKIPPED` and a
visible note) so it never clutters "Needs Review" and never gets
auto-sent, but stays recoverable if the match was a false positive.

**Split, as requested:**
- **Part 1**: sync YNAB's real transactions into a new local table, on the
  same schedule as the existing budgets/accounts/categories/payees sync.
- **Part 2**: at import time, compare CSV rows against that local cache and
  skip anything that matches an existing YNAB transaction.

## Context (from discovery)

- **Scheduler**: `internal/ynab/scheduler.go` — `Scheduler.Start`/`syncAll`
  already runs on startup + every `SYNC_INTERVAL` (default `1h`, configured
  in `internal/app/app.go`), looping per budget through
  `SyncAccounts`/`SyncCategories`/`SyncPayees` via the `BudgetSyncRunner`
  interface. A failure in one sync type is logged and the loop continues
  (fail-open) — this pattern must be preserved for the new sync call.
- **Naming convention**: existing tables/types (`budgets`, `accounts`,
  `categories`, `payees`) intentionally carry NO `ynab_`/`Ynab` prefix, even
  though today they're populated exclusively from the YNAB API — deliberate,
  since the user plans to eventually decouple this app from YNAB
  specifically. The new table is `budget_transactions` / type `BudgetTxn`,
  matching this convention (confirmed with user, `ynab_transactions` was
  explicitly rejected).
- **Existing dedup** (`internal/txn/processor.go` `Preview`, ~line 210):
  checks `FindTransactionByID` against the internal hash ID and sets a
  preview-only display string `"DUPLICATE"` (not a real `TransactionStatus`
  const) — the new "already in YNAB" case follows the exact same
  display-only-marker pattern, additively, without touching this check.
- **Amount conversion**: `int(math.Round(amountFloat * 1000))` already
  exists at `internal/txn/processor.go:263` (used when building the YNAB
  upload request) — reuse this exact conversion for matching; do not
  reimplement it.
- **`YnabStore`** (`internal/sqlite/ynab_store.go`, 1039 lines) is a single
  struct implementing every YNAB-domain storer interface
  (`BudgetStorer`/`AccountStorer`/`CategoryStorer`/`PayeeStorer`/
  `HistoryStorer`/`BudgetFinder`) — `internal/app/app.go` wires the same
  `ynabStore` instance into both `NewSyncer` and `NewProcessor`. The new
  `BudgetTxnStorer`/`BudgetTxnFinder` methods are added to this same struct,
  not a new file, to match this established pattern.
- **`YnabClient` interface** (`internal/ynab/sync.go`) is implemented by both
  the real `Client` (`internal/ynab/client.go`) and `FakeClient`
  (`internal/ynab/fake_client.go`, used in `--mock-ynab` test mode, backed by
  `Fixture` in `internal/ynab/fixture.go`) — both need the new
  `FetchTransactions` method.
- **Migrations**: goose-based, embedded via `//go:embed migrations/*.sql`
  in `internal/sqlite/sqlite.go`. Latest SQL migration is `00004`; a Go
  migration `00005_refingerprint_patterns.go` (in the separate
  `migrations/go` subpackage) already occupies version 5. Next SQL
  migrations are therefore `00006` and `00007`.
- **`transactions` table / `TransactionStore`**
  (`internal/sqlite/transaction_store.go`): `InsertTransaction`,
  `FetchTransactionsByAccount`, `FindTransactionByID` all use an explicit
  column list that must be extended for the new nullable `match_note`
  column.
- **`PRAGMA foreign_keys=ON`** is set in `internal/sqlite/sqlite.go:75` — any
  insert into `budget_transactions` with a `budget_id` that doesn't match an
  existing `budgets.id` (or is empty) fails the FK constraint. The YNAB
  `/transactions` API response does **not** include `budget_id` on each
  transaction (it's implied by the URL) — so `Syncer.SyncTransactions` MUST
  stamp `t.BudgetID = budgetID` on every fetched row before upserting, the
  same way `SyncCategories` (`internal/ynab/sync.go`, `cat.BudgetID =
  budgetID`) and `SyncPayees` (`payee.BudgetID = budgetID`) already do.
  Missing this means every upsert fails silently (logged, sync "succeeds"
  with 0 rows) and the whole feature is a no-op.
- **Existing `Preview` bug to fix, not copy**: `internal/txn/processor.go`'s
  `for i, txn := range txns` loop mutates `txns[i].Status = "DUPLICATE"` but
  appends the per-iteration range variable `txn` (a stale copy) to
  `result.Transactions`, not `txns[i]`. This means the `"DUPLICATE"` status
  never actually reaches the preview template today — the
  `{{else if eq .Status "DUPLICATE"}}` branch in
  `bank-transactions-preview.tmpl.html` is currently dead code. The new
  `"ALREADY_IN_YNAB"` marker must NOT inherit this bug (see Task 6).
- **Interface fan-out**: `YnabClient` (in `internal/ynab/sync.go`) has two
  test-double implementers beyond `FakeClient` —
  `mockYnabClient` in `internal/ynab/sync_test.go` and `fakeYnabClient` in
  `internal/server/handlers_test.go` — both must gain `FetchTransactions` or
  `internal/ynab` and `internal/server` fail to compile. Constructor call-site
  counts (verified via grep, not estimated): `NewProcessor(` appears 17 times
  in `internal/txn/processor_test.go` and 3 times in
  `internal/server/handlers_test.go`; `NewSyncer(` appears 12 times in
  `internal/ynab/sync_test.go` and 3 times in
  `internal/server/handlers_test.go`. All of these need updating when the
  respective constructor signature changes.
- **UI**: preview stat cards are in
  `ui/html/partials/bank-transactions-preview.tmpl.html` (`.Preview.NewCount`
  / `.DuplicateCount` / `.ErrorCount`, each a conditionally-shown
  `stat-card`); the detail panel is
  `ui/html/partials/txn-detail-panel.tmpl.html`, which currently shows the
  bank description via a `.detail-field` block right after the header — the
  new match-note banner slots in right after that block.
- **Rejected alternatives** (do not revisit without new information from the
  user):
  - Live on-demand YNAB API call at import time instead of a cached table —
    contradicts the explicit ask for hourly background sync + local
    comparison, adds import latency, no reuse value.
  - Real incremental `server_knowledge`-based delta sync — the codebase's
    existing sync calls all hardcode `last_knowledge_of_server=0` (full
    refetch each cycle) despite storing `ServerKnowledge` in
    `sync_history.last_known_version`; building real delta-cursor handling
    here would be inconsistent with that pattern and adds cursor-reset/
    deleted-record-delta complexity for no clear win. Stay with full
    refetch, bounded by `since_date`.
  - Fuzzy payee/description matching as part of the match criteria —
    manually-typed YNAB payee names are often unrelated to bank CSV text;
    rejected as an unreliable signal not worth the complexity.
  - Exact-date matching (no window) — rejected; bank settlement dates can
    legitimately shift by a day or more from the actual purchase date (per
    user's own example).

## Development Approach

- **Testing approach**: Regular (code first, then tests) — matches this
  repo's established convention (see
  `docs/plans/completed/20260722-fix-duplicate-transactions.md`).
- Complete each task fully before moving to the next.
- Make small, focused changes.
- **Every task that changes behavior MUST include tests before moving on.**
- All tests must pass before starting the next task.

## Testing Strategy

- Table-driven Go unit tests throughout, matching the existing style in
  `internal/ynab/*_test.go`, `internal/sqlite/*_test.go`,
  `internal/txn/*_test.go`.
- No e2e/UI test framework exists in this project (HTMX + Go templates,
  no Playwright/Cypress) — UI template changes are covered by manual
  verification only (see Post-Completion).

## Implementation Steps

### Task 1: Add `BudgetTxn` model and `budget_transactions` migration

**Files:**
- Modify: `internal/ynab/ynab.go` (add `BudgetTxn` type)
- Create: `internal/sqlite/migrations/00006_budget_transactions.sql`

- [x] add `BudgetTxn` struct to `internal/ynab/ynab.go`: `ID string
      json:"id"`, `BudgetID string json:"-"` (not present in the API
      response — stamped by the syncer, see Task 4), `AccountID string
      json:"account_id"`, `Date string json:"date"` (`YYYY-MM-DD`), `Amount
      int json:"amount"` (milliunits), `PayeeName string json:"payee_name"`,
      `Memo string json:"memo"` — mirror the doc-comment style of the
      existing `Payee`/`Account` structs in that file. **No `Deleted`
      field/column** — see Task 4: the sync purges stale rows by
      delete-then-reinsert instead of tracking a deleted flag, since YNAB's
      `since_date`-filtered response never includes deleted transactions in
      the first place, so a soft-delete flag would just be permanently stale
      dead weight
- [x] write migration `00006_budget_transactions.sql` creating
      `budget_transactions` (id PK, budget_id FK → budgets, account_id, date,
      amount, payee_name, memo) + index
      `idx_budget_txns_account_date ON budget_transactions(account_id, date)`,
      with a matching `-- +goose Down` dropping index then table (follow the
      exact style of `00001_initial_schema.sql`)
- [x] run `go build ./...` to confirm the new type compiles
- [x] run tests — must pass before task 2 (no behavior yet, just schema/model)

### Task 2: Add `FetchTransactions` to the YNAB client, `YnabClient` interface, and `FakeClient`

**Files:**
- Modify: `internal/ynab/client.go`
- Modify: `internal/ynab/sync.go` (`YnabClient` interface, `SyncReq`)
- Modify: `internal/ynab/fake_client.go`
- Modify: `internal/ynab/fixture.go`
- Modify: `internal/ynab/fake_client_test.go`
- Modify: `internal/ynab/fixture_test.go`
- Modify: `internal/ynab/sync_test.go` (`mockYnabClient` — implements `YnabClient`)
- Modify: `internal/server/handlers_test.go` (`fakeYnabClient` — implements `YnabClient`)

- [x] add `SinceDate string` field to `SyncReq` in `internal/ynab/client.go`
- [x] add `TransactionResp`/`TransactionData` response types (`Transactions []BudgetTxn`, `ServerKnowledge int64`), mirroring `PayeeResp`/`PayeeData`
- [x] implement `Client.FetchTransactions(req SyncReq) (TransactionData, error)` calling `GET /budgets/{id}/transactions?since_date={req.SinceDate}`, matching the exact structure/logging/error-handling of `FetchPayees`
- [x] add `FetchTransactions(req SyncReq) (TransactionData, error)` to the `YnabClient` interface in `internal/ynab/sync.go`
- [x] add a `fetchTransactionsFunc` field + `FetchTransactions` method to `mockYnabClient` in `internal/ynab/sync_test.go`, mirroring its existing `fetchPayeesFunc`/`FetchPayees` — required for `internal/ynab` to compile once the interface changes
- [x] add a `FetchTransactions` method to `fakeYnabClient` in `internal/server/handlers_test.go` (returning `ynab.TransactionData{}, nil` is enough — mirror its existing trivial `FetchPayees` stub) — required for `internal/server` to compile once the interface changes
- [x] add `Fixture.Transactions map[string]TransactionData` (keyed by budget ID, mirroring `Fixture.Payees`) to `internal/ynab/fixture.go`; default to empty when absent from fixture JSON (no fixture data changes required — empty map is a valid fixture)
- [x] implement `FakeClient.FetchTransactions(req SyncReq) (TransactionData, error)` returning `c.fixture.Transactions[req.BudgetID]`, mirroring `FetchPayees`
- [x] note: `Client.FetchTransactions`'s real HTTP behavior is intentionally left untested here, consistent with the existing convention — `Client.FetchAccounts`/`FetchCategories`/`FetchPayees` have no direct unit tests either; only `Syncer` (via the mock `YnabClient`) and `FakeClient` are tested at this layer
- [x] write tests for `FakeClient.FetchTransactions` (known budget, unknown budget error, known budget with no fixture data), mirroring `FetchPayees`'s three existing test cases
- [x] update `fixture_test.go` if it asserts on the full `Fixture` struct shape
- [x] run tests — must pass before task 3

### Task 3: Add `budget_transactions` storage to `YnabStore`

**Files:**
- Modify: `internal/sqlite/ynab_store.go`
- Modify: `internal/sqlite/ynab_store_test.go`

- [x] add `UpsertBudgetTxn(ctx, t ynab.BudgetTxn) error` to `YnabStore`, mirroring `UpsertPayee`'s `INSERT OR REPLACE` + existence-check style
- [x] add `DeleteBudgetTxnsSince(ctx, budgetID string, sinceDate string) error` — `DELETE FROM budget_transactions WHERE budget_id = ? AND date >= ?` (used by Task 4's sync to purge stale/YNAB-deleted rows before reinserting the fresh fetch)
- [x] add `FindBudgetTxnMatch(ctx, accountID string, amountMilliunits int, date time.Time) (ynab.BudgetTxn, bool, error)`. Format `date` as `date.Format("2006-01-02")` **in the time.Time's own location** (do NOT convert to UTC first — `Transaction.TxnTime` is parsed in the bank's local timezone, e.g. `Europe/Warsaw`, in `internal/parser/*.go`; converting to UTC first can shift a late-evening local timestamp to the next/previous calendar day and silently narrow the window). Query `account_id = ? AND amount = ? AND date BETWEEN ? AND ?` (±3 days, both bounds formatted the same way), `ORDER BY ABS(julianday(date) - julianday(?)) LIMIT 1`; return `(BudgetTxn{}, false, nil)` on no rows (not an error — this is a normal "no match" outcome, unlike `FindBudgetByID`'s not-found-is-error style)
- [x] extend the hand-built test schema in `internal/sqlite/ynab_store_test.go` (it builds its own `CREATE TABLE` statements rather than running migrations) to include `budget_transactions` — otherwise every new test in this task fails with "no such table"
- [x] write tests for `UpsertBudgetTxn` (insert, then replace/update)
- [x] write tests for `DeleteBudgetTxnsSince` (rows on/after the date removed, rows before untouched, rows for other budgets untouched)
- [x] write tests for `FindBudgetTxnMatch`: exact-date hit, boundary hit (day 3 vs day 4 — day 4 must NOT match), multiple-candidates-picks-closest, no-match-returns-false-not-error, **a negative amount (outflow, e.g. `-123450`)** — YNAB milliunits are negative for outflows just like our parsed `Transaction.Amount`, and every other listed case implicitly uses a positive amount so a sign mistake wouldn't be caught otherwise, **a local-time boundary case** (e.g. a `time.Time` at 23:00 in a non-UTC location, to catch UTC-conversion bugs in the date formatting)
- [x] run tests — must pass before task 4

### Task 4: Add `Syncer.SyncTransactions` and wire into the scheduler

**Files:**
- Modify: `internal/ynab/sync.go` (`BudgetTxnStorer` interface, `Syncer` struct, `NewSyncer`, `SyncTransactions`)
- Modify: `internal/ynab/scheduler.go` (`BudgetSyncRunner` interface, `syncAll`)
- Modify: `internal/app/app.go` (`NewSyncer` call site)
- Modify: `internal/ynab/sync_test.go` (12 existing `NewSyncer(` call sites need the new arg)
- Modify: `internal/server/handlers_test.go` (3 existing `NewSyncer(` call sites need the new arg)
- Modify: `internal/ynab/scheduler_test.go`

- [x] add `BudgetTxnStorer` interface (`UpsertBudgetTxn`, `DeleteBudgetTxnsSince`) to `internal/ynab/sync.go`
- [x] add `budgetTxnStore BudgetTxnStorer` field + constructor param to `Syncer`/`NewSyncer` — before writing this, `grep -rn "NewSyncer("` across the repo (not just `sync_test.go`) to catch every call site, the same discovery step this repo's own `docs/plans/completed/20260721-account-parser-mapping.md` used for a similar constructor change; update `internal/app/app.go` to pass `ynabStore` again (same as the other five storer args) and every test call site in `internal/ynab/sync_test.go` and `internal/server/handlers_test.go`
- [x] implement `Syncer.SyncTransactions(ctx, budgetID string) error`: compute `sinceDate` = 6 months back from `time.Now()` (hardcoded constant, no new config flag), call `client.FetchTransactions(SyncReq{BudgetID: budgetID, SinceDate: sinceDate})`; **on each fetched transaction, set `t.BudgetID = budgetID` before upserting** — the API response has no `budget_id` field, and `budget_transactions.budget_id` is a FK to `budgets` with `PRAGMA foreign_keys=ON` active, so skipping this makes every upsert fail; call `budgetTxnStore.DeleteBudgetTxnsSince(ctx, budgetID, sinceDate)` **before** upserting the fresh batch, so transactions deleted in YNAB since the last sync are actually removed from the local cache instead of lingering forever (this replaces a soft-delete `deleted` flag — see Task 1); save `sync_history` on failure via the existing `saveFailedHistory` helper, save success history (name `"transactions"`, `AddedItems`, `LastKnownVersion: data.ServerKnowledge`) on success — mirror `SyncAccounts`'s overall structure
- [x] add `SyncTransactions(ctx, budgetID string) error` to the `BudgetSyncRunner` interface in `internal/ynab/scheduler.go` and call it in `syncAll`'s per-budget loop alongside the existing three calls, same fail-open (log-and-continue) error handling
- [x] write tests for `SyncTransactions` (success — **assert the upserted `BudgetTxn.BudgetID` matches the budget being synced**, fetch failure, partial upsert failure, delete-sweep called before upsert), mirroring `sync_test.go`'s existing `SyncAccounts` tests
- [x] write/update scheduler test confirming `SyncTransactions` is called per budget and a failure there doesn't block the other sync calls, mirroring existing `scheduler_test.go` assertions
- [x] run tests — must pass before task 5

### Task 5: Add `match_note` column to `transactions` and thread it through `TransactionStore`

**Files:**
- Create: `internal/sqlite/migrations/00007_transactions_match_note.sql`
- Modify: `internal/txn/txn.go` (`Transaction.MatchNote` field)
- Modify: `internal/sqlite/transaction_store.go`
- Modify: `internal/sqlite/transaction_store_test.go`

- [x] write migration `00007_transactions_match_note.sql`: `ALTER TABLE transactions ADD COLUMN match_note TEXT` (nullable, no default — matches the "add nullable, no backfill needed" guidance in the migrations README for optional new fields)
- [x] add `MatchNote string` field to `internal/txn/txn.go`'s `Transaction` struct, alongside `ErrorMsg`
- [x] thread `match_note` through `InsertTransaction`'s column list/args, `FetchTransactionsByAccount`'s SELECT/scan, and `FindTransactionByID`'s SELECT/scan — use `sql.NullString` for scanning (NULL → empty string) since existing rows predate the column
- [x] extend the hand-built test schema in `internal/sqlite/transaction_store_test.go` (it builds its own `CREATE TABLE transactions` rather than running migrations, per the earlier discovery) to include `match_note TEXT` — otherwise every test in this task fails with "no such column"
- [x] write/update `InsertTransaction` tests covering a row with `MatchNote` set and one without
- [x] write/update `FetchTransactionsByAccount`/`FindTransactionByID` tests confirming `MatchNote` round-trips, including a pre-existing-style row where the column is NULL
- [x] run tests — must pass before task 6

### Task 6: Add a shared match helper and use it in `Processor.Preview`

**Files:**
- Modify: `internal/txn/processor.go` (`BudgetTxnFinder` interface, `Processor`, `NewProcessor`, new `matchExistingYnabTxn` helper, `Preview`, `PreviewResult`)
- Modify: `internal/app/app.go` (`NewProcessor` call site)
- Modify: `internal/txn/processor_test.go` (17 existing `NewProcessor(` call sites need the new arg)
- Modify: `internal/server/handlers_test.go` (3 existing `NewProcessor(` call sites need the new arg)

- [x] add `BudgetTxnFinder` interface (`FindBudgetTxnMatch(ctx, accountID string, amountMilliunits int, date time.Time) (ynab.BudgetTxn, bool, error)`) to `internal/txn/processor.go`
- [x] add `budgetTxnFinder BudgetTxnFinder` field + constructor param to `Processor`/`NewProcessor` — before writing this, `grep -rn "NewProcessor("` across the repo to find every call site (not just the ones already known); update `internal/app/app.go` to pass `ynabStore`, and every test call site in `internal/txn/processor_test.go` and `internal/server/handlers_test.go` to pass `nil` where unused, matching the existing pattern for other unused deps in those tests
- [x] add one unexported helper method, `func (p *Processor) matchExistingYnabTxn(ctx context.Context, t Transaction) (ynab.BudgetTxn, bool)`, that: returns `(ynab.BudgetTxn{}, false)` immediately if `p.budgetTxnFinder == nil` (tolerates the `nil` passed in tests/until fully wired) or if `t.Status == TransactionInvalid` (invalid rows have zero `Amount`/`TxnTime` per the parsers — matching on them is meaningless and would double-count against `ErrorCount`); otherwise computes milliunits via the existing `int(math.Round(t.Amount * 1000))` conversion and calls `FindBudgetTxnMatch`; **on a store error, logs it (`slog.Warn`) and returns `(ynab.BudgetTxn{}, false)`** — a lookup failure must never turn into a blocked import (fail-open, consistent with the rest of this plan). Both `Preview` and `Process` call this one helper — no duplicated matching logic between them.
- [x] in `Preview`, after the existing hash-ID duplicate check and only when NOT already a hash-duplicate: call `matchExistingYnabTxn`; on a hit, **set the status on the slice element that is actually appended to the sample** — i.e. `txns[i].Status = "ALREADY_IN_YNAB"` followed by appending `txns[i]` (not the stale `range`-loop copy `txn`) to `result.Transactions`. This also fixes the pre-existing bug where `txns[i].Status = "DUPLICATE"` is set but the stale copy is what actually gets appended, so the "Duplicate" badge never renders today — fix that append for the `"DUPLICATE"` branch at the same time, in the same loop
- [x] add `AlreadyInYnabCount int` to `PreviewResult`, increment it on match, and subtract it from the `NewCount` calculation (`NewCount = TotalCount - DuplicateCount - AlreadyInYnabCount - ErrorCount`)
- [x] add `ImportableCount int` to `PreviewResult`, set to `NewCount + AlreadyInYnabCount`. **Why this is needed**: `bank-transactions-preview.tmpl.html` currently gates the Confirm button and the "nothing to import" empty-state entirely on `NewCount > 0`; a CSV where every row matches an existing YNAB transaction would otherwise have `NewCount == 0`, hide Confirm, and silently skip calling `Process` at all — meaning the matched rows never even get recorded as `SKIPPED`, breaking the "still auditable/recoverable" guarantee from the Overview. Task 8 switches the template's gates to `ImportableCount` instead of `NewCount` (the stat card keeps showing `NewCount` on its own, unchanged)
- [x] write tests for `Preview`: a CSV row with a `budget_transactions` match → `AlreadyInYnabCount` incremented, `result.Transactions[i].Status == "ALREADY_IN_YNAB"` (asserting the fix actually reaches the sample, not just the count), `NewCount` correctly reduced; a row with no match unaffected; **a negative-amount (outflow) row that matches**, to catch a sign bug; **a mix of one invalid row + one matched row + one new row**, asserting `ErrorCount`/`AlreadyInYnabCount`/`NewCount` are each correct and don't double-count the invalid row; `budgetTxnFinder == nil` behaves exactly like today (no crash, nothing flagged)
- [x] run tests — must pass before task 7

  (Note: `"ALREADY_IN_YNAB"` is a transient preview-only string, never a real
  `TransactionStatus` value and never persisted — confirmed it never reaches
  `TransactionStatus.DisplayName()`, which is only called from
  `txn-rows.tmpl.html` against persisted rows; a matched row is persisted
  with the real `TransactionSkipped` status in Task 7, which already has a
  correct `DisplayName()` mapping to "Skipped". No `DisplayName` change
  needed.)

### Task 7: Auto-skip matched rows in `Processor.Process`

**Files:**
- Modify: `internal/txn/processor.go` (`Process`)
- Modify: `internal/txn/processor_test.go`

- [x] in `Process`, for each parsed transaction, call the same `matchExistingYnabTxn` helper added in Task 6 (do not re-implement the match logic here); on a hit, before calling `InsertTransaction`, set `t.Status = TransactionSkipped` and `t.MatchNote` to a message including the matched YNAB transaction's date (e.g. `"possible match: existing YNAB txn on 2026-06-11, matched account/amount"`)
- [x] leave non-matched rows' status untouched (existing behavior — parser-assigned status, typically `DRAFT`)
- [x] write tests for `Process`: a matched row is inserted with `Status = SKIPPED` and a non-empty `MatchNote`; a non-matched row is inserted with its original status and empty `MatchNote`; `budgetTxnFinder == nil` behaves exactly like today
- [x] run tests — must pass before task 8

### Task 8: Surface the match in the UI

**Files:**
- Modify: `ui/html/partials/bank-transactions-preview.tmpl.html`
- Modify: `ui/html/partials/txn-detail-panel.tmpl.html`

- [x] add a 4th conditional stat card (`{{if gt .Preview.AlreadyInYnabCount 0}}`) labeled "Already in YNAB", styled consistently with the existing `stat-duplicate`/`stat-error` cards (reuse or add a `stat-already-in-ynab` CSS class in `ui/static/css/main.css` if the existing `stat-duplicate` styling isn't visually distinct enough — check current CSS before adding new rules)
- [x] update the preview table row's status badge logic to show a distinct badge (e.g. "Already in YNAB") when `.Status == "ALREADY_IN_YNAB"`, alongside the existing `ErrorMsg`/`"DUPLICATE"` branches
- [x] change the Confirm-button gate (currently `{{if gt .Preview.NewCount 0}}`) and the empty-state gate (currently `{{if eq .Preview.NewCount 0}}`) in `bank-transactions-preview.tmpl.html` to use `.Preview.ImportableCount` instead of `.Preview.NewCount` (see Task 6's `ImportableCount` note) — this keeps Confirm available, and `Process` actually running, in the all-matched case, so matched rows still get recorded as `SKIPPED`
- [x] update the empty-state copy ("All transactions in this file have already been imported.") to also cover the all-matched case, e.g. "All transactions in this file are either already imported or already exist in YNAB."
- [x] in `txn-detail-panel.tmpl.html`, add a `.detail-field`-style block right after the existing "Description (from bank)" block, shown only `{{if .Txn.MatchNote}}`, displaying the note text so a user opening a `SKIPPED` transaction can see why it was auto-skipped
- [x] manual test (skipped - not automatable): manually verify in `make run-mock` (or against a real budget) that a CSV row overlapping a manually-entered YNAB transaction shows the new stat/badge/note correctly, and that an all-matched CSV still shows a working Confirm button (see Post-Completion for the full manual scenario)
- [x] run `make check` (this task has no Go unit tests of its own — templates only, per the Testing Strategy's no-e2e-framework note — but the full build/vet/lint/test suite must still stay green)

### Task 9: Verify acceptance criteria

- [x] verify: hourly scheduler now also syncs `budget_transactions` per budget, without blocking accounts/categories/payees sync on failure — confirmed `internal/ynab/scheduler.go`'s `BudgetSyncRunner` interface includes `SyncTransactions`, `syncAll` calls it per budget with the same fail-open (log-and-continue) handling as the other three calls, and `scheduler_test.go` covers this
- [x] verify: a CSV row matching an account+amount+±3-day-window YNAB transaction is flagged in preview and auto-skipped (not auto-sent) on import — confirmed `Processor.Preview` sets `"ALREADY_IN_YNAB"` and `Processor.Process` sets `TransactionSkipped` via the shared `matchExistingYnabTxn` helper, both covered by tests in `processor_test.go`
- [x] verify: a false-positive match remains recoverable — opening the `SKIPPED` transaction in the UI and clicking "Accept & Send to YNAB" still works — matched rows are persisted with the ordinary `TransactionSkipped` status via the existing generic transaction record; no new restriction was added anywhere gating which statuses can be sent to YNAB, so the pre-existing send/accept handler in `internal/server/handlers.go` applies unchanged
- [x] verify: with no sync data yet (fresh DB, `budget_transactions` empty), import behaves exactly as before (fail-open — nothing is ever incorrectly blocked) — `FindBudgetTxnMatch` returns `(BudgetTxn{}, false, nil)` on no rows (not an error), and `matchExistingYnabTxn` also returns `(BudgetTxn{}, false)` when `budgetTxnFinder == nil`, both exercised by existing tests
- [x] run `make check` (runs `go fmt`, `go vet`, `golangci-lint`, and `make test` with `-race` and coverage, per `Makefile`) — this is the project's required pre-commit gate per `CONTRIBUTING.md`, not just `go test ./...` — ran clean, no failures
- [x] review the coverage output `make test` prints as part of `make check` — no separate coverage step needed — all new feature code is well covered: `FindBudgetTxnMatch` 98.3%, `UpsertBudgetTxn` 90.9%, `DeleteBudgetTxnsSince` 90.0%, `SyncTransactions` 87.0%, `Preview` 90.2%, `Process` 93.3%, `matchExistingYnabTxn` 75.0%

### Task 10: Update documentation

- [x] add a short section to `FEATURES.md` documenting the "Already in YNAB" detection (mirrors the existing "Suggestion Engine" section's level of detail)
- [x] no change needed to `internal/sqlite/migrations/README.md` — the `00006`/`00007` split is a plain atomic-migrations case already covered by its existing examples
- [x] move this plan to `docs/plans/completed/20260926-exclude-manual-ynab-transactions.md`

## Technical Details

**Matching rule** (Part 2 core logic):
```
match(csvTxn) :=
    exists row in budget_transactions where
        row.account_id   = csvTxn.account_id
    AND row.amount        = round(csvTxn.amount * 1000)   -- milliunits
    AND row.date BETWEEN (csvTxn.date - 3 days) AND (csvTxn.date + 3 days)
    ORDER BY ABS(row.date - csvTxn.date) ASC
    LIMIT 1
```

**Accepted limitations** (explicitly not handled, to keep scope minimal — revisit only if real usage shows a problem):
- **Currency is not part of the match key.** YNAB accounts are single-currency in practice, so this hasn't been an issue for the existing `Upload` path either; not worth the extra join/column for this feature.
- **One cached YNAB transaction can match multiple CSV rows** in the same import (e.g. four identical same-amount charges within the ±3-day window would all match the same single manual entry, potentially over-skipping genuinely new ones). Accepted for v1 given the ±3-day window is already narrow and same-amount-same-week collisions are uncommon; if this surfaces in practice, the fix is to track already-matched `budget_transactions.id`s within a single `Preview`/`Process` call and skip at most one CSV row per cached transaction.

**Data flow**:
1. Scheduler (hourly + on startup) → `Syncer.SyncTransactions` per budget →
   `Client.FetchTransactions(since_date = now-6mo)` → stamp `BudgetID` on
   each result → delete existing `budget_transactions` rows for that budget
   with `date >= since_date` → upsert the fresh batch (delete-then-reinsert
   keeps the cache free of transactions deleted in YNAB, without a
   soft-delete flag).
2. CSV upload → `Processor.Preview` parses rows → for each row, existing
   hash-ID check first, then (if not already flagged) `FindBudgetTxnMatch` →
   `PreviewResult.AlreadyInYnabCount` shown to user before confirming import.
3. On confirm → `Processor.Process` re-runs the same match → matched rows
   saved as `SKIPPED` with `MatchNote`; everything else saved as before.

**New/changed schema**:
- `budget_transactions(id, budget_id, account_id, date, amount, payee_name, memo)` + `idx_budget_txns_account_date`
- `transactions.match_note TEXT` (nullable, new column)

## Post-Completion

**Manual verification**:
- Add a transaction with a known date/amount/account to
  `internal/ynab/fixtures/default.json`'s new `transactions` key (or pass a
  custom fixture via `--mock-data-file`/`MOCK_DATA_FILE` if you don't want to
  touch the embedded default), run `make run-mock`, then upload a CSV
  containing a matching row within the ±3-day window and confirm it's
  flagged and auto-skipped. (Against a real budget: manually add a
  transaction in the YNAB app, wait for the hourly sync or restart the app
  to trigger the on-startup sync, then upload a matching CSV.)
- Verify a genuinely new transaction (no YNAB-side match) is unaffected —
  regression check against existing import flow.
- Verify the 6-month `since_date` bound doesn't exclude a legitimately older
  manual entry a user might want caught — flag to the user if this surfaces
  in practice; the window is a hardcoded constant, trivial to widen.

**External systems**: none — this is entirely local to the app's own DB and
its existing YNAB API integration; no consuming projects or deployment
config changes required.
