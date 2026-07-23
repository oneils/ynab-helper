# Fix DB Query Performance in Detail Panel

## Overview

The transaction detail panel (`/bank-txns/{id}/detail`) fires 20+ SQLite queries per request, causing `context deadline exceeded` errors and HTTP 500s on k3s with slow persistent-volume storage. Three root causes identified:

1. **N+1 in `FetchCategoriesByBudget`** — one query per category group (15–25 in a typical YNAB budget)
2. **LIKE `%%` full table scan** in `GetCategorySuggestions` Strategy 1 — calls `FindPatternsByDescription("", 50)` which generates `LIKE '%%'`, bypassing all indexes
3. **Redundant full scan** in `GetCategorySuggestions` Strategy 1 — fetching all patterns then filtering in Go, instead of querying by index

Note: `GetSmartSuggestions` still uses a leading-wildcard `LIKE '%...%'` scan (accepted as-is for now — low priority compared to the empty-string scan).

## Context (from discovery)

- Files/components involved:
  - `internal/sqlite/ynab_store.go` — `FetchCategoriesByBudget`, `fetchCategoriesByGroup`
  - `internal/sqlite/pattern_store.go` — `FindPatternsByDescription`
  - `internal/txn/suggestions.go` — `GetCategorySuggestions` Strategy 1
- Related patterns found: `fetchAccountsForBudgets` in `ynab_store.go` already uses a correct batch-IN approach — categories should match it
- Dependencies: `PatternStorer` interface in `internal/txn/suggestions.go` must be extended with new method
- Indexes available: `idx_payee_patterns_payee` on `payee_patterns(payee_id)` — unused today
- Missing index: composite `(budget_id, payee_id)` on `payee_patterns` — needed for `FindPatternsByPayeeID` to avoid cross-budget scans

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- All tests must pass before starting the next task

## Testing Strategy

- Unit tests for every task
- No UI/e2e tests needed (pure DB/logic layer changes)

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document blockers with ⚠️ prefix

## Implementation Steps

---

### Task 1: Replace N+1 `FetchCategoriesByBudget` with a single JOIN query

**Files:**
- Modify: `internal/sqlite/ynab_store.go`
- Create: `internal/sqlite/ynab_store_test.go`

- [x] Create `setupYnabTestDB` helper in `ynab_store_test.go` that hand-rolls the `budgets`, `category_groups`, and `categories` table schema (mirror the convention in `transaction_store_test.go:14`)
- [x] Replace `FetchCategoriesByBudget` with a single SQL query:
  ```sql
  SELECT cg.id, cg.budget_id, cg.name, cg.hidden, cg.deleted,
         c.id, c.category_group_id, c.category_group_name, c.name,
         c.hidden, c.deleted, c.original_category_group_id, c.note,
         c.budgeted, c.activity, c.balance, c.goal_type, c.goal_days,
         c.goal_cadence, c.goal_cadence_frequency, c.goal_target,
         c.goal_target_month, c.goal_creation_month, c.goal_percentage_complete,
         c.goal_months_to_budget, c.goal_under_funded, c.goal_overall_funded,
         c.goal_overall_left
  FROM category_groups cg
  LEFT JOIN categories c ON c.category_group_id = cg.id AND c.deleted = 0
  WHERE cg.budget_id = ? AND cg.deleted = 0
  ORDER BY cg.id
  ```
  **Note**: `AND c.deleted = 0` intentionally excludes deleted categories — this is a behavior improvement over the old `fetchCategoriesByGroup` which had no deleted filter at category level.
- [x] Scan ALL category columns into `sql.Null*` temporaries (not plain `string`/`int`) — LEFT JOIN produces all-NULL category rows for empty groups; scanning NULL into `string` fails with modernc.org/sqlite. Only construct and append `ynab.Category` when `categoryID.Valid`.
- [x] Assemble result preserving group order via `map[string]*ynab.CategoryGroup` + `[]string` order slice
- [x] Remove `fetchCategoriesByGroup` helper (no longer needed)
- [x] Write unit test: budget with 3 groups × 3 categories each → correct structure, categories in each group
- [x] Write unit test: group with no categories → group present in result, `Categories` is nil or empty (empty-group LEFT JOIN NULL scan must not error)
- [x] Write unit test: deleted categories excluded, deleted groups excluded
- [x] Write unit test: budget with no groups → empty slice
- [x] Run tests — must pass before Task 2: `go test ./internal/sqlite/...`

---

### Task 2: Add migration + `FindPatternsByPayeeID` to `PatternStore`

**Files:**
- Create: `internal/sqlite/migrations/00004_payee_patterns_budget_payee_index.sql`
- Modify: `internal/sqlite/pattern_store.go`
- Modify: `internal/txn/suggestions.go` — `PatternStorer` interface
- Create: `internal/sqlite/pattern_store_test.go`

- [x] Create migration `00004_payee_patterns_budget_payee_index.sql`:
  ```sql
  -- +goose Up
  -- +goose StatementBegin
  CREATE INDEX idx_payee_patterns_budget_payee ON payee_patterns(budget_id, payee_id);
  -- +goose StatementEnd

  -- +goose Down
  -- +goose StatementBegin
  DROP INDEX IF EXISTS idx_payee_patterns_budget_payee;
  -- +goose StatementEnd
  ```
  This composite index lets `FindPatternsByPayeeID` find all patterns for a specific payee in a specific budget in a single index scan, avoiding cross-budget row reads.
- [x] Create `setupPatternTestDB` helper in `pattern_store_test.go` that hand-rolls the `payee_patterns` table schema (mirror `transaction_store_test.go:14` convention)
- [x] Add `FindPatternsByPayeeID(ctx context.Context, budgetID, payeeID string, limit int) ([]PayeePattern, error)` to `PatternStorer` interface in `internal/txn/suggestions.go`
- [x] Implement `FindPatternsByPayeeID` in `internal/sqlite/pattern_store.go`:
  ```sql
  SELECT id, budget_id, normalized_description,
         payee_id, payee_name, category_id, category_name,
         occurrence_count, last_seen, created_at, updated_at
  FROM payee_patterns
  WHERE budget_id = ? AND payee_id = ?
    AND category_id IS NOT NULL AND category_id != ''
  ORDER BY occurrence_count DESC, last_seen DESC
  LIMIT ?
  ```
  — `AND category_id IS NOT NULL AND category_id != ''` filters at SQL level to avoid emitting blank suggestions (mirrors the old Go-level `p.CategoryID != ""` guard)
  — uses new `idx_payee_patterns_budget_payee` composite index
- [x] Write unit test: payee with 3 patterns returns them sorted by occurrence_count DESC
- [x] Write unit test: patterns with NULL category_id excluded from results
- [x] Write unit test: unknown payeeID returns empty slice (no error)
- [x] Write unit test: limit is respected
- [x] Run tests — must pass before Task 3: `go test ./internal/sqlite/... ./internal/txn/...`

---

### Task 3: Update `GetCategorySuggestions` Strategy 1 to use `FindPatternsByPayeeID`

**Files:**
- Modify: `internal/txn/suggestions.go`
- Create: `internal/txn/suggestions_test.go`

- [x] In `GetCategorySuggestions`, replace Strategy 1 block:
  ```go
  // Before
  patterns, err = e.patternStore.FindPatternsByDescription(ctx, budgetID, "", 50)
  // ... then filter by payeeID in Go

  // After
  patterns, err = e.patternStore.FindPatternsByPayeeID(ctx, budgetID, payeeID, 50)
  // category_id filter already applied in SQL by FindPatternsByPayeeID
  ```
- [x] Remove the entire in-memory filter loop (`for _, p := range patterns { if p.PayeeID == payeeID && p.CategoryID != "" ... }`) — both conditions are now handled in SQL
- [x] Create a mock `PatternStorer` in `suggestions_test.go` implementing all interface methods including new `FindPatternsByPayeeID`
- [x] Write unit test: `GetCategorySuggestions` with payeeID → calls `FindPatternsByPayeeID`, does NOT call `FindPatternsByDescription`
- [x] Write unit test: `GetCategorySuggestions` with empty payeeID → skips Strategy 1, falls back to Strategy 2 (description LIKE scan via `FindPatternsByDescription`)
- [x] Write unit test: `GetCategorySuggestions` returns empty when both strategies yield no patterns
- [x] Run tests — must pass before Task 4: `go test ./internal/txn/...`

---

### Task 4: Verify acceptance criteria

- [x] Run full test suite: `go test ./...`
- [x] Confirm `detailBankTxnHandler` path now makes ≤ 5 DB queries (manual log-count check):
  - 1 — `FetchByID`
  - 1 — `FindBudgetByAccountID` (budget + accounts still 2 queries, acceptable)
  - 1 — `FetchPayeesByBudget`
  - 1 — `FetchCategoriesByBudget` (was 16–26, now 1)
  - 1 — `GetSmartSuggestions` → `FindPatternsByDescription`
  - 1 — `GetCategorySuggestions` → `FindPatternsByPayeeID` or description fallback
  - Verified via code trace (internal/server/handlers.go:499 detailBankTxnHandler): call chain matches expected 1-query-per-step pattern, confirmed by explore agent trace
- [x] Build binary: `go build ./...`

---

### Task 5: [Final] Cleanup and move plan

- [x] Move this plan to `docs/plans/completed/`

---

## Technical Details

**N+1 fix assembly pattern (Task 1):**
```go
groupMap := make(map[string]*ynab.CategoryGroup)
var groupOrder []string

for rows.Next() {
    // Group fields — plain types (never NULL, group always exists)
    var cgID, cgBudgetID, cgName string
    var cgHidden, cgDeleted int

    // Category fields — ALL nullable (NULL when group has no categories)
    var catID, catGroupID, catGroupName, catName sql.NullString
    var catHidden, catDeleted sql.NullInt64
    // ... all ~20 category columns as sql.Null* types

    err := rows.Scan(&cgID, &cgBudgetID, &cgName, &cgHidden, &cgDeleted,
        &catID, &catGroupID, &catGroupName, &catName,
        &catHidden, &catDeleted, /* ... rest of category nullables */)

    if _, seen := groupMap[cgID]; !seen {
        g := ynab.CategoryGroup{ID: cgID, BudgetID: cgBudgetID, Name: cgName, ...}
        groupMap[cgID] = &g
        groupOrder = append(groupOrder, cgID)
    }
    if catID.Valid { // only non-NULL rows represent real categories
        cat := ynab.Category{ID: catID.String, Name: catName.String, ...}
        groupMap[cgID].Categories = append(groupMap[cgID].Categories, cat)
    }
}

// preserve order
result := make([]ynab.CategoryGroup, 0, len(groupOrder))
for _, id := range groupOrder {
    result = append(result, *groupMap[id])
}
```

**Index added by `FindPatternsByPayeeID` (migration 00004):**
```sql
-- new composite index — covers WHERE budget_id=? AND payee_id=? in one scan
CREATE INDEX idx_payee_patterns_budget_payee ON payee_patterns(budget_id, payee_id);
```
Why composite and not just `payee_id`? The existing `idx_payee_patterns_payee` only narrows by payee — SQLite then filters by `budget_id` in memory, reading all rows for that payee across all budgets. With `(budget_id, payee_id)`, the index covers both conditions and returns only the relevant rows directly.

## Post-Completion

**Manual verification on k3s:**
- Deploy and open transaction detail panel — confirm no 500, response in < 1s
- Check pod logs — no more `context deadline exceeded` errors
- Verify category dropdown in detail panel still shows correct groups/categories
