# Payee & Category Prefill from YNAB (Restore Lost Functionality)

## Overview

When a bank transaction is parsed and shown on the import page, the tool should:
1. Match the transaction description against YNAB payee names (direct string matching)
2. If a payee is matched, prefill the Payee field
3. If that payee has a `LastCategoryID` (the last category used for it in YNAB), prefill the Category field too

This mirrors YNAB's own behavior and was previously working. The current codebase has a pattern-based suggestion engine (`GetSmartSuggestions`) that learns from user choices — still good, but it only works after the user has approved transactions before. The YNAB name-match is the fallback when no learned patterns exist yet.

**Approach: Fallback only.** Pattern-based suggestions win when available; YNAB name matching activates only when patterns return no results.

**Scope:**
- Transaction list (after import) — payee prefill only (no category, categories are not in scope there)
- Detail panel (when clicked) — payee prefill + category prefill from `LastCategoryID`

## Context (from discovery)

- `internal/server/handlers.go` — `enrichTransactionList` (line 266), `detailBankTxnHandler` (line 323)
  - 6 call sites of `enrichTransactionList`: ~lines 127, 227, 461, 570, 1044, 1128
- `internal/server/handlers_test.go` — existing `TestEnrichTransactionList_*` tests
- `internal/txn/processor.go` — `SuggestPayee(t Transaction, payees []ynab.Payee) ynab.Payee` (line 317); does substring match on `t.Payee` (falling back to `t.Description` when empty); this is the single matcher to use in both paths
- `internal/ynab/ynab.go` — `Payee.LastCategoryID` (line 92), `Payee.Deleted` (line 90)
- `internal/ynab/sync.go` — `Syncer.FetchPayeesByBudget` (line 244) returns `[]ynab.Payee` sorted by name
- `internal/txn/suggestions.go` — `normalize` is unexported; do NOT reference it from package `server`

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- All tests must pass before starting the next task
- Run `go test ./...` after each task

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add ➕ for newly discovered tasks
- Add ⚠️ for blockers

## Implementation Steps

### Task 1: Add YNAB payee name fallback to `enrichTransactionList`

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/handlers_test.go`

The function currently receives collaborators as injected function params. We follow the same pattern — no access to `s.*` inside the function.

- [ ] Add two new parameters to `enrichTransactionList`:
  - `getPayeesByBudget func(context.Context, string) ([]ynab.Payee, error)`
  - `suggestPayee func(txn.Transaction, []ynab.Payee) ynab.Payee`
- [ ] Add a `payeeCache map[string][]ynab.Payee` inside the loop (same pattern as `budgetCache`)
- [ ] When `payeeSuggestions` is empty after calling `getSuggestions`:
  - Fetch/cache payees for the budget
  - Call `suggestPayee(t, payees)`; guard against empty `payee.Name` (`payee.Name == ""` skips silently; `SuggestPayee` already handles `Deleted` payees only by name match, which is acceptable)
  - If result has a non-empty `ID`: set `rows[i].SugPayee = result.Name`, `rows[i].AutoFilled = true`
  - Do NOT attempt to prefill category from `LastCategoryID` here — categories are not in scope; the detail panel handles it
- [ ] Update all 6 callers of `enrichTransactionList` (lines ~127, 227, 461, 570, 1044, 1128) to pass:
  - `s.Syncer.FetchPayeesByBudget`
  - `s.TxnProcessor.SuggestPayee`
- [ ] Update all existing `TestEnrichTransactionList_*` tests: add the two new params, passing `nil`-returning stubs to preserve existing behavior
- [ ] Add `TestEnrichTransactionList_FallbackPayeeMatch` — no pattern match, `suggestPayee` returns a payee, `SugPayee` set + `AutoFilled = true`
- [ ] Add `TestEnrichTransactionList_FallbackNoMatch` — no pattern match, `suggestPayee` returns zero value (empty ID), row has no suggestions
- [ ] Add `TestEnrichTransactionList_FallbackSkippedWhenPatternExists` — `getSuggestions` returns a result, `getPayeesByBudget` is never called
- [ ] Add `TestEnrichTransactionList_FallbackEmptyPayeeName` — `suggestPayee` returns a payee with empty `Name`/`ID`, no prefill set
- [ ] Run `go test ./internal/server/...` — must pass before Task 2

### Task 2: Add YNAB payee name fallback to `detailBankTxnHandler`

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/handlers_test.go`

The handler already fetches `payees []ynab.Payee` and `categories []ynab.Category` before computing suggestions. Use the same `SuggestPayee` method for consistency.

- [ ] After `GetSmartSuggestions` returns empty (`len(payeeSugs) == 0`):
  - Call `s.TxnProcessor.SuggestPayee(transaction, payees)` to get fallback payee
  - If `fallbackPayee.ID != ""`: set `sugPayeeID = fallbackPayee.ID` and keep a reference to `fallbackPayee`
- [ ] After `GetCategorySuggestions` returns empty (`len(catSugs) == 0`):
  - If fallback payee was found and `fallbackPayee.LastCategoryID != ""`: set `sugCatID = fallbackPayee.LastCategoryID`
  - `SugCategoryName` resolves automatically via the existing `categoryNameByID(categories, sugCatID)` call (categories are in scope)
- [ ] Extract the above fallback decisions into a small unexported helper `applyYnabPayeeFallback(t txn.Transaction, payees []ynab.Payee, suggestFn func(txn.Transaction, []ynab.Payee) ynab.Payee, patternPayeeID, patternCatID string) (payeeID, catID string)` — pure function, easy to unit test
- [ ] Add `TestApplyYnabPayeeFallback_PayeeAndCategoryFromLastCategoryID` — no pattern results, fallback finds payee with LastCategoryID, both IDs returned
- [ ] Add `TestApplyYnabPayeeFallback_NoFallbackWhenPatternExists` — pattern payeeID is non-empty, returns it unchanged, no fallback triggered
- [ ] Add `TestApplyYnabPayeeFallback_CategoryEmptyWhenLastCategoryIDMissing` — fallback payee found but `LastCategoryID == ""`, catID returned empty
- [ ] Run `go test ./internal/server/...` — must pass before Task 3

### Task 3: Verify acceptance criteria

- [ ] Verify list-view shows payee prefill (AutoFilled badge) for a transaction that matches a YNAB payee name with no learned patterns
- [ ] Verify list-view does NOT prefill category (not in scope for list)
- [ ] Verify detail panel shows both payee and category prefilled when fallback fires
- [ ] Verify when pattern-based suggestions exist, fallback is NOT triggered in either path
- [ ] Run full test suite: `go test ./...` — all must pass

### Task 4: [Final] Move plan to completed

- [ ] Move this plan to `docs/plans/completed/`

## Technical Details

**Matching routine:** `s.TxnProcessor.SuggestPayee(t, payees)` is used for both paths. It checks `t.Payee` first; if empty it matches against `t.Description`. Using the same function for both list and detail ensures consistent prefill — a user who sees a payee in the list will see the same payee when they click into the detail panel.

**Why no category in the list view:** `enrichTransactionList` has no `categories` slice in scope (it would require injecting yet another lookup function). The list-view category column already works when pattern-based suggestions exist. The detail panel is the authoritative place where users see and confirm the category selection.

**`SuggestPayee` edge cases:** The function does substring matching — a payee with an empty name would match everything (`strings.Contains(x, "") == true`). Guard this with `if result.ID == ""` (which `SuggestPayee` already returns on no-match). The synced payee set from `FetchPayeesByBudget` may include deleted payees — this is existing behavior and acceptable.

**6 callers of `enrichTransactionList`** (all in `handlers.go`, confirm with grep before editing):
- `importBankTxnsHandler` (~line 127)
- `bankTxnsHandler` (~line 227)
- `skipBankTxnHandler` (~line 461)
- `detailBankTxnHandler` (~line 570) — if enrichment is used there too
- (~line 1044)
- (~line 1128)

## Post-Completion

**Manual verification:**
- Import a fresh CSV with no learned patterns → verify payee is prefilled in the list if payee name matches YNAB payees; no category in list
- Click one of those transactions → verify detail panel shows same payee + category from `LastCategoryID`
- Import a CSV for a payee with learned patterns → verify pattern-based suggestion wins in both views (fallback not triggered)
