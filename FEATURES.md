# Split-Panel Transaction Review

## Overview

The Transactions page (`/import-bank-txns`) uses a split-panel layout for fast transaction review:
- **Left panel**: Filterable list with status tabs (Needs Review, Accepted, Skipped, Invalid)
- **Right panel**: Detail panel with payee/category selects pre-filled from YNAB history

After importing a CSV via the Home page, you are redirected to this split-panel view where
you can process 100+ transactions without page-per-transaction navigation.

## How It Works

### Import Flow

1. Upload a bank CSV on the Home page (`/`)
2. Preview the parsed transactions
3. Click "Confirm" — all transactions are saved and you are redirected to `/import-bank-txns`

### Split-Panel Review

- **Filter card** (sticky at top): unified budget/account selector and status tabs — All / Needs Review / Accepted / Skipped / Invalid with per-status counts
- **List rows** display: Date, Description (truncated), Amount, Payee, Category, Status; loaded via **infinite scroll** (HTMX sentinel row, `GET /bank-txns/rows`)
- Rows with YNAB payee matches show an "auto" badge
- Click a row to open the **detail panel**, which is sticky and dynamically positioned below the filter card so they never overlap

### Detail Panel

- Payee `<select>`: pre-filled if YNAB found a matching payee via `last_used_category_id`
- Category `<select>`: narrows automatically when payee changes; pre-filled from YNAB data
- **Remember toggle**: fires `save-inline` automatically whenever both Payee and Category are filled; visually disabled (greyed out) when either field is empty
- Actions:
  - **Accept & Send to YNAB** — posts the transaction to YNAB and marks it Accepted
  - **Save** — persists edits, keeps status as Needs Review
  - **Skip** — marks the transaction as Skipped

### Status Mapping

| DB Value  | Display      |
|-----------|--------------|
| DRAFT     | Needs Review |
| PROCESSED | Accepted     |
| SKIPPED   | Skipped      |
| INVALID   | Invalid      |

## Architecture

- **Templates**: `bank-transactions.tmpl.html` (list), `txn-detail-panel.tmpl.html` (detail), `status-tabs.tmpl.html` (tabs)
- **CSS**: Split-panel layout in `main.css` (`.txn-split-layout`, `.txn-list-panel`, `.txn-detail-panel`)
- **HTMX**: Row clicks load detail panel; status tabs filter via `hx-get`; action buttons update via `hx-post`
- **Routes**: `GET /import-bank-txns` (page), `GET /bank-txns` (filtered list), `GET /bank-txns/rows` (infinite scroll batches), `GET /bank-txns/{id}/detail` (panel)

## Suggestion Engine

Two-stage fallback, no confidence scoring:

1. **Learned patterns** — match bank transaction description against stored patterns; if found, pre-fill payee + category
2. **YNAB payee name fallback** — if no pattern match, fuzzy-match the description against YNAB payee names; on hit, pre-fill payee + `last_used_category_id` category and show the "auto" badge

Unknown transactions (no match in either stage) → empty dropdowns.
