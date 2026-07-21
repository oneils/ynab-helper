# Millenium Bank Parser

## Overview

Add a `MilleniumParser` that parses CSV exports from Bank Millennium (Polish bank). The CSV has a two-column amount structure (separate debit/credit columns) which differs from existing parsers. The parser must be wired into the app's parser registry so any YNAB account named "Millennium" is matched automatically.

## Context (from discovery)

**CSV format** (`millenium-Historia_transakcji_20260701_135052.csv`):
- Encoding: UTF-8 with BOM (affects only `data[0][0]` — the header row, which is skipped)
- Delimiter: comma
- 11 columns (0-indexed):

| Index | Name                   | Used for          |
|-------|------------------------|-------------------|
| 0     | Numer rachunku/karty   | (ignored)         |
| 1     | Data transakcji        | Transaction date  |
| 2     | Data rozliczenia       | (ignored)         |
| 3     | Rodzaj transakcji      | (ignored)         |
| 4     | Na konto/Z konta       | (ignored)         |
| 5     | Odbiorca/Zleceniodawca | (ignored)         |
| 6     | Opis                   | Payee + Description |
| 7     | Obciążenia             | Debit amount (negative, e.g. `-23.00`) |
| 8     | Uznania                | Credit amount (positive, e.g. `100.00`) |
| 9     | Saldo                  | (ignored)         |
| 10    | Waluta                 | Currency (`PLN`)  |

- Date format: `2006-01-02` (ISO 8601, same as PKO)
- Amount: exactly one of col 7 or col 8 is filled per row; the other is empty `""`
- **Debit amounts are already pre-signed negative** (e.g. `-16.55`) — verified from real fixture. Do NOT negate them in code.
- For `ZAKUP - FIZ. UŻYCIE KARTY` (physical card purchase) rows, Opis follows the pattern `MERCHANT NAME  CITY POL DATE` — verbose but used as-is
- **Encoding note:** CSV is UTF-8 (not Windows-1250). Do NOT use `convertToUTF8` (that's PKO-specific). Model after `santander.go`, not `pko.go`.

**Existing patterns:**
- Interface: `Parse(acc txn.BankAccount, data [][]string) []txn.Transaction`
- Registration: static map in `internal/app/app.go` keyed by bank name constant
- Discovery: case-insensitive substring match on account name
- Amount helper: `getAmount(index, row)` — single column; Millenium needs a two-column variant
- Adjacent column convention: PKO uses `DescriptionIndex` and `DescriptionIndex+1` — Millenium will use `AmountIndex` (col 7, debit) and `AmountIndex+1` (col 8, credit)
- Files involved: `internal/parser/`, `internal/app/app.go`

## Development Approach

- **Testing approach**: TDD — write tests first, then implement to make them pass
- Complete each task fully before moving to the next
- All tests must pass before starting the next task

## Testing Strategy

- **Unit tests**: `internal/parser/millenium_test.go` — mock `Hasher` and `TimeProvider` (reuse existing `mockHasher` and `mockTimeProvider`)
- Test both debit and credit rows
- Test edge case: both amount columns empty → `TransactionInvalid`
- Mirror the test structure from `pko_test.go`

## Implementation Steps

### Task 1: Write failing tests (TDD)

**Files:**
- Create: `internal/parser/millenium_test.go`

- [x] write `TestMilleniumParser_Parse_EmptyData` — empty slice returns empty slice
- [x] write `TestMilleniumParser_Parse_ValidDebit` — debit row (col 7=`"-16.55"`, col 8=`""`) → amount=-16.55, date, payee=Opis, currency=`PLN`, status=`TransactionDraft`
- [x] write `TestMilleniumParser_Parse_ValidCredit` — credit row (col 7=`""`, col 8=`"87.10"`) → amount=+87.10
- [x] write `TestMilleniumParser_Parse_DebitSignIsPreserved` — assert a debit row with input `"-23.00"` produces amount `-23.00` (sign comes from column value, not column position); locks the pre-signed debit invariant
- [x] write `TestMilleniumParser_Parse_InvalidColumns` — wrong column count → `TransactionInvalid` with non-empty `ErrorMsg`
- [x] write `TestMilleniumParser_Parse_BothAmountsEmpty` — both col 7 and col 8 empty → `TransactionInvalid`
- [x] write `TestMilleniumParser_Parse_InvalidAmount` — table-driven: bad value in debit column AND bad value in credit column → both yield `TransactionInvalid`
- [x] write `TestMilleniumParser_Parse_InvalidDate` — unparseable date → `TransactionInvalid`
- [x] write `TestMilleniumParser_Parse_HeaderHandling` — table-driven: with header (skip row 0), without header, header-only file (1 row → 0 transactions after skip)
- [x] write `TestMilleniumParser_Parse_PolishCharacters` — Opis containing Polish characters (e.g. `"Żabka Sklep Ząb"`) → `Payee` field round-trips unchanged (guards against accidental double-decode)
- [x] write `TestMilleniumParser_UniqueIDs` — two distinct rows produce distinct non-empty IDs
- [x] confirm tests fail (no implementation yet): `go test ./internal/parser/...`

### Task 2: Implement MilleniumParser

**Files:**
- Modify: `internal/parser/parser.go` (add `MilleniumBankName` constant)
- Create: `internal/parser/millenium.go`

- [x] add `MilleniumBankName = "Millennium"` constant to `internal/parser/parser.go`
- [x] create `internal/parser/millenium.go` with `MilleniumParser` struct, `NewMilleniumParser` constructor, and `Parse` method
- [x] implement `getMilleniumAmount(row []string, debitIdx, creditIdx int) (float64, error)` — package-level helper: if `row[debitIdx] != ""` parse debit (already signed), else if `row[creditIdx] != ""` parse credit, else return error; error message must identify which column was attempted
- [x] `Parse` flow: skip header → `validColumnsAmount` → `getMilleniumAmount` → parse date (`time.ParseInLocation`) → hash row → build transaction with `row[cfg.DescriptionIndex]` as both `Payee` and `Description`, `getCurrency(cfg, row)` for currency
- [x] run tests — must all pass: `go test ./internal/parser/...`

### Task 3: Wire parser into the app

**Files:**
- Modify: `internal/app/app.go`

- [x] add `milleniumConfig()` function returning `parser.Config{TransactionDateIndex: 1, DescriptionIndex: 6, AmountIndex: 7, CurrencyIndex: 10, DateFormat: "2006-01-02", BankName: parser.MilleniumBankName, ColumnsAmount: 11, Header: parser.HeaderCfg{HasHeader: true}}`
- [x] add `parser.MilleniumBankName: parser.NewMilleniumParser(milleniumConfig(), sha256.New(), timeProvider)` to the parsers map in `New()`
- [x] run full test suite: `go test ./...`

### Task 4: Verify acceptance criteria

- [x] verify debit transaction parses to negative amount
- [x] verify credit transaction parses to positive amount
- [x] verify `ZAKUP - FIZ. UŻYCIE KARTY` Opis (e.g. `PUTKA KLIMCZAKA 17  WARSZAWA POL 2026-06-29`) is stored as-is in `Payee` and `Description`
- [x] run full test suite: `go test ./...`

### Task 5: [Final] Move plan to completed

- [x] move this plan to `docs/plans/completed/`

## Technical Details

**Amount extraction logic** (`getMilleniumAmount`):
```
if row[debitIdx] != "":
    return strconv.ParseFloat(row[debitIdx], 64)
if row[creditIdx] != "":
    return strconv.ParseFloat(row[creditIdx], 64)
return 0, error("both debit and credit columns are empty at line N")
```

**Config values for `milleniumConfig()`:**
| Field                | Value        |
|----------------------|--------------|
| TransactionDateIndex | 1            |
| DescriptionIndex     | 6            |
| AmountIndex          | 7 (debit)    |
| AmountIndex+1        | 8 (credit)   |
| CurrencyIndex        | 10           |
| DateFormat           | "2006-01-02" |
| BankName             | "Millennium" |
| ColumnsAmount        | 11           |
| Header.HasHeader     | true         |

**BOM note:** UTF-8 BOM in the CSV only affects `data[0][0]` (the header cell). Since `Header.HasHeader = true` skips the first row, BOM does not affect data parsing. `ValidateHeader` must remain false — if it is ever enabled, the BOM prefix on `data[0][0]` will break any exact-string header comparison.

**Encoding:** CSV is already UTF-8. Do NOT call `convertToUTF8` (that is PKO's Windows-1250 decoder). Copying PKO's encoding logic would mangle Polish characters like `ż`, `ą`, `ę` in Opis.

**Bank name matching:** YNAB account names like "Bank Millennium" or "Millennium PLN" will match via the case-insensitive substring search in `processor.go`.

## Post-Completion

**Manual verification:**
- Upload a real Millenium CSV export through the web UI and confirm transactions appear with correct amounts, dates, and payees
- Check a physical card purchase row (`ZAKUP - FIZ. UŻYCIE KARTY`) to confirm the full Opis string is captured as payee
