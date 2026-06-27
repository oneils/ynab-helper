# Settings Page Redesign + Detail Panel Fixes

## Overview

Two parallel UI improvements:

1. **Settings page** (`/settings`): Replace the Pico CSS-era layout with the custom design system. Add a single global budget selector at the top; remove per-section duplicates. Rename "Last known" → "Incremental". Add brief descriptions per sync action. Improve sync history display.

2. **Transaction detail panel**: Widen from 288px → 400px. Switch action buttons from a cramped horizontal row to a vertical stack (full-width), eliminating text wrapping ("Accept & Send\nto YNAB").

No new routes or Go handler changes needed — only templates and CSS (except Task 5, which adds two bool fields to an existing view-data struct and a log line).

## Context (from discovery)

- `ui/html/pages/ynab-settings.tmpl.html` — main settings template (uses Pico CSS patterns: `<article>`, `<footer>`, `role="switch"`, `.grid`)
- `ui/html/partials/sync-statuses.tmpl.html` — sync history partial rendered into `#sync-status` via htmx; also has a duplicate `id="sync-status"` bug (outer section + partial both declare it)
- `ui/html/partials/txn-detail-panel.tmpl.html` — right panel partial; buttons at lines 54–86 in `.detail-actions`
- `ui/static/css/main.css` — design system; `--sidebar-right-width: 288px` at line 72; `.detail-actions` at line 1659; `.txn-detail-panel` at lines 1497–1508
- `internal/server/handlers.go` — `settingsViewHandler` (~line 531), sync handlers (~560–671)
- `internal/server/server.go` line 63 — `GET /sync-history` route → `syncHistoryHandler` (used by the history budget `<select>` hx-get; must stay alive)
- **Button CSS that actually exists**: `button.danger` / `.button.danger`, `button.secondary` / `.button.secondary`, `.button-primary`, `.button-secondary` — no `.btn-ghost`, `.btn-block`, or `.btn-danger` exist

## Development Approach

- **Testing approach:** Regular (code first; CSS/template-only changes have no automated tests)
- CSS and template tasks: verify visually after `go run ./cmd/ynab-helper`
- Run `go test ./...` after Task 5 (only task with Go changes)

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document blockers with ⚠️ prefix

## What Goes Where

- **Implementation Steps** — changes in this repo
- **Post-Completion** — manual browser verification

---

## Implementation Steps

### Task 1: Widen the detail panel

**Files:**
- Modify: `ui/static/css/main.css`

- [ ] change `--sidebar-right-width` from `288px` to `400px` (line 72)
- [ ] update `.txn-detail-panel` `min-width` from `320px` to `400px` (line 1499) to match the new target
- [ ] check `@media (max-width: 1280px)` — the panel is hidden when *closed* (`.txn-detail-panel:not(.txn-detail-panel--open)`), not unconditionally; confirm open panel at narrow widths is acceptable or add an overlay/overflow rule

---

### Task 2: Fix detail panel action buttons

Use the existing button CSS convention (`.button.danger`, `.button.secondary`, `.button-primary`, etc.) — **do not invent new utility classes**.

**Files:**
- Modify: `ui/static/css/main.css`
- Modify: `ui/html/partials/txn-detail-panel.tmpl.html`

- [ ] in `.detail-actions` CSS: add `flex-direction: column; gap: var(--spacing-sm)` and remove any row-specific rules
- [ ] add `.detail-actions button { width: 100% }` so all buttons go full-width (buttons already carry `btn` class which inherits element styles)
- [ ] in `txn-detail-panel.tmpl.html`: change Skip button class from `btn btn-secondary` to `btn button danger` (using existing `button.danger` CSS rule) so it renders as a red/destructive outlined style
- [ ] confirm "Accept & Send to WNAB" / "Save Selections" / "Skip" labels fit without wrapping at 400px

---

### Task 3: Settings page — global budget selector + redesign layout

Replace Pico-CSS-era `<article>`/`<footer>` structure with the custom design system. Single global `<select>` at top propagates to each form via JS + hidden inputs. Guard against submitting an empty budget.

**Files:**
- Modify: `ui/html/pages/ynab-settings.tmpl.html`

- [ ] add page header: `<div class="page-header"><h2>Sync</h2><p class="page-description">Manage YNAB data synchronisation</p></div>`
- [ ] add global budget selector with `hx-get="/sync-history" hx-target="#sync-status" hx-trigger="change"` so changing budget also refreshes the history section (keeps the `/sync-history` route alive):
  ```html
  <div class="form-group settings-budget-selector">
      <label for="global-budget">Budget</label>
      <select id="global-budget" hx-get="/sync-history" hx-target="#sync-status" hx-trigger="change" name="budget">
          <option value="">Select budget…</option>
          {{range .Budgets}}<option value="{{.ID}}">{{.Name}}</option>{{end}}
      </select>
  </div>
  ```
- [ ] replace "Sync Budgets with Accounts" `<article>` with a `.settings-card`; description: "Fetches your budgets and linked accounts from YNAB"; form posts to `/ynab-budgets-sync`; no budget field needed
- [ ] replace "Sync Categories" card: remove its `<select name="budget">`; add `<input type="hidden" name="budget" class="budget-input">`; rename checkbox to "Incremental (from last sync)"; add description "Syncs category groups and categories"; add `disabled` attribute to Sync button by default, removed via JS when a budget is selected
- [ ] replace "Sync Payees" card: same pattern as Categories
- [ ] add inline JS: on global selector change → copy value to `.budget-input` hidden inputs AND enable/disable Sync buttons based on whether a budget is selected:
  ```html
  <script>
  document.getElementById('global-budget').addEventListener('change', function() {
      const val = this.value;
      document.querySelectorAll('.budget-input').forEach(el => el.value = val);
      document.querySelectorAll('.sync-budget-btn').forEach(btn => btn.disabled = !val);
  });
  </script>
  ```
- [ ] mark Sync buttons for Categories/Payees with class `sync-budget-btn` and set `disabled` initially in HTML (since the default option is empty)
- [ ] remove `.grid` wrapper that placed Categories and Payees side-by-side — each card gets its own full-width row

---

### Task 4: Settings page — sync history partial redesign

Fix the duplicate `id="sync-status"` bug and replace Pico cards with design-system table.

**Files:**
- Modify: `ui/html/partials/sync-statuses.tmpl.html`
- Modify: `ui/static/css/main.css`

- [ ] remove the budget `<select>` from the partial — history is now filtered by the global selector's `hx-get="/sync-history"` (Task 3); the partial is rendered both on page load (all history via `settingsViewHandler`) and after sync (filtered via `syncCategoriesHandler`/`syncPayeesHandler`)
- [ ] fix duplicate `id="sync-status"`: the outer `<section id="sync-status">` in `ynab-settings.tmpl.html` line 76 wraps the partial which also opens `<section id="sync-status">` — remove the inner `<section id="sync-status">` wrapper from the partial; the partial content should be a bare fragment that the outer section contains
- [ ] replace `<article>` cards with a simple table using design-system classes; columns: Action, Updated at, Status (badge), Message
- [ ] use existing `.badge` / status badge classes for success/error — check `main.css` for `.badge-success` / `.badge-error` (lines ~1258–1280); if they exist, use them; otherwise use `button.success` / `button.danger` pattern as labels
- [ ] add `.settings-card` CSS: `background: var(--bg-surface); border: 1px solid var(--bg-border); border-radius: var(--radius-lg); padding: var(--spacing-lg); margin-bottom: var(--spacing-md)`
- [ ] "No history found" empty state: wrap in `<p class="text-muted">` or equivalent muted style

---

### Task 5: Fix payee/category dropdowns empty in detail panel

Dropdowns show only "Select…" with no options. Root cause is most likely payees/categories not synced after a fresh DB, but could also be a budget-lookup failure. Add logging + empty-state UI.

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `ui/html/partials/txn-detail-panel.tmpl.html`
- Modify: `ui/static/css/main.css`

- [ ] in `detailBankTxnHandler` after `FetchPayeesByBudget`: add `slog.Info("detail panel data", "budgetID", budget.ID, "payeeCount", len(payees), "categoryCount", len(categories))` so the empty-list case is visible in logs
- [ ] in `txn-detail-panel.tmpl.html`: use `{{if not .Payees}}` directly (no new struct fields needed — Go templates evaluate slice emptiness natively); replace the payee `<select>` with:
  ```html
  {{if not .Payees}}
  <p class="detail-empty-hint">No payees — sync from <a href="/settings">Settings → Sync Payees</a> first.</p>
  {{else}}
  <select id="detail-payee" name="payee" class="detail-select">…</select>
  {{end}}
  ```
- [ ] same pattern for `{{if not .Categories}}`
- [ ] add `.detail-empty-hint` CSS: `font-size: 0.8rem; color: var(--text-muted); margin: var(--spacing-xs) 0`
- [ ] run `go test ./...` — must pass (only Go change is the log line)

---

### Task 6: Verify acceptance criteria

- [ ] run `go test ./...` — all tests pass
- [ ] detail panel opens at 400px wide with no layout clipping
- [ ] all 3 action buttons stack vertically, no text wrapping; Skip renders in danger/red style
- [ ] payee/category empty-state hint shows on a fresh DB; dropdowns populate after syncing payees/categories
- [ ] settings page: changing global budget selector refreshes sync history AND propagates to Categories/Payees forms
- [ ] Categories/Payees Sync buttons are disabled until a budget is selected
- [ ] sync history renders as table with status badges after a sync action
- [ ] settings page looks correct in dark theme (no hardcoded colours)

---

### Task 7: [Final] Cleanup

- [ ] search templates for any remaining `<article>`, `role="switch"`, Pico-specific classes in settings templates
- [ ] move this plan to `docs/plans/completed/`

---

## Technical Details

**Global budget → form wiring pattern:**
```
<select id="global-budget" hx-get="/sync-history" hx-target="#sync-status">  (not inside any form)
  ↕  JS change listener
hidden <input type="hidden" name="budget" class="budget-input"> inside each form
.sync-budget-btn buttons start disabled; enabled when budget is selected
```
POST body is unchanged — handlers still read `r.PostForm.Get("budget")`.
`GET /sync-history` route (server.go:63) stays alive — global selector's `hx-get` drives history filtering on budget change.

**Button CSS (existing classes only):**
| Button | Class | Notes |
|---|---|---|
| Accept & Send to YNAB | `btn button-primary` | uses `.button-primary` rule |
| Save Selections | `btn button-secondary` | uses `.button-secondary` rule |
| Skip | `btn button danger` | uses `button.danger` rule → red/destructive |

**Panel width:**
- `--sidebar-right-width: 400px` (was 288px)
- Panel is hidden only when *closed* (`not(.txn-detail-panel--open)`), not unconditionally at narrow widths — confirm overflow behaviour is acceptable at <1280px

---

## Post-Completion

**Manual browser verification:**
- Test settings sync flow: select budget → Sync Categories → confirm history table updates
- Test at 1440px and 1280px for panel width and layout correctness
- Test detail panel at both viewport sizes; confirm buttons don't wrap at 400px
- Test payee/category empty-state and populated state
