# Account-to-Parser Mapping UI

## Overview

Replace the fragile account-name substring match with an explicit, persistent mapping between YNAB accounts and parsers. Users configure the mapping in the Settings page via an auto-saving dropdown per account. The name-substring fallback is removed entirely; processing fails clearly when no mapping is configured.

Problem solved: accounts named "My Checking" or "Savings" no longer silently fall through with a cryptic "no parser found" error at upload time.

## Context (from discovery)

- Parser registry: `internal/app/app.go:56-61` — `map[string]txn.ReportParser` keyed by `SantanderBankName`, `RevolutBankName`, `PKOBankName`, `MilleniumBankName`
- Parser lookup to replace: `internal/txn/processor.go:132-139` (substring loop, same block in both `Process` and `Preview`)
- Store pattern to follow: `internal/sqlite/pattern_store.go` — separate struct, own file, passed through `DB`
- Migration system: goose embedded SQL files in `internal/sqlite/migrations/`, latest is `00002`
- Settings page: `ui/html/pages/ynab-settings.tmpl.html` with partials in `ui/html/partials/`
- Routes: `internal/server/server.go`, handlers in `internal/server/handlers.go`
- Processor tests use inline mock structs (not moq): `internal/txn/processor_test.go`

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- Every task that changes behaviour MUST include tests before moving on
- All tests must pass before starting the next task

## Testing Strategy

- Unit tests for store methods (`GetParserMapping`, `SaveParserMapping`) using an in-memory SQLite DB — same pattern as `internal/sqlite/transaction_store_test.go`
- Unit tests for `Processor.Process` and `Processor.Preview` using a mock `ParserMappingLookup` — extend `internal/txn/processor_test.go`
- No e2e tests (project has none)

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Add SQLite migration for `account_parser_mappings`

**Files:**
- Create: `internal/sqlite/migrations/00003_account_parser_mappings.sql`

- [x] Create migration file with goose up/down directives
- [x] Table: `account_parser_mappings(account_id TEXT PK, parser_name TEXT NOT NULL, updated_at TEXT NOT NULL)` — `parser_name` allows empty string (means "not set"); NOT NULL prevents NULLs
- [x] Migration verified when Task 2 store tests pass (`go test ./internal/sqlite/...`)

### Task 2: Implement `ParserMappingStore`

**Files:**
- Create: `internal/sqlite/parser_mapping_store.go`
- Create: `internal/sqlite/parser_mapping_store_test.go`
- Modify: `internal/sqlite/sqlite.go`

- [x] Create `ParserMappingStore` struct with `*sql.DB` field, follow `pattern_store.go` structure
- [x] Implement `GetParserMapping(ctx, accountID string) (string, error)` — returns `("", nil)` for both missing row AND stored empty string; treats both as "not mapped"
- [x] Implement `SaveParserMapping(ctx, accountID, parserName string) error` — upsert with `updated_at = now`; passing empty string stores `""` which is treated identically to missing (effectively clears the mapping — no separate delete method needed)
- [x] Add `parserMappingStore *ParserMappingStore` field to `DB` struct in `sqlite.go`
- [x] Initialise it in `New()` alongside the other stores
- [x] Add `ParserMappingStore() *ParserMappingStore` accessor method on `DB`
- [x] In test setup, create the `account_parser_mappings` table with an inline `CREATE TABLE` statement — same approach as `setupTestDB` in `transaction_store_test.go` (goose migrations are NOT run in tests; the inline schema must match the migration DDL exactly)
- [x] Write tests: `GetParserMapping` — missing row returns `("", nil)`, stored name returns it, stored `""` returns `("", nil)`
- [x] Write tests: `SaveParserMapping` — insert, upsert (second save with new name overwrites), save `""` then `GetParserMapping` returns `("", nil)`
- [x] Run tests: `go test ./internal/sqlite/...` — must pass

### Task 3: Add `ParserMappingLookup` interface and wire into Processor

**Files:**
- Modify: `internal/txn/processor.go`
- Modify: `internal/txn/processor_test.go`
- Modify: `internal/app/app.go`

- [x] Add `ParserMappingLookup` interface to `processor.go`:
  ```go
  type ParserMappingLookup interface {
      GetParserMapping(ctx context.Context, accountID string) (string, error)
  }
  ```
- [x] Add `mappingStore ParserMappingLookup` field to `Processor` struct
- [x] Update `NewProcessor` signature: add `mappingStore ParserMappingLookup` parameter
- [x] Grep for all `txn.NewProcessor(` call sites (`grep -rn "NewProcessor(" .`) and list them before touching the signature — update every caller
- [x] Add `ParserMappingLookup` interface and `mappingStore` field; update `NewProcessor` signature
- [x] Replace the name-substring loop in `Process` (lines ~132–139):
  - Call `GetParserMapping`; propagate DB errors as Go `error`
  - Return `fmt.Errorf("no parser mapped for account [%s] — configure it in Settings > Parser Mappings", accName)` when `parserName == ""`
  - Return `fmt.Errorf("parser %q is not registered", parserName)` if `p.parsers[parserName]` is nil
- [x] Replace the name-substring loop in `Preview` (different return contract from `Process`):
  - Call `GetParserMapping`; propagate DB errors as Go `error` (infrastructure failure)
  - When `parserName == ""`: return `&PreviewResult{ValidationErrors: [fmt.Sprintf("no parser mapped for account [%s]...", accName)]}, nil` — NOT a Go error
  - When `p.parsers[parserName]` is nil (stale/unknown parser name): return `&PreviewResult{ValidationErrors: [fmt.Sprintf("parser %q is not registered", parserName)]}, nil` — NOT a Go error
- [x] Remove `strings` import from `processor.go` if now unused (still used by `SuggestPayee`, kept)
- [x] Update `internal/app/app.go`: pass `db.ParserMappingStore()` as the new argument to `txn.NewProcessor`
- [x] Add `mockParserMappingLookup` struct to `processor_test.go`
- [x] Update all existing tests that call `NewProcessor(...)` to pass the mock
- [x] Add test: `Process` returns error when mapping not found
- [x] Add test: `Process` succeeds when mapping exists and parser is registered
- [x] Add test: `Preview` returns `PreviewResult` with `ValidationErrors` (nil Go error) when mapping not found
- [x] Add test: `Preview` returns `PreviewResult` with `ValidationErrors` (nil Go error) when parser name is unknown
- [x] Run tests: `go test ./internal/txn/... ./internal/app/...` — must pass

### Task 4: Add server handlers for parser mapping UI

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/handlers.go`

- [x] No new field on `Server` needed — `Server` already holds `DB *sqlite.DB` (accessed via `s.DB`); call `s.DB.ParserMappingStore()` directly in handlers (consistent with `s.DB.Ping(...)` already in use)
- [x] Add handler `parserMappingsHandler` (GET `/settings/parser-mappings?budget=<id>`):
  - Fetch budget via `s.Syncer.FindBudgetByID` to get its accounts
  - For each account, call `s.DB.ParserMappingStore().GetParserMapping(ctx, acc.ID)`
  - Build a view-model slice of `{Account ynab.Account, ParserName string}`
  - Render the `parser-mapping-rows` template with the slice + available parser names list
- [x] Add handler `saveParserMappingHandler` (POST `/settings/parser-mappings/{accountID}`):
  - `chi.URLParam(r, "accountID")`
  - Read `parser_name` from `r.FormValue("parser_name")` (empty string = clear mapping)
  - Call `s.DB.ParserMappingStore().SaveParserMapping(ctx, accountID, parserName)`
  - Respond with `HX-Trigger: {"showToast": {"message": "Parser mapping saved", "type": "success"}}` and `hx-swap="none"` (no body needed since dropdown is already updated client-side)
- [x] Register both routes in `server.go` routes():
  - `r.Get("/settings/parser-mappings", s.parserMappingsHandler)`
  - `r.Post("/settings/parser-mappings/{accountID}", s.saveParserMappingHandler)`
- [x] Run: `go build ./...` — must compile

### Task 5: Add templates

**Files:**
- Create: `ui/html/partials/parser-mappings.tmpl.html`
- Modify: `ui/html/pages/ynab-settings.tmpl.html`

- [x] Create `ui/html/partials/parser-mappings.tmpl.html` with two named templates:
  - `{{define "parser-mappings-section"}}` — section with a **dedicated** budget `<select>` (NOT the existing `#global-budget` which is already bound to sync-history); this selector has `hx-get="/settings/parser-mappings"` and `hx-target="#parser-mapping-rows"`
  - `{{define "parser-mapping-rows"}}` — `<tbody id="parser-mapping-rows">` rows, one per account: account name + parser `<select>` with `hx-post="/settings/parser-mappings/{{.AccountID}}"`, `hx-trigger="change"`, `hx-swap="none"`
  - Dropdown options: `<option value="">— not set —</option>` then Santander, Revolut, PKO, Millennium
  - Pre-select the current `ParserName` value using `{{if eq .ParserName "Santander"}}selected{{end}}`
- [x] Add "Parser Mappings" section to `ui/html/pages/ynab-settings.tmpl.html`:
  - Pass budgets via template data (already present on settings page via `settingsViewHandler`)
  - Render `{{template "parser-mappings-section" .}}`
- [x] Verify template cache picks up the new partial (it globs `html/partials/*.tmpl.html` — confirmed in `handlers.go:1447`)
- [x] manual smoke-test (skipped - not automatable): start app, open Settings, select a budget in the parser-mappings selector, verify account rows render with dropdowns, change a dropdown, verify toast appears and DB is updated

### Task 6: Verify acceptance criteria

- [x] manual test (skipped - not automatable): Upload a CSV for an account that has a mapping → processes successfully
- [x] manual test (skipped - not automatable): Upload a CSV for an account with no mapping → shows clear error "no parser mapped for account [X] — configure it in Settings > Parser Mappings"
- [x] manual test (skipped - not automatable): Set mapping in Settings, change it, verify the new parser is used on next upload
- [x] Run full test suite: `go test ./...` — all tests pass
- [x] Run `go vet ./...` — no warnings

### Task 7: Finalise

**Files:**
- Move: `docs/plans/20260721-account-parser-mapping.md` → `docs/plans/completed/`

- [x] Move this plan to `docs/plans/completed/`

## Technical Details

**Parser name constants** (used as dropdown values and DB storage):
- `parser.SantanderBankName = "Santander"`
- `parser.RevolutBankName = "Revolut"`
- `parser.PKOBankName = "PKO"`
- `parser.MilleniumBankName = "Millennium"`

**`SaveParserMapping` upsert SQL**:
```sql
INSERT INTO account_parser_mappings (account_id, parser_name, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(account_id) DO UPDATE SET parser_name = excluded.parser_name, updated_at = excluded.updated_at
```

**"Not set" semantics**: selecting "— not set —" submits `parser_name=""`. `SaveParserMapping` stores `""`. `GetParserMapping` returns `("", nil)` for both missing rows and stored `""` — both are treated identically as "not mapped" by the processor. No separate delete method needed.

**No new `Server` field needed**: `Server` already holds `DB *sqlite.DB`; handlers call `s.DB.ParserMappingStore()` directly (consistent with existing `s.DB.Ping(...)` usage in `readinessCheckHandler`).

**`Process` vs `Preview` return contract for no-mapping**:
- `Process`: returns `(error)` — blocks upload
- `Preview`: returns `(&PreviewResult{ValidationErrors: [...]}, nil)` — renders a validation error in the preview UI, same as the existing no-parser-found path

**HTMX pattern for auto-save row**:
```html
<select name="parser_name"
        hx-post="/settings/parser-mappings/{{.AccountID}}"
        hx-trigger="change"
        hx-swap="none">
  <option value="">— not set —</option>
  <option value="Santander" {{if eq .ParserName "Santander"}}selected{{end}}>Santander</option>
  ...
</select>
```

## Post-Completion

**Manual verification**:
- Confirm existing accounts with names like "PKO Savings" no longer auto-detect — they now need an explicit mapping
- After adding the mapping in Settings, verify upload works end-to-end (parse → preview → confirm → YNAB)
