# Sortable Imported Transactions

## Overview

The Imported Transactions page (`/import-bank-txns`) currently lists transactions
hardcoded oldest-first (`ORDER BY txn_time ASC` in the SQL store). This plan:

1. Flips the default order to **newest-first** (more conventional for a
   transaction feed — matches banking apps, email, etc.).
2. Adds a single toggle button, styled consistently with the existing
   `status-tabs` filter row, that flips between newest-first and oldest-first.
   The button label always describes the action a click will perform (e.g.
   showing "Newest first" while oldest-first is active, and vice versa), with
   a directional arrow icon.
3. Persists the chosen order per-browser via `localStorage` so it survives a
   full page reload, since the app has no server-side session/cookie layer.

**Blast radius**: the list is rendered by **seven** handlers sharing one
duplicated anonymous struct shape (`bankTxnsHandler`, `skipBankTxnHandler`,
`uploadTxnToYnabHandler`, `uploadBankTxnsHandler`, `saveInlineTxnHandler`,
`bulkSkipTxnsHandler`, plus `bankTxnRowsHandler` for the infinite-scroll
fragment, plus `importBankTxnsHandler` for the initial page shell), the
`TransactionStorer` interface, and its test mock. This plan changes all of
them — it is not a narrow, single-handler change.

## Context (from discovery)

- **Stack**: Go 1.25 + `go-chi/chi/v5`, SQLite (`modernc.org/sqlite`) via
  `pressly/goose` migrations, server-rendered `html/template` pages/partials,
  HTMX 1.9.12 (CDN) for partial swaps, small vanilla-JS helpers in
  `ui/static/js/`. No session/cookie/localStorage usage exists yet anywhere in
  the codebase — all current filter state (budget/account/status) is
  stateless, carried purely via URL query params on each HTMX request.
  `hx-push-url="true"` does exist elsewhere (main nav, `ui/html/index.tmpl.html`)
  but not on any transaction-list endpoint — those stay URL-query-only by
  existing convention. This plan keeps that convention and adds `localStorage`
  only for the one preference that should survive a reload (see Task 4).
- **Current sort**: hardcoded in
  `internal/sqlite/transaction_store.go:100` — `query += " ORDER BY txn_time ASC"`
  inside `FetchTransactionsByAccount`. This is the only query method behind
  the imported-transactions list; no in-memory re-sort happens anywhere else.
  `internal/txn/processor.go`'s `Processor.Fetch` just passes through to it.
- **`txn_time` is date-only, not a real timestamp**: every parser
  (`internal/parser/ing.go:80`, `millenium.go:75`, `pko.go:96`,
  `revolut.go:91`, `santander.go:78`) parses dates with `DateFormat`
  `"2006-01-02"` or `"02-01-2006"` — no time component. Same-day transactions
  tie on `txn_time`, and SQLite gives ties no stable order. Since infinite
  scroll re-runs the full query per page and slices in Go (see below), tied
  rows can land differently across two requests, duplicating or skipping rows
  across scroll pages. **A deterministic tiebreaker is required**, not
  optional polish (Task 1).
- **List architecture**: DB-sorted, then **infinite scroll** (not classic
  pagination) — pagination is in-memory Go slicing (`txns[start:end]`) on top
  of the already-sorted full result. `importBankTxnsHandler`
  (`handlers.go:93`, full page shell) and `bankTxnsHandler` (`handlers.go:358`,
  filtered reload) both do this; `bankTxnRowsHandler` (`handlers.go:303`,
  load-more fragment) does too. A sort param must flow through all three, plus
  every action handler that re-renders the list after a mutation (see below).
- **The seven `bank-transactions` render sites** — all currently build an
  *identical* anonymous struct
  `{Txns []TxnListRow; PageMeta PageMeta; Budget, Account, ActiveStatus string;
  StatusCounts map[string]int}` and call
  `s.render(w, http.StatusOK, "import-txns.tmpl.html", "bank-transactions", data)`:
  `bankTxnsHandler` (`handlers.go:421`), `skipBankTxnHandler` (`:683`),
  `uploadTxnToYnabHandler` (`:797`), `uploadBankTxnsHandler` (`:1069`),
  `saveInlineTxnHandler` (`:1350`), `bulkSkipTxnsHandler` (`:1438`). Since this
  struct is already duplicated 7 times with the exact same shape, this plan
  extracts it into one named type (`TxnListData`) rather than editing 7
  anonymous struct literals by hand — DRY is clearly warranted here, not
  premature abstraction, given the shape already repeats verbatim today.
- **Existing toggle pattern to mirror**: `ui/html/partials/status-tabs.tmpl.html`
  — HTMX buttons (`hx-get="/bank-txns?..."`, `hx-target="#txn-list-panel"`,
  `hx-swap="innerHTML"`) with server-driven active state
  (`{{if eq .ActiveStatus "X"}}`) plus a belt-and-suspenders inline `onclick`
  that flips CSS classes client-side before the swap lands. These buttons live
  in `.filter-card`, *outside* `#txn-list-panel`, and each button's URL is
  fixed (always points at one literal status) — no attribute ever needs
  updating after a click. **This plan places the sort toggle differently**:
  see Technical Details for why it goes *inside* `#txn-list-panel` (top of
  `bank-transactions.tmpl.html`) instead of in `.filter-card` like the status
  tabs — a flip-button's target changes every click, and putting it inside the
  swap target lets every list re-render (including the six action handlers
  above) refresh it for free, with no out-of-band swap machinery.
- **No persistence layer** exists anywhere (confirmed via full-repo grep for
  `localStorage`/session/cookie code). `localStorage` is new to this codebase;
  the read/write lives in a small inline `<script>` block, matching the
  existing pattern already used in `import-txns.tmpl.html:60-89` for other
  page-load JS.
- **Files involved**:
  - `internal/sqlite/transaction_store.go` (+ `transaction_store_test.go`)
  - `internal/txn/processor.go` (`TransactionStorer` interface, `ProcessParams`)
  - `internal/txn/processor_test.go` (`mockTransactionStore`)
  - `internal/server/handlers.go` (7 render sites above + `TxnListData` type)
  - `internal/server/handlers_test.go`
  - `ui/html/pages/import-txns.tmpl.html`
  - `ui/html/partials/bank-transactions.tmpl.html`
  - `ui/html/partials/txn-rows.tmpl.html`
  - `ui/html/partials/status-tabs.tmpl.html`
  - `ui/html/partials/accounts-select.tmpl.html`
  - `ui/html/partials/txn-detail-panel.tmpl.html` (save-inline actions target `#txn-list-panel` too)
  - `ui/static/css/main.css`
- **Existing test conventions**: `internal/sqlite/transaction_store_test.go`
  uses a real in-memory SQLite DB (`sql.Open("sqlite", ":memory:")`), no
  mocks, sequential `t.Fatalf` assertions. `internal/server/handlers_test.go`
  tests pure helper functions directly (e.g. `TestParsePagination_*`,
  `TestNewPageMeta_*`) rather than the HTTP handlers themselves, and can
  execute a single named template via the exported `NewTemplateCache()`
  (`handlers.go:1529`) — used in Task 3 to test the toggle's rendered
  attributes without a JS test runner. No mocking framework; dependencies
  passed as plain closures. Test command: `make test` →
  `go test -race -vet=off -coverprofile=cover.out ./...`.
- **No `CLAUDE.md` exists in this repo.** The closest project-conventions doc
  is `FEATURES.md`.

## Development Approach

- **Testing approach**: Regular (implement, then write/update tests per task).
- Complete each task fully, with passing tests, before moving to the next.
  Task 1's test gate is scoped to `go test ./internal/sqlite/...` — the full
  suite (`go test ./...`) won't go green until Task 2 lands the
  `TransactionStorer` interface and mock updates, since those live outside
  `internal/sqlite`.
- No e2e framework (Playwright/Cypress) exists in this repo — manual
  verification only, listed under Post-Completion.
- Backward compatibility: `/bank-txns` and friends are HTMX partial
  endpoints, never pushed to the address bar (no `hx-push-url` on them), so
  there's no bookmarked-URL scenario to preserve. The compatibility
  requirement is narrower: any request that omits `sort` must behave exactly
  as the new default (newest-first) — never error.

## Testing Strategy

- **Unit tests**: required for every task — store-layer sort + tiebreaker
  behavior, the new `parseSortOrder` handler helper, `Processor.Fetch`
  forwarding `Sort` to the store, and template-rendering assertions for the
  toggle button's attributes in both states.
- **e2e tests**: none exist in this project; see Post-Completion for manual
  verification steps instead.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.

## Implementation Steps

### Task 1: Add sort order + stable tiebreaker to the store layer

**Files:**
- Modify: `internal/sqlite/transaction_store.go`
- Modify: `internal/sqlite/transaction_store_test.go`

- [x] change `FetchTransactionsByAccount(ctx, accID, status string)` to
      `FetchTransactionsByAccount(ctx, accID, status, sortOrder string)`
- [x] normalize `sortOrder` to a whitelisted `ASC`/`DESC` SQL fragment
      (`dir := "DESC"; if strings.EqualFold(sortOrder, "asc") { dir = "ASC" }`)
      — never interpolate the raw param directly, keep it to the two literal
      values to avoid any injection surface
- [x] replace `query += " ORDER BY txn_time ASC"` with
      `query += " ORDER BY txn_time " + dir + ", id " + dir` — the second key
      gives same-day transactions (all parsers are date-only, see Context) a
      deterministic order so infinite-scroll pages never duplicate/skip rows
- [x] write test `TestFetchTransactionsByAccount_SortDesc` — insert several
      transactions with distinct `txn_time`, fetch with `sortOrder="desc"`,
      assert descending order
- [x] write test `TestFetchTransactionsByAccount_SortAsc` — same setup,
      `sortOrder="asc"`, assert ascending order
- [x] write test `TestFetchTransactionsByAccount_TiedTxnTimeIsStable` —
      insert multiple transactions sharing one `txn_time`, run the query
      twice (both `asc` and `desc`), assert identical row order both times
- [x] write test `TestFetchTransactionsByAccount_SortInvalidDefaultsToDesc` —
      garbage `sortOrder` value (e.g. `"'; DROP TABLE"`) falls back to
      descending, proving the whitelist holds
- [x] run `go test ./internal/sqlite/...` — must pass before task 2 (the
      wider `go test ./...` is expected to fail until Task 2 updates the
      `TransactionStorer` interface and its mock — that's fine, not a
      regression to chase down here)

### Task 2: Introduce `TxnListData`, thread sort through every handler

**Files:**
- Modify: `internal/txn/processor.go`
- Modify: `internal/txn/processor_test.go`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/handlers_test.go`

- [x] add `FetchTransactionsByAccount(ctx context.Context, accID, status,
      sortOrder string) ([]Transaction, error)` to the `TransactionStorer`
      interface (`processor.go:15-21`) and update `mockTransactionStore`
      (`processor_test.go:31`) to match
- [x] add `Sort string` field to `ProcessParams`; update `Processor.Fetch` to
      pass `params.Sort` through to `txnStore.FetchTransactionsByAccount`
- [x] write test `TestProcessorFetch_PassesSortToStore` on the existing
      `mockTransactionStore` (asserts the value received matches what was
      passed in `ProcessParams`)
- [x] add `parseSortOrder(r *http.Request) string` helper next to
      `parsePagination` in `handlers.go` — reads `sort` query/form param,
      returns `"asc"` only on exact case-insensitive match, else defaults to
      `"desc"`
- [x] define a named type `TxnListData` in `handlers.go` with the fields
      currently duplicated across all 7 render sites — `Txns []TxnListRow`,
      `PageMeta PageMeta`, `Budget string`, `Account string`,
      `ActiveStatus string`, `StatusCounts map[string]int`, plus the new
      `Sort string` — and replace the anonymous struct literals in
      `bankTxnsHandler` (`:421`), `skipBankTxnHandler` (`:683`),
      `uploadTxnToYnabHandler` (`:797`), `uploadBankTxnsHandler` (`:1069`),
      `saveInlineTxnHandler` (`:1350`), and `bulkSkipTxnsHandler` (`:1438`)
      with it
- [x] in each of those 6 handlers plus `bankTxnRowsHandler` and
      `importBankTxnsHandler`, call `sort := parseSortOrder(r)`, pass
      `Sort: sort` into the `txn.ProcessParams{...}` used for the `Fetch`
      call, and set `Sort: sort` on the resulting `TxnListData`
      (`bankTxnRowsHandler` and `importBankTxnsHandler` use their own
      per-handler struct today — give both a `Sort` field too, named
      consistently)
- [x] write tests `TestParseSortOrder_DefaultsToDesc`,
      `TestParseSortOrder_ExplicitAsc`,
      `TestParseSortOrder_InvalidValueDefaultsToDesc` (mirroring the
      `TestParsePagination_*` table style)
- [x] run `go test ./...` — full suite must pass before task 3

### Task 3: Add the sort toggle inside the list panel, wired through HTMX

**Files:**
- Modify: `ui/html/partials/bank-transactions.tmpl.html`
- Modify: `ui/html/partials/txn-rows.tmpl.html`
- Modify: `ui/html/partials/status-tabs.tmpl.html`
- Modify: `ui/html/partials/accounts-select.tmpl.html`
- Modify: `ui/html/partials/txn-detail-panel.tmpl.html`
- Modify: `ui/html/pages/import-txns.tmpl.html`
- Modify: `internal/server/handlers_test.go`

- [x] add a `{{define "sort-toggle"}}` block at the **top of
      `bank-transactions.tmpl.html`**, inside the `{{define "bank-transactions"}}`
      body, above the `{{if .Txns}}` table block — a single
      `<button id="sort-toggle-btn" class="sort-toggle">` whose label/icon/
      `hx-get` target the *opposite* of `.Sort` (e.g. currently `desc` →
      button reads "Oldest first" and requests `sort=asc`),
      `hx-target="#txn-list-panel"`, `hx-swap="innerHTML"`, an inline
      `onclick` that writes the new value to `localStorage` (Task 4), plus a
      hidden `<input type="hidden" id="sort-state" name="sort"
      value="{{.Sort}}">` next to it. Because this whole block is inside
      `bank-transactions`, it re-renders correctly on *every* list response —
      status change, account/budget change, skip, save, upload, bulk-skip, or
      the toggle itself — with no out-of-band swap needed
  - one design note worth recording: an out-of-band (`hx-swap-oob`) copy of
    the button living in `.filter-card` was considered and rejected — it
    would need to stay byte-identical to an in-panel copy, risks rendering
    twice on the full page load, and buys nothing this simpler placement
    doesn't already give for free
- [x] add `hx-include="#sort-state"` to all five status-tab buttons in
      `status-tabs.tmpl.html` (not a hardcoded `&sort=` query value — the
      buttons live outside `#txn-list-panel` and never re-render themselves,
      so their `hx-get` URL can't be updated after a toggle click; including
      the always-current hidden input is the only way they stay correct)
- [x] extend the account `<select>`'s `hx-include` in
      `accounts-select.tmpl.html` from `"[name='budget']"` to
      `"[name='budget'], #sort-state"` so switching accounts preserves sort
- [x] add `hx-include="#sort-state"` to: the skip button in
      `txn-rows.tmpl.html` (`hx-post .../skip`), and the save/skip actions in
      `txn-detail-panel.tmpl.html` (`hx-post` calls around lines 61, 73, 82)
      — without this, skipping or saving a transaction silently resets the
      visible list to the default order mid-session
- [x] add `&sort={{$.Sort}}` to the infinite-scroll sentinel's `hx-get` in
      `txn-rows.tmpl.html:54` so "load more" pages keep the same order as the
      initial page
- [x] write a template-rendering test (using `NewTemplateCache()`) asserting
      the `sort-toggle` button's `hx-get` and visible label differ correctly
      between `TxnListData{Sort: "asc"}` and `TxnListData{Sort: "desc"}`
- [x] manual test (skipped - not automatable): manually verify (dev server)
      that toggling sort re-orders the list; switching status tabs, account,
      or budget keeps the chosen sort; skipping/saving a transaction keeps
      the chosen sort; scrolling to load more keeps the chosen sort
- [x] run full test suite — must pass before task 4

### Task 4: Persist sort choice via localStorage

**Files:**
- Modify: `ui/html/pages/import-txns.tmpl.html`

- [x] extend the existing inline `<script>` IIFE (around line 60) with: on
      load, read `localStorage.getItem('ynab_txn_sort')`; if it's a valid
      value (`"asc"`/`"desc"`), a budget is selected (`{{if .Budget}}` guard,
      server-side), and it differs from the server-rendered `{{.Sort}}`,
      re-fetch the list panel via `htmx.ajax('GET', url, {target:
      '#txn-list-panel', swap: 'innerHTML'})` — build `url` from the same
      values already in the DOM at page-load time (`#budget-filter`,
      `#account`, the active `.status-tab`'s status, read directly rather
      than re-deriving), so the returning user's preference wins over the
      hardcoded newest-first default without a jarring second render
- [x] confirm the toggle button's `onclick` (added in Task 3) writes the
      clicked-to value to `localStorage.setItem('ynab_txn_sort', ...)`
- [x] run full test suite — must pass before task 5 (no new Go code in this
      task; template-rendering coverage for the toggle itself was written in
      Task 3, since that's where the templates were introduced)

### Task 5: Style the sort toggle

**Files:**
- Modify: `ui/static/css/main.css`

- [ ] add `.sort-toggle` styled consistently with the existing
      `.pagination-btn` (outlined button) and `.action-link` (icon + text row)
      patterns already in `main.css`
- [ ] add `.sort-toggle:hover` / `:disabled` (while a request is in flight,
      via `hx-disabled-elt="this"` on the button) states
- [ ] manually verify against the existing theme CSS variables
      (`var(--ui-primary)`, `var(--text-secondary)`, etc. — this repo has one
      `:root` palette, no light/dark toggle) — no visual regression test
      exists in this repo, manual check only
- [ ] no unit tests applicable (CSS-only change); run full test suite anyway
      to confirm nothing else broke

### Task 6: Verify acceptance criteria

- [ ] verify default order on a fresh `/import-bank-txns` load is
      newest-first
- [ ] verify clicking the toggle flips to oldest-first and the button label
      updates to offer "Newest first" next
- [ ] verify the choice survives a full page reload (localStorage) for the
      same browser
- [ ] verify switching status tab / account / budget preserves the chosen
      sort order
- [ ] verify skipping or saving a transaction preserves the chosen sort order
- [ ] verify infinite-scroll "load more" keeps consistent order — no
      duplicate/out-of-order rows across pages, including for a day with
      multiple same-`txn_time` transactions
- [ ] run full test suite: `make test`
- [ ] confirm no e2e suite exists to run (documented above) — skipped
      intentionally, not silently

### Task 7: [Final] Update documentation

- [ ] update `FEATURES.md` if this introduces a pattern worth recording
      (e.g. "toggle controls that must re-render live inside the swap
      target, not beside it"; "localStorage is now used for one per-browser
      UI preference")
- [ ] move this plan to `docs/plans/completed/`

## Technical Details

**Why the toggle lives inside `#txn-list-panel`, not `.filter-card`:**
`status-tabs` buttons are a *fixed segmented control* — each button's
`hx-get` always points at one literal status value, so nothing about a
button's own markup needs to change after a click; only its CSS active-class
needs updating, which the existing client-side `onclick` already does. A
single *flip* button is different: after a click, the next click must go the
*other* direction, so the button's own `hx-get` URL (and label/icon) must be
re-rendered with the new state. Placing it inside `bank-transactions.tmpl.html`
means it's part of the exact content every one of the seven list-rendering
handlers already replaces — it refreshes for free, with no out-of-band swap,
no duplicate-definition drift risk, and no duplicate-DOM-id risk on the full
page load (where `bank-transactions` is rendered inline once, not twice).

**Sort param flow:**
```
?sort=asc|desc  (query/form param, default "desc" = newest-first)
  → parseSortOrder(r) in handlers.go   [new helper, mirrors parsePagination]
  → txn.ProcessParams.Sort
  → Processor.Fetch(ctx, params)
  → TransactionStorer.FetchTransactionsByAccount(ctx, accID, status, sortOrder)
  → "ORDER BY txn_time " + dir + ", id " + dir   [whitelisted dir, tiebreak on id]
```

**Why `#sort-state` (hidden input) instead of baking `&sort=` into URLs**:
Controls that live outside `#txn-list-panel` (status tabs, account/budget
selects) are never re-rendered by a list response, so a URL value baked into
their `hx-get` attribute would freeze at page-load time and go stale the
moment the user toggles sort once. `hx-include="#sort-state"` reads the
*current* value from the DOM at request time instead — and since that hidden
input is itself part of `bank-transactions.tmpl.html`, it's kept current by
every list re-render, the same mechanism that keeps the toggle button itself
current.

**Why `localStorage` for sort specifically, and not the other filters**:
Budget/account/status are working selections the user actively re-picks each
session (which account am I reconciling right now); sort direction is closer
to a standing display preference (I always want newest-first) that's
annoying to re-set every visit. Scoping persistence to just this one
preference avoids introducing a bigger state-management change (e.g.
`hx-push-url` across every filter) for a plan whose actual ask was the sort
toggle.

## Post-Completion

**Manual verification:**
- Toggle sort with a large transaction set (multiple infinite-scroll pages,
  including a day with several same-`txn_time` transactions) to confirm no
  row gets skipped or duplicated across the sort change.
- Verify behavior in a private/incognito window (no stored preference) shows
  newest-first by default.
- Verify clearing site data resets to the newest-first default cleanly.
