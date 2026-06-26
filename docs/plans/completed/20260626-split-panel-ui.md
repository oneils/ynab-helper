# Split-Panel Transaction Review UI

## Overview

Replace the current full-page transaction edit form with a split-panel layout for the Transactions page:
list on the left (with status tabs and auto-filled payee/category names), detail panel on the right
(payee/category dropdowns pre-filled from YNAB payee history, memo, action buttons). This matches the
NAB Helper reference design and makes reviewing 100+ transactions fast without page-per-transaction navigation.

**No confidence scores.** Suggestions come from YNAB's `last_used_category_id` on payees — it's a
binary match (known payee → pre-fill, unknown → empty dropdown). No percentage scoring needed for
the bridge period.

No Pico CSS to remove — the project already uses a fully custom design system (`main.css`).
The CSS already defines `--sidebar-right-width: 288px` and a `.sidebar-right` media query stub.

## Context (from discovery)

- **Files involved:**
  - `ui/html/index.tmpl.html` — base layout
  - `ui/html/pages/import-txns.tmpl.html` — Transactions page (filter dropdowns → table)
  - `ui/html/pages/home.tmpl.html` — Import page (step wizard + upload form + table)
  - `ui/html/partials/bank-transactions.tmpl.html` — transaction table partial
  - `ui/html/partials/bank-transactions-preview.tmpl.html` — preview partial (upload → confirm two-step)
  - `ui/html/partials/bank-txn.tmpl.html` — full-page single-txn edit form (to be removed)
  - `ui/html/partials/transaction-row.tmpl.html` — inline edit row
  - `ui/static/css/main.css` — design system (2093 lines, no Pico)
  - `internal/server/server.go` — chi routes
  - `internal/server/handlers.go` — all handlers
  - `internal/txn/txn.go` — TransactionStatus type (DRAFT/SKIPPED/PROCESSED/INVALID)
  - `internal/txn/processor.go` — preview/confirm logic; uses INVALID for parse errors, "DUPLICATE" string for dedup
  - `internal/sqlite/transaction_store.go` — `FetchTransactionsByAccount`, `INSERT OR IGNORE` dedup

- **Existing routes relevant to this plan:**
  - `GET /bank-txns` → `bankTxnsHandler` — list transactions
  - `GET /bank-txns/{id}` → `fetchBankTxnHandler` — single txn edit page (to be removed)
  - `POST /bank-txns/{id}/skip` → skip
  - `GET /bank-txns/{id}/edit-inline` / `view-inline` / `save-inline` — inline row editing (to be removed)
  - `GET /bank-txns/api/payee-suggestions` / `category-suggestions` — suggestion APIs
  - `POST /ynab-add-txn` → POST to YNAB (marks PROCESSED)
  - `POST /preview-bank-txns` → parse CSV, show preview (does NOT save)
  - `POST /confirm-bank-txns` → save transactions to DB

- **Critical facts from code review:**
  - `txn.Transaction` has `Payee string` (raw parser text) but **no Category or Confidence fields** — suggestion engine is called per-transaction in `fetchBankTxnHandler`, never for the list. List rows cannot show category/confidence without a new enrichment step.
  - Upload is a two-step flow: `preview-bank-txns` → renders preview partial with "Confirm" button → `confirm-bank-txns` → saves to DB. The split-panel navigation must happen after `confirm-bank-txns`, not after `preview-bank-txns`.
  - `TransactionInvalid` ("INVALID") already means **parse errors** (set by `processor.go`). It must NOT be relabeled "Duplicate" — those are different things.
  - `GET /bank-txns/{id}` + `fetchBankTxnHandler` renders `bank-txn.tmpl.html` — this route must be removed together with the template.

## Development Approach

- **Testing approach:** Regular (code first, then tests for Go changes)
- CSS/template-only tasks: no automated tests needed; verify visually
- Go handler/store changes: write unit tests, run `go test ./...` before next task
- Run `make test` or `go test ./...` after each Go change

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document blockers with ⚠️ prefix

## What Goes Where

- **Implementation Steps** — code changes in this repo
- **Post-Completion** — manual browser verification

---

## Implementation Steps

### Task 1: Status model — rename display labels (without breaking INVALID)

Map existing `TransactionStatus` constants to user-facing display names. Keep `INVALID` meaning
"parse error" (do NOT rename it "Duplicate"). A separate user-triggered "mark as duplicate" flow
will use `SKIPPED` with a `skip_reason` column, or simply reuse SKIPPED — see Task 5.

Current DB values → display:
- `DRAFT` → "Needs Review"
- `PROCESSED` → "Accepted"
- `SKIPPED` → "Skipped"
- `INVALID` → "Invalid" (parse error, keep its real meaning)

**Files:**
- Modify: `internal/txn/txn.go`
- Modify: `ui/html/partials/bank-transactions.tmpl.html` (status badge labels)
- Modify: `ui/html/pages/import-txns.tmpl.html` (filter dropdown options)

- [x] add `DisplayName()` method on `TransactionStatus` returning the user-facing label
- [x] update status badge rendering in templates to use `DisplayName()`
- [x] update filter dropdown to show display names (keep DB values as option values)
- [x] write test for `DisplayName()` covering all four statuses
- [x] run `go test ./...` — must pass before Task 2

---

### Task 2: Split-panel layout — CSS

Add two-column split panel for the Transactions page. The CSS already has `--sidebar-right-width: 288px`
and a `.sidebar-right` responsive stub.

**Files:**
- Modify: `ui/static/css/main.css`

- [x] add `.txn-split-layout` flex container: `display: flex; height: calc(100vh - 7rem); overflow: hidden`
- [x] add `.txn-list-panel` (left): `flex: 1; overflow-y: auto; border-right: 1px solid var(--bg-border)`
- [x] add `.txn-detail-panel` (right): `width: var(--sidebar-right-width); min-width: 320px; overflow-y: auto; display: none`
  - shown via `.txn-detail-panel--open { display: block }`
- [x] add status tab bar: `.status-tabs`, `.status-tab`, `.status-tab--active` (with `.status-tab-count`)
- [x] add `.auto-filled-badge` — a small neutral pill ("auto") shown when payee/category was pre-filled from YNAB
- [x] update `@media (max-width: 1280px)`: hide `.txn-detail-panel` (overlay later if needed, not in scope now)
- [x] verify all new classes use CSS vars only (no hardcoded hex)

---

### Task 3: Add `CountByStatus` to transaction store

The status tabs need per-status counts. Add a store method that returns counts for a given account.

**Files:**
- Modify: `internal/sqlite/transaction_store.go`
- Modify: `internal/sqlite/transaction_store_test.go` (or create if absent)

- [x] add `CountByStatus(ctx, accountID string) (map[TransactionStatus]int, error)` to the store interface and SQLite implementation
  - SQL: `SELECT status, COUNT(*) FROM transactions WHERE account_id = ? GROUP BY status`
- [x] write table-driven tests for `CountByStatus` (empty account, mixed statuses)
- [x] run `go test ./...` — must pass before Task 4

---

### Task 4: Enrich list rows with auto-filled payee/category

`bankTxnsHandler` returns raw `txn.Transaction` rows with no category. Add a view-model struct
that carries the suggested payee name and category name (from YNAB payee lookup), with a boolean
flag indicating whether a match was found — used to show/hide the `.auto-filled-badge`.

**Files:**
- Create or Modify: `internal/server/handlers.go` (or dedicated `bank_txns_handler.go`)
- Modify: `ui/html/partials/bank-transactions.tmpl.html` (consume new view-model fields)

- [x] define `TxnListRow` view-model:
  ```go
  type TxnListRow struct {
      Txn           txn.Transaction
      SugPayee      string // empty if no match
      SugCategory   string // empty if no match
      AutoFilled    bool   // true if either field was pre-filled
  }
  ```
- [x] in `bankTxnsHandler`: after fetching transactions, look up each row's top payee + category
  - use `GetSuggestions(ctx, budgetID, txn.Payee, 1)` for payee; `GetCategorySuggestions(ctx, budgetID, txn.Payee, "", 1)` for category
  - derive `budgetID` from account via existing `FindBudgetByAccID`
  - set `AutoFilled = true` if either suggestion returned a result
  - no confidence scores stored or used
- [x] update `bank-transactions.tmpl.html` to render `SugPayee` and `SugCategory` as plain text; show `.auto-filled-badge` if `AutoFilled`
- [x] write test for the enrichment logic (mock suggestion engine, verify view-model fields, verify AutoFilled flag)
- [x] run `go test ./...` — must pass before Task 5

---

### Task 5: Status tabs partial + update Transactions page structure

Replace filter dropdowns with status tabs. Tabs include per-status counts and filter the list via htmx.
Status tabs must carry the active budget/account context so filtering stays per-account.

**Files:**
- Create: `ui/html/partials/status-tabs.tmpl.html`
- Modify: `ui/html/pages/import-txns.tmpl.html`
- Modify: `internal/server/handlers.go` (add counts to view data)
- Modify: `internal/txn/processor.go` (add CountByStatus method)

- [x] create `{{define "status-tabs"}}` partial:
  - tabs: All / Needs Review / Accepted / Skipped / Invalid
  - each tab: `hx-get="/bank-txns?budget={{.Budget}}&account={{.Account}}&status=<VALUE>" hx-target="#txn-list-panel" hx-swap="innerHTML"`
  - active tab gets `.status-tab--active`; pass `ActiveStatus` from handler
  - show count next to label using `StatusCounts` map from handler
- [x] add `StatusCounts map[string]int` and `ActiveStatus string` to the handler's view-data struct
- [x] call `CountByStatus` in `bankTxnsHandler` and populate view-data
- [x] update `import-txns.tmpl.html`:
  - remove `.filter-section` block
  - wrap in `.txn-split-layout`
  - left: `{{template "status-tabs" .}}` + `<div id="txn-list-panel">{{template "bank-transactions" .}}</div>`
  - right: `<div id="txn-detail-panel" class="txn-detail-panel"></div>`
- [x] run `go test ./...` — must pass before Task 6

---

### Task 6: Transaction list row redesign

Redesign table rows: date, description (truncated, full in `title`), amount, payee (+ "auto" badge if pre-filled),
category (+ "auto" badge if pre-filled), status badge. Clicking a row loads the detail panel via htmx.

**Files:**
- Modify: `ui/html/partials/bank-transactions.tmpl.html`

- [x] remove columns: `#`, Currency, Account, Memo (move to detail panel)
- [x] add columns: Date, Description, Amount, Payee (+ "auto" badge if pre-filled), Category (+ "auto" badge if pre-filled), Status
- [x] each row: `hx-get="/bank-txns/{{$row.Txn.ID}}/detail?account={{$row.Txn.Account.ID}}" hx-target="#txn-detail-panel" hx-swap="innerHTML" hx-on::after-request="document.getElementById('txn-detail-panel').classList.add('txn-detail-panel--open')"`
- [x] add `.txn-row--selected` CSS class; clicking a row adds it, removes from others (small inline JS or htmx class-tools)
- [x] keep Skip button inline as quick action (no panel needed for skip)
- [x] remove Edit button and bulk-actions toolbar
- [x] no new Go changes in this task (uses enriched view-model from Task 4)

---

### Task 7: Transaction detail panel + new routes

Create the right-panel partial. Add `GET /bank-txns/{id}/detail` route.
Remove the now-dead `GET /bank-txns/{id}` route and `fetchBankTxnHandler`.

Note on "Mark as Duplicate": since `INVALID` means parse-error, a user-triggered "duplicate" action
will simply **skip** the transaction (POST to existing `/bank-txns/{id}/skip`). The Skip tab covers it.
No new status needed — keeps the model clean.

**Files:**
- Create: `ui/html/partials/txn-detail-panel.tmpl.html`
- Modify: `internal/server/server.go` (add `GET /bank-txns/{id}/detail`; remove `GET /bank-txns/{id}` + inline routes)
- Modify: `internal/server/handlers.go` (add `detailBankTxnHandler`; remove `fetchBankTxnHandler`, inline handlers)

- [x] create `{{define "txn-detail-panel"}}` partial:
  - header: amount large (red outflow / green inflow), date
  - "Description (from bank)": raw `txn.Description`
  - Payee: `<select>` pre-filled with top suggestion; small "(suggested)" label if auto-filled, nothing if manual
  - Category: same pattern
  - Memo: `<textarea name="memo">`
  - Actions:
    - "Accept & Send to YNAB" → `hx-post="/ynab-add-txn"` (marks PROCESSED; on success reload list row + close panel or advance)
    - "Save" → `hx-post="/bank-txns/{id}/save-inline"` (saves edits, stays DRAFT)
    - "Skip" → `hx-post="/bank-txns/{id}/skip"` (marks SKIPPED, closes panel)
  - ~~Previous/Next navigation~~ — **deferred** (YAGNI; list is visible on left, no nav needed)
- [x] add `GET /bank-txns/{id}/detail` → `detailBankTxnHandler`:
  - fetch txn by ID
  - derive budgetID from `txn.Account.ID` via `FindBudgetByAccID`
  - fetch top payee suggestion (1) + top category suggestion (1); store as name strings only, no scores
  - render `txn-detail-panel` partial
- [x] remove `GET /bank-txns/{id}` route + `fetchBankTxnHandler` from server.go/handlers.go
- [x] remove `GET /bank-txns/{id}/edit-inline`, `view-inline`, `save-inline` routes IF fully superseded
  - kept `save-inline` POST — detail panel's "Save" button reuses it
- [x] write tests for `detailBankTxnHandler` (txn not found, suggestions resolved, correct template data)
- [x] run `go test ./...` — must pass before Task 8

---

### Task 8: Home page — simplify upload form + fix post-confirm navigation

Remove the step wizard. After `confirm-bank-txns` succeeds, navigate to `/import-bank-txns`
so the split-panel shows the newly imported transactions.

**Files:**
- Modify: `ui/html/pages/home.tmpl.html`
- Modify: `ui/html/partials/bank-transactions-preview.tmpl.html` (confirm button target)
- Modify: `internal/server/handlers.go` (`confirmBankTxnsHandler` — add redirect or `HX-Redirect` header)
- Modify: `ui/static/css/main.css` (remove unused `.form-steps` styles)

- [x] remove `<div class="form-steps">` block from `home.tmpl.html`
- [x] add bank parser selector: `<select name="parser">` with options Revolut / Santander / PKO BP
  - parser is auto-detected from account name in processor.go — no manual selector needed
- [x] in `confirmBankTxnsHandler`: after successful DB insert, return htmx `HX-Redirect: /import-bank-txns` header
  - this sends the user to the split-panel Transactions page where they can review newly imported txns
- [x] remove `{{template "bank-transactions" .}}` from `home.tmpl.html` (transactions now live on `/import-bank-txns`)
- [x] remove unused `.form-steps`, `.step-item`, `.step-number`, `.step-label`, `.step-divider` CSS
- [x] run `go test ./...` — must pass before Task 9

---

### Task 9: Verify acceptance criteria

- [x] upload a bank CSV → preview shown → confirm → redirect to `/import-bank-txns` split panel (manual test — not automatable)
- [x] imported transactions appear in "Needs Review" tab; rows with auto-filled payee/category show "auto" badge (manual test — not automatable)
- [x] status tab counts are correct (All / Needs Review / Accepted / Skipped / Invalid) (manual test — not automatable)
- [x] click a row → detail panel opens; payee/category dropdowns pre-filled if YNAB match found (manual test — not automatable)
- [x] "Accept & Send to YNAB" → transaction moves to Accepted, list row updates (manual test — not automatable)
- [x] "Save" → edits persisted, transaction stays Needs Review (manual test — not automatable)
- [x] "Skip" → transaction moves to Skipped tab, panel closes (manual test — not automatable)
- [x] inline "Skip" button on list row still works (manual test — not automatable)
- [x] verify dark theme applies correctly throughout new panel (manual test — not automatable)
- [x] run `go test ./...` — all tests pass

---

### Task 10: [Final] Cleanup

- [x] remove `bank-txn.tmpl.html` (replaced by `txn-detail-panel.tmpl.html`)
- [x] remove `transaction-row.tmpl.html` if no longer referenced
- [x] verify no orphaned CSS classes remain (search templates for class usage)
- [x] update `FEATURES.md`
- [x] move this plan to `docs/plans/completed/`

---

## Technical Details

**Status mapping (DB → display):**
| DB value  | Display      | Tab label    |
|-----------|--------------|--------------|
| DRAFT     | Needs Review | Needs Review |
| PROCESSED | Accepted     | Accepted     |
| SKIPPED   | Skipped      | Skipped      |
| INVALID   | Invalid      | Invalid      |

**"Mark as Duplicate" decision:** Not a separate status. User skips it → it's in the Skipped tab.
The `INVALID` status retains its existing meaning (parse error), avoiding a semantic collision.

**Previous/Next navigation:** Deferred. The list is always visible on the left — the user can just
click another row. No server-side session or query-param threading needed.

**Detail handler account context:**
```
GET /bank-txns/{id}/detail?account={accountID}
  → fetch txn by ID
  → FindBudgetByAccID(txn.Account.ID)  ← same pattern as fetchBankTxnHandler
  → GetSuggestions(ctx, budgetID, txn.Payee, 3)
  → GetCategorySuggestions(ctx, budgetID, txn.Payee, "", 3)
  → render txn-detail-panel
```

**Status tabs htmx (always include budget + account):**
```html
hx-get="/bank-txns?budget={{.Budget}}&account={{.Account}}&status=DRAFT"
hx-target="#txn-list-panel"
hx-swap="innerHTML"
```

**Suggestion display:** Binary — either a YNAB payee match exists (pre-fill + "auto" badge) or it doesn't
(empty dropdown, user fills manually). No percentage scores at any layer.

---

## Post-Completion

**Manual browser verification:**
- Test on 1440px, 1280px, 768px — detail panel hides correctly on narrow viewports
- Test with 100+ transactions for scroll performance in the list panel
- Verify YNAB sync still marks transactions PROCESSED correctly after the flow change
