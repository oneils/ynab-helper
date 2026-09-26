# Split-Panel Transaction Review

## Overview

The Transactions page (`/import-bank-txns`) uses a split-panel layout for fast transaction review:
- **Left panel**: Filterable list with status tabs (Needs Review, Accepted, Skipped, Invalid)
- **Right panel**: Detail panel with payee/category selects pre-filled from YNAB history

After importing a CSV via the Home page, you are redirected to this split-panel view where
you can process 100+ transactions without page-per-transaction navigation.

## How It Works

### Import Flow

1. Upload a bank CSV on the Home page (`/`)
2. Preview the parsed transactions
3. Click "Confirm" — all transactions are saved and you are redirected to `/import-bank-txns`

### Split-Panel Review

- **Filter card** (sticky at top): unified budget/account selector and status tabs — All / Needs Review / Accepted / Skipped / Invalid with per-status counts
- **List rows** display: Date, Description (truncated), Amount, Payee, Category, Status; loaded via **infinite scroll** (HTMX v4 sentinel row, `GET /bank-txns/rows`)
- Rows with YNAB payee matches show an "auto" badge
- Click a row to open the **detail panel**, which is sticky and dynamically positioned below the filter card so they never overlap

### Detail Panel

- Payee `<select>`: pre-filled if YNAB found a matching payee via `last_used_category_id`
- Category `<select>`: narrows automatically when payee changes; pre-filled from YNAB data
- **Remember toggle**: opt-in form field included in the Accept & Send to YNAB request; when checked, a payee/category pattern is recorded only after a successful send; disabled (and force-unchecked) until both Payee and Category are filled
- Actions:
  - **Accept & Send to YNAB** — posts the transaction to YNAB and marks it Accepted
  - **Save** — persists edits, keeps status as Needs Review
  - **Skip** — marks the transaction as Skipped

### Status Mapping

| DB Value  | Display      |
|-----------|--------------|
| DRAFT     | Needs Review |
| PROCESSED | Accepted     |
| SKIPPED   | Skipped      |
| INVALID   | Invalid      |

### Sorting

- List defaults to **newest-first** (`ORDER BY txn_time DESC, id DESC` — the
  `id` tiebreak keeps same-day transactions in a stable order across
  infinite-scroll pages, since `txn_time` is date-only).
- A **sort toggle** button (top of `bank-transactions.tmpl.html`) flips
  between newest-first and oldest-first. Its label always describes the
  action a click will perform.
- The chosen order is carried via a `sort` query/form param and a hidden
  `#sort-state` input; other controls outside `#txn-list-panel` (status tabs,
  account select) use `hx-include="#sort-state"` to stay in sync, since their
  own `hx-get` URLs are never re-rendered by a list response.
- The choice is persisted per-browser via `localStorage`
  (`ynab_txn_sort`) — the only client-side persistence in this app, chosen
  because sort is a standing display preference rather than a per-session
  filter selection like budget/account/status.

## Architecture

- **Templates**: `bank-transactions.tmpl.html` (list, includes the sort
  toggle), `txn-detail-panel.tmpl.html` (detail), `status-tabs.tmpl.html`
  (tabs)
- **CSS**: Split-panel layout in `main.css` (`.txn-split-layout`, `.txn-list-panel`, `.txn-detail-panel`)
- **HTMX (v4.0.0, CDN-only, no npm/bundler)**: Row clicks load detail panel; status tabs filter via `hx-get`; action buttons update via `hx-post`
- **Routes**: `GET /import-bank-txns` (page), `GET /bank-txns` (filtered list), `GET /bank-txns/rows` (infinite scroll batches), `GET /bank-txns/{id}/detail` (panel)
- **HTMX v1→v4 event mapping** (migrated 2026-08-31; see `docs/plans/completed/20260831-htmx4-migration.md` for full audit): htmx 4 restructured all `htmx:*` events to a `htmx:phase:action[:sub-action]` pattern and moved requests from `XMLHttpRequest` to `fetch()`. Names in use in this codebase:
  - `htmx:beforeRequest` → `htmx:before:request`
  - `htmx:afterRequest` → `htmx:after:request`
  - `htmx:beforeSwap` → `htmx:before:swap`
  - `htmx:afterSwap` → `htmx:after:swap`
  - `htmx:afterSettle` → `htmx:after:settle`
  - `hx-on::after-request` → `hx-on::after:request`
  - `htmx:xhr:progress` → removed, no replacement (fetch exposes no upload-progress signal); the upload progress bar was downgraded to an indeterminate indicator in `ui/static/js/main.js`
  - Response headers (`HX-Trigger`, `HX-Refresh`, `HX-Redirect`, `HX-Reswap`, set in `internal/server/handlers.go`) are **unchanged** in v4 — no renaming happened server-side
  - `evt.preventDefault()`-based request cancellation is unchanged in v4, but the `detail` payload shape moved: `after:swap` exposes the target at `detail.ctx.target`, `after:settle` exposes it at `detail.task.target` (there is no top-level `detail.target` in v4)
- **Pattern to reuse**: a toggle control whose own markup (label/target URL)
  must change after each click belongs *inside* the swap target it affects
  (here, inside `bank-transactions.tmpl.html`), not beside it — it then
  re-renders for free on every response instead of needing an out-of-band
  swap. Fixed controls (like the status tabs) don't need this.

## Suggestion Engine

Two-stage fallback with confidence-tiered gating:

1. **Learned patterns** — match bank transaction description against stored patterns (exact fingerprint match, falling back to a token-broadened search), ranked by description similarity and aggregated evidence across matching patterns
2. **YNAB payee name fallback** — if no pattern match, fuzzy-match the description against YNAB payee names; on hit, pre-fill payee + `last_used_category_id` category and show the "auto" badge

Each suggestion's confidence score is classified into a tier: below `SuggestThreshold` it's hidden entirely, at or above `SuggestThreshold` it's shown as a suggestion, and at or above `PrefillThreshold` it auto-fills the form field.

Unknown transactions (no match in either stage) → empty dropdowns.

## Already-in-YNAB Detection

Catches transactions the user typed directly into YNAB by hand, which would
otherwise reappear as "new" in a later CSV import and risk being pushed to
YNAB a second time.

1. **Background sync** — on the same hourly (+ on-startup) schedule as
   accounts/categories/payees, `Syncer.SyncTransactions` fetches each
   budget's real YNAB transactions (`since_date` = 6 months back) into a
   local `budget_transactions` cache table, keyed by budget/account/date/
   amount. Stale rows are purged (delete-then-reinsert) so YNAB-side
   deletions are reflected locally.
2. **Field-based match at import time** — for each CSV row not already
   caught by the existing hash-ID duplicate check, `Processor` looks for a
   cached YNAB transaction with the same account + amount and a date within
   ±3 days (to absorb bank settlement-date drift). Currency and payee/
   description are not part of the match key.
3. **Auto-skip, not drop** — a match is flagged `"ALREADY_IN_YNAB"` in the
   preview (its own stat card and row badge) and, on import, saved with
   status `SKIPPED` and a `MatchNote` describing the matched YNAB
   transaction's date. It's never silently dropped or auto-sent, and stays
   fully recoverable — opening the transaction still allows "Accept & Send
   to YNAB" if the match was a false positive.

Fail-open throughout: with no sync data yet, or if the match lookup errors,
import behaves exactly as it did before this feature existed.
