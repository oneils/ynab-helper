# ING Bank Parser

## Overview

Add a CSV parser for ING Bank Śląski S.A. exports so users can import their ING transactions into YNAB.

ING exports differ from the other supported banks in two structural ways that require special handling:
1. **Semicolon delimiter** — all other supported parsers consume comma-delimited CSVs; ING uses `;` throughout.
2. **Multi-row preamble** — the file begins with ~18 metadata lines before the real column-header row; the parser must scan for `Data transakcji` to find where data actually starts.

Both issues are solved transparently (automatic delimiter detection in the upload handlers, preamble scan in the parser itself), so the user experience is identical to every other parser.

## Context (from discovery)

- **Files involved:**
  - `internal/parser/parser.go` — constants (`SantanderBankName`, etc.) live here
  - `internal/parser/ing.go` — new file
  - `internal/parser/ing_test.go` — new file
  - `internal/app/app.go` — parsers map + per-bank config functions
  - `internal/server/handlers.go` — two upload code paths (`previewBankTxnsHandler`, `confirmBankTxnsHandler`) that instantiate `csv.Reader` directly
  - `internal/txn/processor.go` — two more `csv.Reader` constructions inside `Process()` (line 125) and `Preview()` (line 197); the legacy `uploadBankTxnsHandler` delegates here
  - `ui/html/partials/parser-mappings.tmpl.html` — dropdown listing supported parsers
- **Related patterns:** `MilleniumParser`, `PKOParser` (Windows-1250 conversion reused), `SantanderParser` (hash-column stability pattern reused)
- **Dependencies:** `golang.org/x/text/encoding/charmap` (already used by PKO), `golang.org/x/text/transform`

## ING CSV Format (from real export analysis)

| Index | Column name                        | Notes                                      |
|-------|------------------------------------|--------------------------------------------|
| 0     | Data transakcji                    | YYYY-MM-DD, date used for parsing          |
| 1     | Data księgowania                   | booking date; may be empty                 |
| 2     | Dane kontrahenta                   | counterparty → **Payee**                   |
| 3     | Tytuł                              | title/description → **Description**       |
| 4     | Nr rachunku                        | account number                             |
| 5     | Nazwa banku                        | bank name                                  |
| 6     | Szczegóły                          | type tag (PRZELEW, TR.KART, TR.BLIK, …)   |
| 7     | Nr transakcji                      | bank-assigned transaction ID               |
| 8     | Kwota transakcji (waluta rachunku) | **Amount** — comma decimal (`-250,00`)     |
| 9     | Waluta                             | **Currency** (PLN)                         |
| 10    | Kwota blokady / zwolnienie blokady | block/hold amount — populated for hold entries |
| 11    | Waluta                             | currency for col 10                        |
| 12    | Kwota płatności w walucie          | usually empty                              |
| 13    | Waluta                             | usually empty                              |
| 14    | Saldo po transakcji                | balance — varies between exports (excluded from hash) |
| 15    | Waluta                             | balance currency                           |
| 16–20 | (empty)                            | trailing padding columns                   |

**Total: 21 columns (indices 0–20)**

**Special rows to skip silently (intentional divergence from other parsers):**
- Rows with `< 21` columns — preamble metadata + footer "Dokument ma charakter informacyjny…". Other parsers call `validColumnsAmount` which produces `TransactionInvalid`; ING skips these silently because preamble/footer rows are expected and should not surface as errors. Rows with `> 21` columns are implicitly accepted but safe (indices 0–14 are within bounds).
- Rows where `col 8` is empty — authorization hold/block-release entries (col 10 holds the block amount; the corresponding debit in col 8 appears as a separate row)

**Stable hash columns:** `{0, 2, 3, 7, 8, 9}` — date, counterparty, title, transaction ID, amount, currency. Excludes booking date (col 1), balance (col 14), and trailing empties which can differ between re-exports.

## Development Approach

- **Testing approach:** Regular (code first, then tests per task)
- Complete each task fully before moving to the next
- All tests must pass before starting the next task
- Maintain backward compatibility — existing parsers and their tests must remain green

## Implementation Steps

---

### Task 1: Add ING bank name constant

**Files:**
- Modify: `internal/parser/parser.go`

- [x] Add `INGBankName = "ING"` constant alongside the existing bank name constants
- [x] Verify existing tests still pass: `go test ./internal/parser/...`

---

### Task 2: Implement ING parser

**Files:**
- Create: `internal/parser/ing.go`

- [x] Define `INGParser` struct with `cfg Config`, `hasher Hasher`, `timeProvider TimeProvider`
- [x] Implement `NewINGParser(cfg Config, hasher Hasher, tp TimeProvider) INGParser`
- [x] Implement `Parse(acc txn.BankAccount, data [][]string) []txn.Transaction`:
  - Scan rows for the first one where `strings.TrimSpace(row[0]) == "Data transakcji"` → that is the header; data starts at the next index. If no header found, start from row 0 (supports test data without preamble).
  - For each data row:
    - `len(row) < 21` → `continue` silently (preamble/footer rows)
    - `strings.TrimSpace(row[8]) == ""` → `continue` silently (block/hold entry)
    - Parse amount via `getAmount(8, row)`; on error → `TransactionInvalid`
    - Parse date via `time.ParseInLocation("2006-01-02", row[0], warsawLocation)`; on error → `TransactionInvalid`
    - Convert `row[2]` (counterparty) and `row[3]` (description) from Windows-1250 to UTF-8; on error → `TransactionInvalid`
    - Generate SHA-256 hash via `buildHashInput(cfg, row)` using `HashColumns`
    - Append `txn.Transaction` with `Payee = counterparty`, `Description = description`
- [x] Add `convertToUTF8(input string) (string, error)` method (identical to PKO's implementation)
- [x] Set `RawText = strings.Join(row, ",")` — uses comma joiner like all sibling parsers (the CSV was already split by the reader; the join is for display/logging only)
- [x] Run `go build ./...` to verify it compiles

---

### Task 3: Write ING parser tests (anonymized)

**Files:**
- Create: `internal/parser/ing_test.go`

Test data uses only ASCII-safe names (no Windows-1250 multi-byte chars) so the encoding round-trip is a no-op in test scenarios; real Polish characters are exercised by the converter that is already tested in the PKO parser. The test data below is fully anonymized from the real export.

- [x] `TestINGParser_Parse_EmptyData` — returns empty slice
- [x] `TestINGParser_Parse_DirectData_NoHeader` — 21-column transaction rows passed without any preamble/header row → all parsed
- [x] `TestINGParser_Parse_WithPreambleAndHeader` — realistic structure: 2 metadata rows + header row + 2 transaction rows → only 2 transactions returned
- [x] `TestINGParser_Parse_ValidCardPayment` — TR.KART row: asserts `Amount`, `Payee`, `Description`, `Currency`, `TxnTime`, `Status == Draft`
- [x] `TestINGParser_Parse_ValidWireTransfer` — PRZELEW row (positive amount / income)
- [x] `TestINGParser_Parse_ValidBLIKPayment` — TR.BLIK row
- [x] `TestINGParser_Parse_BlockEntrySkipped` — row with empty col 8 produces 0 transactions
- [x] `TestINGParser_Parse_FooterRowSkipped` — row with < 21 columns produces 0 transactions
- [x] `TestINGParser_Parse_InvalidDate` → `TransactionInvalid` with non-empty `ErrorMsg`
- [x] `TestINGParser_Parse_InvalidAmount` → `TransactionInvalid` with non-empty `ErrorMsg`
- [x] `TestINGParser_Parse_RawTextPopulated` — valid row sets `RawText != ""`
- [x] `TestINGParser_HashColumns_StableAcrossReExport` — two rows identical in cols {0,2,3,7,8,9} but differing in col 14 (balance) produce the **same** ID
- [x] `TestINGParser_HashColumns_DifferentForDifferentAmount` — two rows differing in col 8 produce different IDs
- [x] `TestINGParser_UniqueIDs` — two distinct transactions produce different, non-empty IDs
- [x] Run `go test ./internal/parser/...` — all tests must pass

---

### Task 4: Wire ING parser in app

**Files:**
- Modify: `internal/app/app.go`

- [x] Add `ingConfig() parser.Config` function:
  ```
  TransactionDateIndex: 0
  DescriptionIndex:     3
  AmountIndex:          8
  CurrencyIndex:        9
  DateFormat:           "2006-01-02"
  BankName:             parser.INGBankName
  ColumnsAmount:        21
  Header:               parser.HeaderCfg{HasHeader: false}  // preamble scan handles this
  HashColumns:          []int{0, 2, 3, 7, 8, 9}
  ```
- [x] Add `parser.INGBankName: parser.NewINGParser(ingConfig(), sha256.New(), timeProvider)` to the `parsers` map in `New()`
- [x] Run `go build ./...`

---

### Task 5: Add CSV delimiter auto-detection and variable-column-count support

ING uses `;` while all other banks use `,`. There are **four** `csv.Reader` construction sites that must all be updated. Additionally, Go's `csv.Reader.FieldsPerRecord` defaults to 0 (inferred from row 1 and then enforced), which means the ING preamble's variable-width rows will cause `ReadAll()` to return "wrong number of fields" before the parser ever runs — set `FieldsPerRecord = -1` on every reader.

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `internal/txn/processor.go`

- [x] Add package-level helper `detectCSVComma(data []byte) rune` in `handlers.go`:
  - Count `;` and `,` in the first 500 bytes of the file
  - Return `';'` if semicolons > commas, else return `','`
  - ING's first line alone has 15+ semicolons; comma-delimited bank exports have none in the header
- [x] In `previewBankTxnsHandler` (~line 1079): set `csvReader.Comma = detectCSVComma(fileBytes)` **and** `csvReader.FieldsPerRecord = -1` before `csvReader.ReadAll()`
- [x] In `confirmBankTxnsHandler` (~line 1143): set `csvReader.Comma = detectCSVComma(fileBytes)` **and** `csvReader.FieldsPerRecord = -1` before `csvReader.ReadAll()`
- [x] In `processor.Process()` (~line 125 in `processor.go`): refactored `uploadBankTxnsHandler` to pre-parse the CSV (with delimiter detection) and pass via `params.Data`, eliminating the `params.File`/`FileHandler` reader path entirely from `ProcessParams` and `Process()`.
- [x] In `processor.Preview()` (~line 197 in `processor.go`): removed the `params.File` path; `Preview()` now requires pre-parsed `Data`.
- [x] Add tests for `detectCSVComma` in `handlers_test.go` (table-driven): semicolon-dominant input → `';'`, comma-dominant → `','`, empty/short input → `','`
- [x] Run `go build ./...`

---

### Task 6: Update Settings UI parser dropdown

**Files:**
- Modify: `ui/html/partials/parser-mappings.tmpl.html`

- [x] Add `<option value="ING" {{if eq .ParserName "ING"}}selected{{end}}>ING</option>` to the `parser_name` select inside the `"parser-mapping-rows"` template block (after the existing Millennium option)
- [x] Verify the template parses correctly: `go build ./...`

---

### Task 7: Verify acceptance criteria

- [x] Run `go test ./...` — full suite green, no regressions
- [x] Confirm ING appears in the settings dropdown (manual check or template rendering test) — verified via `parser-mappings.tmpl.html` option present
- [x] Verify the preamble rows from the provided sample file are correctly skipped — covered by `TestINGParser_Parse_WithPreambleAndHeader` and `TestINGParser_Parse_FooterRowSkipped`
- [x] Verify block/hold entry rows are silently skipped — covered by `TestINGParser_Parse_BlockEntrySkipped`

---

### Task 8: [Final] Cleanup

- [x] Move this plan to `docs/plans/completed/`

## Technical Details

**Preamble scan logic (pseudocode):**
```
dataStart = 0
for i, row in data:
    if len(row) > 0 and TrimSpace(row[0]) == "Data transakcji":
        dataStart = i + 1
        break
// if no header found, dataStart stays 0 → works for test data passed directly
```

**Delimiter detection:**
```go
func detectCSVComma(data []byte) rune {
    n := len(data)
    if n > 500 {
        n = 500
    }
    sample := data[:n]
    if bytes.Count(sample, []byte{';'}) > bytes.Count(sample, []byte{','}) {
        return ';'
    }
    return ','
}
```

**csv.Reader configuration (required for ING, safe for all banks):**
```go
csvReader.LazyQuotes = true
csvReader.TrimLeadingSpace = true
csvReader.Comma = detectCSVComma(fileBytes)
csvReader.FieldsPerRecord = -1  // disables row-count enforcement; ING preamble rows have variable widths
```

**ING-specific skip conditions (both silent, no TransactionInvalid):**
- `len(row) < 21` — preamble, empty, or footer rows. Intentional divergence from other parsers (which use `validColumnsAmount` → TransactionInvalid) because these rows are structural, not data.
- `strings.TrimSpace(row[8]) == ""` — authorization hold/block-release entries

**Encoding:** Windows-1250 → UTF-8 for cols 2 and 3 only; date/amount/currency cols are ASCII-safe and need no conversion.

## Post-Completion

**Manual verification:**
- Upload the real ING CSV (`ING-Lista_transakcji_nr_0243141344_220726.csv`) against an ING account in the app and confirm ~93 valid transactions are imported (2 block entries + footer should be silently skipped, leaving 95 total rows minus 2 = 93).
- Confirm Polish characters in counterparty names render correctly in the transaction list.
- Confirm re-uploading the same file produces no duplicates (hash stability).
