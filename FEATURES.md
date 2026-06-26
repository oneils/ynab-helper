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

- **Status tabs** show per-status counts and filter the list (All / Needs Review / Accepted / Skipped / Invalid)
- **List rows** display: Date, Description (truncated), Amount, Payee, Category, Status
- Rows with YNAB payee matches show an "auto" badge
- Click a row to open the **detail panel** on the right

### Detail Panel

- Payee dropdown: pre-filled if YNAB found a matching payee via `last_used_category_id`
- Category dropdown: same pattern — pre-filled if YNAB data exists for the payee
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
- **Routes**: `GET /import-bank-txns` (page), `GET /bank-txns?status=` (filtered list), `GET /bank-txns/{id}/detail` (panel)

## Suggestion Engine

Uses YNAB payee history (`last_used_category_id`) — no confidence scoring. It's a binary match:
known payee → pre-fill + "auto" badge, unknown → empty dropdown.
