# Import History UI Improvements

## Overview

Three improvements to the Import History page:
1. **Table scrolling** — fix the transaction table height, add scroll with a sticky header
2. **Pagination** — server-side pagination via HTMX (page/limit query params)
3. **Unified filter bar** — combine Budget, Account dropdowns and status tabs into one cohesive visual block

## Context (from discovery)

- Template: `ui/html/pages/import-txns.tmpl.html`
- Partials: `ui/html/partials/bank-transactions.tmpl.html`, `ui/html/partials/status-tabs.tmpl.html`, `ui/html/partials/accounts-select.tmpl.html`
- Handlers: `internal/server/handlers.go` — `importBankTxnsHandler` (L75), `bankTxnsHandler` (L214)
- CSS: `ui/static/css/main.css` — classes `txn-split-layout`, `txn-list-panel`, `budget-acct-bar`, `.status-tabs`
- Routing: `internal/server/server.go`

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Changes limited to CSS, HTML templates, and Go handlers — minimal scope
- Backward compatibility: HTMX endpoints without pagination continue to work (default page=1)

## Implementation Steps

---

### Task 1: Table scrolling — fix height and add sticky header

**Files:**
- Modify: `ui/static/css/main.css`
- Modify: `ui/html/partials/bank-transactions.tmpl.html`

- [ ] add `display: flex; flex-direction: column; overflow: hidden` to `.txn-list-panel`
- [ ] wrap `<table>` in `<div class="txn-table-scroll">` with `overflow-y: auto; flex: 1`
- [ ] set `max-height: calc(100vh - 260px)` on `.txn-table-scroll` (tune as needed to fit the layout)
- [ ] make `<thead>` sticky: `.transactions-table thead th { position: sticky; top: 0; z-index: 2; background: var(--bg-secondary); }`
- [ ] NOTE: table rows use `hx-get="/bank-txns/{id}/detail"` with `hx-target="#txn-detail-panel"` — the scroll wrapper does not affect this, no HTMX risk
- [ ] visual check: scroll the table — the header must remain in place

---

### Task 2: Unified filter bar — combine Budget/Account dropdowns and status tabs

**Files:**
- Modify: `ui/html/pages/import-txns.tmpl.html`
- Modify: `ui/html/partials/status-tabs.tmpl.html`
- Modify: `ui/static/css/main.css`

**Problem:** Budget, Account dropdowns and status tabs look like three unrelated elements with inconsistent styling.
**Solution:** One `.filter-card` container with two rows and an internal divider.

```
┌─────────────────────────────────────────────────────────┐
│  🔷 Budget          💳 Account                          │
│  [Poland Family ▼]  [All Accounts ▼]                    │
│ ─────────────────────────────────────────────────────── │
│  All   Needs Review 32   Accepted   Skipped   Invalid   │
└─────────────────────────────────────────────────────────┘
```

- [ ] in `import-txns.tmpl.html`, wrap `.budget-acct-bar` and the `status-tabs` include in a single `<div class="filter-card">`
- [ ] remove the separate `margin` between dropdowns and tabs — use CSS `border-top` inside the card as the separator
- [ ] CSS `.filter-card` — `background: var(--bg-secondary); border: 1px solid var(--border); border-radius: 8px; padding: 0; overflow: hidden`
- [ ] `.filter-card .budget-acct-bar` — `padding: 16px 20px 14px`
- [ ] `.filter-card .status-tabs` — `padding: 0 20px; border-top: 1px solid var(--border); background: transparent`
- [ ] increase spacing between icon+label and the dropdown: `gap: 6px; margin-bottom: 8px`
- [ ] status tabs: remove outer border-radius (they sit inside the card), align `.status-tab` height to `38px`
- [ ] verify all three themes (auto/light/dark) — `var(--border)` and `var(--bg-secondary)` must render correctly

---

### Task 3: Pagination — server-side, HTMX-friendly

**Files:**
- Modify: `internal/server/handlers.go`
- Create: `ui/html/partials/pagination.tmpl.html`
- Modify: `ui/html/partials/bank-transactions.tmpl.html`

**Approach:** page + limit query params; `bankTxnsHandler` returns a slice + pagination metadata.
Pagination renders inside the `bank-transactions` partial; HTMX replaces `#txn-list-panel` entirely — **the same target used by status tabs** (no new container ID introduced).

**Key decisions from review:**
- ❌ Do NOT add `id="txn-list-container"` inside the partial — it would cause HTMX to nest the response inside itself. Pagination targets the same `#txn-list-panel` as status tabs.
- ❌ Do NOT use `| add` in templates — no such function exists in `html/template`. Pre-compute `PrevPage int` and `NextPage int` in `PageMeta` on the Go side.
- ✅ Use `ActiveStatus` (not `Status`) for the active filter field — matches the existing template field name.

- [ ] add helper `parsePagination(r *http.Request) (page, limit int)` to `handlers.go` — defaults page=1, limit=50; clamp limit to 200
- [ ] in `bankTxnsHandler` (L214): apply pagination to the `txns` slice after filtering; pass `PageMeta{Page, PrevPage, NextPage, Limit, Total, TotalPages}` to the template
- [ ] in `importBankTxnsHandler` (L75): likewise add `PageMeta`
- [ ] create `ui/html/partials/pagination.tmpl.html`:
  - Previous / Next buttons (disabled when `PrevPage == 0` / `NextPage == 0`)
  - "Page X of Y (Z transactions)"
  - HTMX: `hx-get="/bank-txns"`, `hx-vals` with page/limit/budget/account/status, `hx-target="#txn-list-panel"`, `hx-swap="innerHTML"`
  - NOTE: switching status tab naturally resets to page=1 (page is not included in tab URLs — expected behavior)
- [ ] in `bank-transactions.tmpl.html`, include `{{ template "pagination" . }}` after `</table>` (no wrapper with a new ID)
- [ ] write unit test for `parsePagination` (valid values, defaults, edge cases — page < 1, limit > 200)
- [ ] run tests: `go test ./...`

---

### Task 4: Verify acceptance criteria

- [ ] open the page — table with >10 transactions scrolls; header stays pinned
- [ ] Budget + Account dropdowns + status tabs appear as a single block (one border around all)
- [ ] pagination: Prev/Next buttons change the page without a full reload; active status filter is preserved
- [ ] verify all three themes (auto/light/dark) — no visual artifacts
- [ ] verify responsive (≤1024px) — filter-card does not break
- [ ] run `go test ./...` — all tests green

### Task 5: [Final] Update documentation

- [ ] update CLAUDE.md if new patterns were discovered
- [ ] move this plan to `docs/plans/completed/`

## Technical Details

**Pagination data struct (Go):**
```go
type PageMeta struct {
    Page       int
    PrevPage   int // 0 when on the first page (disables Prev button)
    NextPage   int // 0 when on the last page (disables Next button)
    Limit      int
    Total      int
    TotalPages int
}
```

**Template data (bankTxnsHandler):**
```go
// Add PageMeta to the existing anonymous struct in the handler
// Use ActiveStatus (not Status) — matches the field name in existing templates
struct {
    Txns         []TxnListRow
    PageMeta     PageMeta
    Budget       string
    Account      string
    ActiveStatus string    // matches status-tabs.tmpl.html
    StatusCounts map[string]int
    // ...other existing fields
}
```

**HTMX pagination link example:**
```html
{{if .PageMeta.PrevPage}}
<button hx-get="/bank-txns"
        hx-vals='{"page": "{{.PageMeta.PrevPage}}", "limit": "{{.PageMeta.Limit}}", "budget": "{{.Budget}}", "account": "{{.Account}}", "status": "{{.ActiveStatus}}"}'
        hx-target="#txn-list-panel"
        hx-swap="innerHTML">
  Previous
</button>
{{end}}
```

**Performance note:** `enrichTransactionList` enriches all transactions before pagination is applied (YNAB API calls per request). Acceptable at limit=50, but may become a bottleneck at 500+ transactions. Optimization (slice first, enrich second) is out of scope for this plan.

**CSS variables** already in use in the project (`var(--bg-secondary)`, `var(--border)`) — rely on them for theme support.

## Post-Completion

**Manual verification:**
- Check performance with a large number of transactions (100+)
- Confirm that scrolling the table does not cause the right-side detail panel to jump
- Confirm HTMX does not lose the selected row when changing pages (current selection state)
