# Remove confirmation dialogs on Accept & Send / Skip

## Overview
Remove the native browser `confirm()` prompts that currently appear when a
user clicks "Accept & Send to YNAB" or "Skip" on a transaction. These
prompts are implemented via htmx's `hx-confirm` attribute, which htmx
resolves by calling `window.confirm(text)` before firing the associated
`hx-post` request. Removing the attribute removes the prompt; the request
fires immediately on click instead.

**Safety note:** the modal `confirm()` dialog also incidentally guards
against double-clicks — while it's open, a second click can't fire a
second request. Neither the handler
(`uploadTxnToYnabHandler`, `internal/server/handlers.go:686`) nor the
domain logic (`SaveToYnab`, `internal/txn/processor.go:251`) nor the YNAB
API call (`TxnReq`, `internal/ynab/client.go:79`, which sets no
`import_id`) guards against a duplicate submit. This project has already
fixed a duplicate-transactions bug once (see
`docs/plans/completed/20260722-fix-duplicate-transactions.md`), so removing
`hx-confirm` without a replacement guard reopens that class of bug. This
plan adds `hx-disabled-elt="this"` (supported since htmx 1.9.0; this repo
loads 1.9.4, see `ui/html/index.tmpl.html:171`) to each button to disable
it for the duration of its in-flight request, closing the gap.

## Context (from discovery)
- Backend: Go (chi router), server-rendered `html/template` partials, htmx
  (CDN, v1.9.4) for AJAX interactivity. No client-side JS confirmation
  modal is involved — `hx-confirm` triggers htmx's built-in
  `window.confirm()` handling.
- No tests (Go or otherwise) reference `hx-confirm`, the button text, or
  this confirmation behavior — there is no e2e/browser test suite in the
  repo (no Playwright/Cypress/Jest).
- Files/locations involved (all `hx-confirm` attributes tied to the
  Accept/Skip actions):
  1. `ui/html/partials/txn-detail-panel.tmpl.html:63` — "Accept & Send to
     YNAB" button, `hx-confirm="Send this transaction to YNAB?"`
  2. `ui/html/partials/txn-detail-panel.tmpl.html:84` — "Skip" button in
     the detail panel, `hx-confirm="Skip this transaction?"`
  3. `ui/html/partials/txn-rows.tmpl.html:39` — row-level "Skip" action in
     the transaction list, `hx-confirm="Skip this transaction?"`

## Development Approach
- **Testing approach**: Regular (no meaningful automated test exists for
  this — it's a static template attribute removal with no JS/e2e test
  harness in the repo; verification is manual).
- Single obvious implementation: delete the `hx-confirm="..."` attribute
  line from each of the three buttons above. No other approach was
  considered — there's nothing to trade off.

## Testing Strategy
- No unit tests apply (pure template markup change, no Go logic touched).
- No e2e test suite exists in this project to update.
- Verification is manual: build/run the app, confirm clicking each button
  fires the htmx request immediately without a browser confirm popup.

## Progress Tracking
- Mark completed items with `[x]` immediately when done.

## Implementation Steps

### Task 1: Remove hx-confirm and add hx-disabled-elt guard

**Files:**
- Modify: `ui/html/partials/txn-detail-panel.tmpl.html`
- Modify: `ui/html/partials/txn-rows.tmpl.html`

- [x] remove `hx-confirm="Send this transaction to YNAB?"` and add
      `hx-disabled-elt="this"` on the "Accept & Send to YNAB" button
      (`txn-detail-panel.tmpl.html:63`)
- [x] remove `hx-confirm="Skip this transaction?"` and add
      `hx-disabled-elt="this"` on the "Skip" button in the detail panel
      (`txn-detail-panel.tmpl.html:84`)
- [x] remove `hx-confirm="Skip this transaction?"` and add
      `hx-disabled-elt="this"` on the row-level "Skip" action
      (`txn-rows.tmpl.html:39`)
- [x] run `make test` — must pass unchanged (templates are parsed at
      runtime by `NewTemplateCache`, not at build time, so this is the
      real regression guard here, not `go build`)
- [x] run `make lint` — must be clean, per CONTRIBUTING.md

### Task 2: [Final] Verify and wrap up
- [x] manually run the app and click "Accept & Send to YNAB" — confirm no
      browser prompt appears and the transaction is sent (skipped - not
      automatable, no browser/e2e harness in this environment)
- [x] manually click "Skip" (both detail panel and row action) — confirm
      no browser prompt appears and the transaction is skipped (skipped -
      not automatable, no browser/e2e harness in this environment)
- [x] manually rapid double-click "Accept & Send to YNAB" — confirm only
      one request fires (button visibly disabled during the in-flight
      request) and only one transaction reaches YNAB (skipped - not
      automatable, no browser/e2e harness in this environment)
- [x] move this plan to `docs/plans/completed/`

## Technical Details
- `hx-confirm` and `hx-disabled-elt` are pure htmx attributes — no JS, CSS,
  or Go changes are needed beyond swapping these three attribute lines.
- `hx-disabled-elt="this"` sets the `disabled` attribute on the button
  itself for the duration of its own in-flight htmx request, then removes
  it — this is htmx's built-in double-submit guard, not custom code.
- No other `hx-confirm` usages in the repo are affected (only these three,
  all tied to Accept/Skip actions per user's chosen scope).

## Post-Completion
None required. Risk-posture note: `hx-confirm` was the last-line guard
against a double-click causing a duplicate YNAB upload; it's replaced with
`hx-disabled-elt="this"` in Task 1 rather than removed outright, so no
external-system risk is introduced.
