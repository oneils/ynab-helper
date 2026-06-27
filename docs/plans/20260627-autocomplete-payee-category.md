# Autocomplete for Payee & Category in Import Bank Transactions Detail Panel

## Overview
Replace the static `<select>` dropdowns for Payee and Category in the right-side detail panel (`/import-bank-txns`) with HTML5 `<input>` + `<datalist>` autocomplete fields. When the user selects a payee, the category datalist dynamically narrows to suggestions linked to that payee via `/api/category-suggestions`.

Problem solved: with 50+ payees/categories, the current dropdown is slow to navigate. Autocomplete lets users type to filter instantly.

## Context (from discovery)
- Files/components involved:
  - `ui/html/partials/txn-detail-panel.tmpl.html` — right panel template
  - `internal/server/handlers.go` — `detailBankTxnHandler()` (lines 322–389)
  - `ui/static/js/datalist-sync.js` — existing ID sync for table rows (not reusable directly)
  - `ui/static/css/main.css` — `.detail-select` styling (lines 1688–1705)
  - `internal/server/server.go` — route registration (line 57–58 for suggestion APIs)
- Related patterns: `datalist-sync.js` already handles text→ID sync for `<input list=...>` in table rows; detail panel needs similar logic but outside a `<tr>` context
- Dependencies: `/api/category-suggestions` already exists and returns `{"suggestions": [...]}` (wrapped object; each item has `category_id` and `category_name` fields per `internal/txn/suggestions.go` lines 37–44)

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- No new Go dependencies; no new JS libraries
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Testing Strategy
- **Unit tests**: Go handler tests to verify template data includes payee/category names
- **E2E / manual**: Verify in browser at `http://localhost:8080/import-bank-txns`
  - Typing in Payee input filters the list
  - Selecting a Payee auto-updates Category suggestions
  - Form submission still sends correct IDs (not names) to backend
  - Pre-filled suggestions appear correctly as text in inputs

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## What Goes Where
- **Implementation Steps**: all code changes live in this repo
- **Post-Completion**: manual browser testing of the golden path

## Implementation Steps

### Task 1: Extend Go handler to expose payee/category names in template data

**Files:**
- Modify: `internal/server/handlers.go`

The current `detailBankTxnHandler` passes `SugPayeeID` and `SugCategoryID` but not the names. The template needs names to pre-fill text inputs. Also needs `BudgetID` accessible on the form for the JS fetch to category-suggestions.

- [x] In `detailBankTxnHandler` (line ~370), add `SugPayeeName` and `SugCategoryName` fields to the anonymous template data struct
- [x] Populate `SugPayeeName` by looking up the payee name from the fetched payees slice matching `SugPayeeID`
- [x] Populate `SugCategoryName` by looking up the category name from the fetched categories slice matching `SugCategoryID`
- [x] Ensure `BudgetID` is already in the struct (it is — line 374); confirm it's correct
- [x] Extract two pure helper functions in `handlers.go` (or a nearby file): `payeeNameByID(payees []ynab.Payee, id string) string` and `categoryNameByID(categories []ynab.Category, id string) string`; these keep the handler lean and are trivially testable
- [x] Write unit tests `TestPayeeNameByID` and `TestCategoryNameByID` covering: match found, no match (returns ""), empty slice — following the existing pure-function test pattern in `handlers_test.go`
- [x] Run tests: `go test ./internal/server/...` — must pass before Task 2

### Task 2: Replace `<select>` with `<input>` + `<datalist>` in txn-detail-panel.tmpl.html

**Files:**
- Modify: `ui/html/partials/txn-detail-panel.tmpl.html`

Replace both dropdowns. Each visible text input gets a paired hidden input for the ID (which the form submits).

- [ ] Replace Payee `<select id="detail-payee" name="payee">` with:
  ```html
  <input
    type="text"
    id="detail-payee-input"
    class="detail-autocomplete"
    list="detail-payees-list"
    placeholder="Type to search payee…"
    value="{{.SugPayeeName}}"
    autocomplete="off"
  >
  <datalist id="detail-payees-list">
    {{range .Payees}}
    <option value="{{.Name}}" data-id="{{.ID}}"></option>
    {{end}}
  </datalist>
  <input type="hidden" id="detail-payee-id" name="payee" value="{{.SugPayeeID}}">
  ```
- [ ] Remove the old "Select…" empty option and "(suggested)" hint paragraph for Payee; suggestion is now shown via pre-filled `value="{{.SugPayeeName}}"`
- [ ] Replace Category `<select id="detail-category" name="category">` with:
  ```html
  <input
    type="text"
    id="detail-category-input"
    class="detail-autocomplete"
    list="detail-categories-list"
    placeholder="Type to search category…"
    value="{{.SugCategoryName}}"
    autocomplete="off"
  >
  <datalist id="detail-categories-list">
    {{range .Categories}}
    <option value="{{.Name}}" data-id="{{.ID}}"></option>
    {{end}}
  </datalist>
  <input type="hidden" id="detail-category-id" name="category" value="{{.SugCategoryID}}">
  ```
- [ ] Remove old "(suggested)" hint paragraph for Category
- [ ] Add `data-description="{{.Txn.Description}}"` attribute to the wrapping `<form>` element so JS can read it for the category-suggestions API call; read `budget_id` from the existing `<input type="hidden" name="budget">` field (already present in the form) rather than adding a duplicate attribute
- [ ] Verify the empty-state messages ("No payees synced yet…", "No categories synced yet…") are preserved or adapted for the no-data case
- [ ] Manual check: load the detail panel in browser, confirm inputs appear and datalist dropdown opens on typing
- [ ] No new Go tests needed for this task (pure template change; covered by Task 1 tests)

### Task 3: Create detail-panel.js for ID sync and dynamic category fetch

**Files:**
- Create: `ui/static/js/detail-panel.js`
- Modify: `ui/html/index.tmpl.html` (add `<script>` tag alongside existing scripts at lines 158–163)

The existing `datalist-sync.js` is scoped to `<tr>` rows (`input.closest('tr')`) and won't find hidden fields in the detail panel. This new module handles the detail panel specifically. All scripts in this project are registered centrally in `ui/html/index.tmpl.html` — do NOT add `<script>` to the page-level template.

- [ ] Create `ui/static/js/detail-panel.js` with an IIFE

- [ ] **Store the full category option set**: on `init()`, read all `<option>` elements from `#detail-categories-list` and store in a JS-side array (`fullCategoryOptions`); use this to restore the list when the payee is cleared or changed

- [ ] **Payee ID sync**: listen to `change` and `blur` on `#detail-payee-input`; find matching `<option>` in `#detail-payees-list` by value (case-insensitive); write `option.dataset.id` to `#detail-payee-id`; if no match, clear hidden field and restore full category list; trigger category update if a valid payee ID was found

- [ ] **Dynamic category update on payee selection** — non-destructive design:
  1. Read `budget_id` from existing `<input name="budget">` and `description` from form `data-description` attribute
  2. Track a `latestPayeeId` variable; set it to the current payee ID before the fetch; discard the response if it doesn't match (prevents stale race)
  3. Fetch `GET /api/category-suggestions?budget_id=X&description=Y&payee_id=Z`
  4. Read `response.json().suggestions` (array of `{category_id, category_name}` objects)
  5. If suggestions returned and payee ID still matches: replace `#detail-categories-list` options with suggestions only (but keep `fullCategoryOptions` intact for later restore); auto-fill `#detail-category-input` + hidden field with `suggestions[0]` if exactly one result
  6. If no suggestions or fetch fails: restore `#detail-categories-list` from `fullCategoryOptions` (user can still find any category by typing)
  7. If payee is cleared (hidden field becomes empty): restore `#detail-categories-list` from `fullCategoryOptions` and clear category input + hidden field

- [ ] **Category ID sync**: listen to `change` and `blur` on `#detail-category-input`; find matching `<option>` in `#detail-categories-list` by value (case-insensitive); write `option.dataset.id` (or look up from `fullCategoryOptions` if list was narrowed) to `#detail-category-id`; clear hidden field if no match

- [ ] **HTMX body listener registered once** (outside `init()`): listen to `htmx:afterSettle` on `document.body`; if `event.detail.target.id === 'txn-detail-panel'`, call `init()` to rebind event listeners and refresh `fullCategoryOptions`; ensure the listener is added outside `init()` so repeated panel swaps don't stack duplicate body-level listeners

- [ ] Add `<script src="/static/js/detail-panel.js" crossorigin="anonymous"></script>` to `ui/html/index.tmpl.html` alongside the other script tags (lines 158–163)

- [ ] Write manual test checklist (in a comment at top of JS file):
  - [ ] Typing partial payee name filters options
  - [ ] Selecting payee sets hidden `#detail-payee-id` to correct ID
  - [ ] After payee select, category datalist updates with suggestions
  - [ ] Selecting a category NOT in the payee suggestions still works (type its name, confirm correct ID submitted)
  - [ ] Clearing payee restores full category list
  - [ ] Selecting category sets hidden `#detail-category-id` to correct ID
  - [ ] Form submission (Accept & Send to YNAB) sends payee ID and category ID (not names)

- [ ] No automated JS tests (project has no JS test runner); cover via manual browser check above

### Task 4: Style the new autocomplete inputs

**Files:**
- Modify: `ui/static/css/main.css`

The existing `.detail-select` styles target `<select>`. Add `.detail-autocomplete` to match.

- [ ] Add `.detail-autocomplete` class with the same rules as `.detail-select` (width, background, border, color, padding, font-size, border-radius)
- [ ] Ensure focus state (`:focus`) matches the existing `.detail-select:focus` style
- [ ] Verify no visual regression on the memo textarea or button layout below the inputs
- [ ] Manual: open detail panel and confirm inputs match surrounding UI visually

### Task 5: Verify end-to-end form submission

**Files:**
- No code changes; verification only

- [ ] Start server: `go run ./cmd/web`
- [ ] Open `http://localhost:8080/import-bank-txns`, click a transaction
- [ ] Type a partial payee name, select from dropdown, confirm category datalist updates
- [ ] Select a category that is NOT among the payee's suggestions (type its name manually), click "Accept & Send to YNAB" — verify correct category ID is submitted (not empty, not the name string)
- [ ] Click a different transaction, confirm detail panel re-initializes correctly (JS rebound after HTMX swap)
- [ ] Click "Remember Selections" — verify pattern is recorded (check DB or flash message)
- [ ] Run full test suite: `go test ./...` — all tests must pass

### Task 6: Verify acceptance criteria
- [ ] Payee field shows autocomplete dropdown when typing
- [ ] Category field shows autocomplete dropdown when typing
- [ ] Selecting a payee dynamically narrows category suggestions to payee-linked ones
- [ ] Pre-existing smart suggestion is pre-filled as text in both inputs
- [ ] Hidden ID fields carry correct IDs on form submit (not display names)
- [ ] Works after HTMX panel reload (click a second transaction)
- [ ] Run full test suite: `go test ./...`

### Task 7: [Final] Update documentation and move plan
- [ ] Update CLAUDE.md if new patterns discovered (e.g., detail panel JS module pattern)
- [ ] Move this plan to `docs/plans/completed/20260627-autocomplete-payee-category.md`

## Technical Details

**Template data struct extension** (`handlers.go`):
```go
data := struct {
    Txn            txn.Transaction
    BudgetID       string
    Payees         []ynab.Payee
    Categories     []ynab.Category
    SugPayeeID     string
    SugPayeeName   string   // NEW
    SugCategoryID  string
    SugCategoryName string  // NEW
    ActiveStatus   string
}{...}
```

**Datalist option format** (template):
```html
<option value="Amazon" data-id="abc-123-uuid"></option>
```
Text → ID mapping lives in `data-id` attribute; JS reads it to sync hidden field.

**Category suggestions fetch** (JS):
```
GET /api/category-suggestions?budget_id=...&description=...&payee_id=...
→ { "suggestions": [{ "category_id": "...", "category_name": "...", "confidence": 0.9, ... }] }
```

**Form submission fields** (unchanged from current):
- `payee` → payee ID (from hidden field `#detail-payee-id`)
- `category` → category ID (from hidden field `#detail-category-id`)

## Post-Completion

**Manual verification:**
- Test with 0 payees synced (empty state message still shows)
- Test with a transaction that has no prior pattern (no pre-fill, empty inputs, full category list)
- Test keyboard-only navigation (Tab through fields, Enter to select from datalist)
- Test in Safari (datalist support differs from Chrome/Firefox)
