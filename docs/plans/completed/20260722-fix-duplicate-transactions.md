# Fix Duplicate Imported Transactions

## Overview

Bank CSV parsers compute the transaction ID by hashing the entire raw CSV row
(`strings.Join(row, ",")`). Some columns are not stable across different exports
of the same transaction:

- **Millennium column 9 (Saldo)**: running account balance; changes when subsequent
  transactions are added, reversed, or adjusted between export runs
- **Millennium column 2 (Data rozliczenia)**: settlement date; may differ while a
  transaction is still pending (set to today) vs. after it settles (set to actual date)
- **Santander columns 7–8**: appear to contain balance and a page/sequence counter that
  changes between exports

Millennium column 0 (Numer) is kept in the hash because it is likely a stable
bank-assigned transaction ID; including it reduces collision risk (e.g. two genuine
PETSTATION purchases for the same amount on the same day). If duplicates persist after
this fix, Numer may need to be excluded too (see Task 3 verification step).

Result: re-importing overlapping date ranges or re-exporting after pending transactions
settle stores the same real-world transaction twice with different IDs, bypassing
`INSERT OR IGNORE`.

**Secondary issue**: Santander and Millennium parsers do not store `RawText` on valid
transactions (PKO and Revolut do), making forensic debugging impossible.

This plan fixes the ID computation to use only stable columns and adds `RawText`
storage for the two affected parsers. Existing duplicates in the DB are left as-is
(there are 4 pairs; user will skip them manually).

## Context (from discovery)

- Parsers: `internal/parser/santander.go`, `internal/parser/millenium.go`,
  `internal/parser/pko.go`, `internal/parser/revolut.go`
- Parser config: `internal/parser/parser.go` (`Config` struct), `internal/app/app.go`
  (config functions)
- ID hash computed at: `santander.go:95-109`, `millenium.go:90-104`,
  `pko.go:111-125`, `revolut.go:106-120`
- `RawText` missing from valid txn append: `santander.go:111-122`, `millenium.go:106-117`
- Confirmed duplicates in `data/ynab.db`: 4 pairs (3 Millennium, 1 Santander)
- Test files: `internal/parser/santander_test.go`,
  `internal/parser/santander_integration_test.go`, `internal/parser/millenium_test.go`
- Parser config helpers: `internal/app/app.go:105-163`

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- Every task that changes behavior MUST include tests before moving on
- All tests must pass before starting the next task

## Testing Strategy

- Unit tests for the `buildHashInput` helper (parser_test.go)
- Update existing parser tests: confirm IDs are unchanged for PKO/Revolut (no
  `HashColumns` set → all-columns behavior preserved)
- New tests for Santander and Millennium: verify that two rows differing only in
  excluded columns (Saldo, Numer, settlement date) produce the **same** ID
- New tests: verify that two rows differing in included stable columns produce
  **different** IDs
- `RawText` tests: verify `raw_text` is populated on valid transactions
- No e2e tests (project has none)

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Add `HashColumns` to `Config` and `buildHashInput` helper

**Files:**
- Modify: `internal/parser/parser.go`

The `Config` struct gains a `HashColumns []int` field. When nil/empty (PKO, Revolut),
all columns are joined — preserving existing behavior. When set, only those column
indices are joined.

- [x] Add `HashColumns []int` field to `Config` struct in `parser.go`
- [x] Add `buildHashInput(cfg Config, row []string) string` helper:
  - if `len(cfg.HashColumns) == 0`: return `strings.Join(row, ",")`
  - otherwise: build a slice of `row[idx]` for each idx in `cfg.HashColumns` and
    return `strings.Join(parts, ",")`
- [x] Write unit tests for `buildHashInput` in `internal/parser/parser_test.go`:
  - empty `HashColumns` → same as `strings.Join`
  - non-empty `HashColumns` → only selected columns included
  - all indices valid (rows are pre-validated by `validColumnsAmount` before reaching
    this function, so out-of-bounds means a misconfigured `HashColumns`; panic or
    index-out-of-range is acceptable and preferred over silent skip — no defensive
    guard needed)
- [x] Run tests: `go test ./internal/parser/...` — must pass

### Task 2: Update Santander parser (stable hash + RawText)

**Files:**
- Modify: `internal/parser/santander.go`
- Modify: `internal/app/app.go`
- Modify: `internal/parser/santander_test.go`
- Modify: `internal/parser/santander_integration_test.go`

Santander CSV columns (`ColumnsAmount: 9`):
- 0: empty field / sequence (exclude)
- 1: date (include)
- 2: description/title (include)
- 3: recipient name for transfers (include)
- 4: recipient account number (include)
- 5: amount (include)
- 6: empty (exclude)
- 7: balance (exclude)
- 8: page/counter (exclude)

Hash will use `HashColumns: []int{1, 2, 3, 4, 5}`.

- [x] In `santander.go:96`, change the argument to `p.hasher.Write` from
  `[]byte(rowString)` to `[]byte(buildHashInput(p.cfg, row))` — target the
  `hasher.Write` call only; do NOT change the `rowString := strings.Join(row, ",")`
  assignment at line 47, which must remain the full row for `RawText` and error
  diagnostics
- [x] Add `RawText: rowString` to the valid transaction append at `santander.go:111-122`
- [x] Add `HashColumns: []int{1, 2, 3, 4, 5}` to `santanderConfig()` in `app.go`
- [x] Update tests in `santander_test.go` and `santander_integration_test.go`:
  - Add test: two rows identical in columns 1–5 but differing in columns 7–8 → **same
    ID** (dedup works)
  - Add test: two rows differing in column 2 (description) → **different IDs** (no
    false dedup)
  - Add test: valid transaction has non-empty `RawText`
- [x] Run tests: `go test ./internal/parser/... ./internal/app/...` — must pass

### Task 3: Update Millennium parser (stable hash + RawText)

**Files:**
- Modify: `internal/parser/millenium.go`
- Modify: `internal/app/app.go`
- Modify: `internal/parser/millenium_test.go`

Millennium CSV columns (`ColumnsAmount: 11`):
- 0: Numer — stable bank-assigned ID (include; reduces collision risk for genuine
  same-day same-amount transactions)
- 1: Data transakcji (transaction date) — stable (include)
- 2: Data rozliczenia (settlement date) — mutable for pending txns (exclude)
- 3: Rodzaj (type) — stable (include)
- 4: Na konto (to account) — stable (include)
- 5: Odbiorca (recipient) — stable (include)
- 6: Opis (description) — stable (include)
- 7: Obciążenia (debit) — stable (include)
- 8: Uznania (credit) — stable (include)
- 9: Saldo (balance) — changes between exports (exclude)
- 10: Waluta (currency) — stable (include)

Hash will use `HashColumns: []int{0, 1, 3, 4, 5, 6, 7, 8, 10}`.

- [x] In `millenium.go:91`, change the argument to `p.hasher.Write` from
  `[]byte(rowString)` to `[]byte(buildHashInput(p.cfg, row))` — target the
  `hasher.Write` call only; do NOT change the `rowString := strings.Join(row, ",")`
  assignment at line 44, which must remain the full row for `RawText` and error
  diagnostics
- [x] Add `RawText: rowString` to the valid transaction append at `millenium.go:106-117`
- [x] Add `HashColumns: []int{0, 1, 3, 4, 5, 6, 7, 8, 10}` to `milleniumConfig()` in
  `app.go`
- [x] Update tests in `millenium_test.go`:
  - Add test: two rows identical in columns 0, 1, 3–8, 10 but with different settlement
    date (col 2) and Saldo (col 9) → **same ID** (dedup works)
  - Add test: two rows differing in description (col 6) or amount (col 7/8) → **different
    IDs** (no false dedup)
  - Add test: two rows with different Numer (col 0) and same everything else → **different
    IDs** (Numer contributes to uniqueness)
  - Add test: valid transaction has non-empty `RawText`
- [x] ⚠️ After real-world verification: if duplicates still appear in Millennium imports,
  Numer (col 0) may also be unstable → remove it from `HashColumns` and update the
  collision-risk note (manual test - skipped, not automatable; noted for future follow-up)
- [x] Run tests: `go test ./internal/parser/... ./internal/app/...` — must pass

### Task 4: Verify acceptance criteria

- [x] Verify that re-importing an overlapping Millennium CSV no longer creates duplicate
  rows: parse the same CSV twice and confirm only one row per transaction in the store
  (manual test - skipped, not automatable; covered by unit tests confirming stable
  hash IDs for rows differing only in excluded columns)
- [x] Verify that Santander re-import behaves the same (manual test - skipped, not
  automatable; covered by unit tests confirming stable hash IDs)
- [x] Verify PKO and Revolut IDs are unchanged (no `HashColumns` set → behavior same
  as before) — confirmed via code inspection: `pkoConfig()`/`revolutConfig()` in
  `app.go` set no `HashColumns`, so `buildHashInput` falls back to `strings.Join(row, ",")`
- [x] Run full test suite: `go test ./...` — all tests pass
- [x] Run `go vet ./...` — no warnings
- [x] Move this plan to `docs/plans/completed/`

## Technical Details

**`buildHashInput` signature:**
```go
func buildHashInput(cfg Config, row []string) string {
    if len(cfg.HashColumns) == 0 {
        return strings.Join(row, ",")
    }
    parts := make([]string, 0, len(cfg.HashColumns))
    for _, idx := range cfg.HashColumns {
        parts = append(parts, row[idx]) // panics on bad config — intentional
    }
    return strings.Join(parts, ",")
}
```

No bounds check: rows reaching this function have already passed `validColumnsAmount`,
so an out-of-bounds index means a misconfigured `HashColumns` — a loud panic is
preferable to silently producing weaker or colliding hashes.

**Why these Santander columns (1–5)?**
Real Santander exports show: card payments populate only columns 1, 2, 5; BLIK transfers
also populate 3, 4. Columns 7 (balance "1000,00") and 8 (counter "29") appear in BLIK
rows and change between exports. Column 0 is always empty. Column 6 is always empty.
Including columns 1–5 covers all uniquely identifying stable fields.

**Why these Millennium columns (0, 1, 3–8, 10)?**
- Col 0 (Numer): kept — assumed stable bank-assigned ID; reduces collision risk
  (two genuine same-day same-amount transactions get different Numer values)
- Col 2 (Data rozliczenia): excluded — settlement date changes while a transaction
  is pending (set to today) then updates to the actual settlement date
- Col 9 (Saldo): excluded — running balance changes as subsequent transactions
  are added, reversed, or adjusted between export runs
- Remaining columns (0, 1, 3, 4, 5, 6, 7, 8, 10) represent the transaction's
  stable semantic identity: bank ID, date, type, who, description, amounts, currency

**Collision risk acknowledgment:**
If two genuinely different Millennium transactions on the same day share the same type,
recipient, description, and amount (e.g. two identical café purchases in the same
minute that the bank gave different Numer values to), the second would be deduplicated
away. This is unusual. If Numer turns out to be unstable (export-relative), excluding
it increases the collision surface; a follow-up fix would be needed (noted as ⚠️ in
Task 3).

Santander has no equivalent stable per-row ID to Millennium's Numer, so it carries
the same collision risk without that mitigation: two genuinely different Santander
transactions on the same day with identical description, recipient, and amount would
hash to the same ID and be deduplicated away. Columns 7–8 (balance, counter) were
excluded specifically because they're unstable across exports, but they were also the
only fields that would have distinguished such transactions. This is accepted as an
unlikely edge case, same as the Millennium one above.

## Post-Completion

**Manual verification:**
- Manually check that the 4 existing duplicate pairs in the DB are visible as pairs
  (they remain DRAFT/PROCESSED). User can skip the extra DRAFT rows manually.
- Re-import a real Millennium CSV that was previously imported to confirm no new
  duplicates are created.
- Re-import a real Santander CSV for the same confirmation.
