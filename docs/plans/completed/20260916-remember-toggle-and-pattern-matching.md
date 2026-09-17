# Remember Toggle Semantics & Pattern-Matching Fixes

## Overview

Two intertwined problems, diagnosed in code review of the "smart suggestions" feature:

1. **The "Remember Selections" checkbox lies about what it does.** It fires its own
   `POST /bank-txns/{id}/save-inline` request that records a payee/category pattern
   immediately on toggle, *regardless of whether the transaction is ever sent to
   YNAB*. Meanwhile `Accept & Send to YNAB` *unconditionally* records the same
   pattern again (`handlers.go:725` / `handlers.go:759`). Result: `occurrence_count`
   can double-increment for a single transaction, and patterns get created for
   transactions the user never actually accepted. Neither a rename nor new copy on
   the checkbox is honest until this is fixed — remembering must become an explicit
   opt-in that only takes effect after a successful send to YNAB.

2. **The matching algorithm behind payee/category suggestions has eight separate
   correctness problems**: a fragile substring-only candidate search, weak
   normalization that leaves card numbers/dates/reference IDs in the fingerprint,
   an incorrect Jaccard similarity (can exceed 1.0 with repeated tokens), no
   confidence floor before a suggestion gets auto-filled, category ranking that
   ignores the current description once a payee is known, "keep highest single
   pattern" instead of aggregating evidence per category, no invalidation of a
   pattern the user has since corrected, and `last_seen` that can move backwards on
   re-import.

This plan fixes the checkbox semantics first (it's the root cause of double
learning and gates everything else), then reworks the matching engine so
suggestions are actually trustworthy, then closes the test-coverage gap the
review found (green tests today only prove the code runs, not that
recommendations are correct).

## Context (from discovery)

**Files/components involved:**
- `internal/server/handlers.go` — `uploadTxnToYnabHandler` (Accept & Send to
  YNAB, unconditional pattern recording at `:725`/`:759`), `saveInlineTxnHandler`
  (checkbox's own endpoint, `:1227`), `payeeSuggestionsHandler` /
  `categorySuggestionsHandler` (JSON APIs backing detail-panel JS), detail-panel
  data assembly (`:547-586`, always takes `payeeSugs[0]`/`catSugs[0]` with no
  confidence check).
- `internal/server/server.go` — route registration for `save-inline` (to be
  removed).
- `internal/txn/suggestions.go` — `SuggestionEngine`, `calculateConfidence`,
  `tokenSimilarity` (broken Jaccard), `normalize` (weak — diacritics only).
- `internal/txn/processor.go` — `RecordPattern`, `GetSmartSuggestions`,
  `GetCategorySuggestions` (thin wrappers around the engine).
- `internal/sqlite/pattern_store.go` — `UpsertPattern` (occurrence increment,
  `last_seen` overwrite bug), `FindPatternsByDescription` (brittle
  `LIKE '%'||desc||'%'`), `FindPatternsByPayeeID`.
- `internal/sqlite/migrations/` — goose migrations, embedded via
  `//go:embed migrations/*.sql`, applied automatically on startup
  (`internal/sqlite/sqlite.go:runMigrations`). Currently SQL-only up to
  `00004_payee_patterns_budget_payee_index.sql`; goose also supports Go-coded
  migrations registered the same way, which this plan needs for the
  fingerprint backfill.
- `ui/html/partials/txn-detail-panel.tmpl.html` — the checkbox markup (`:68-78`)
  and the `Accept & Send to YNAB` button (`hx-include="#detail-form"`, `:56-67`).
- `ui/static/js/detail-panel.js` — `fetchCategorySuggestions` (replaces the
  *entire* category list with suggestions, `:207`), `guardRememberToggle` /
  `updateRememberToggleState` (checkbox enable/disable logic, `:237-300`).

**Related patterns found:**
- Route existence is regression-tested via `chi.Walk` in
  `internal/server/handlers_test.go` (see `TestDetailRoute_SaveInlineStillExists`,
  which this plan deliberately inverts/removes).
- DB tests use an in-memory `modernc.org/sqlite` DB with an inline schema
  (`internal/sqlite/pattern_store_test.go:setupPatternTestDB`), not the real
  goose migration chain — the new migration needs its own test path.
- `SuggestionEngine` is tested purely through a hand-rolled `mockPatternStore`
  (`internal/txn/suggestions_test.go`) — no real bank-description fixtures yet.

**Dependencies identified:**
- `github.com/pressly/goose/v3` for migrations (already a dependency).
- No JS test runner in this repo — detail-panel.js changes are covered by
  server-side tests where the behavior is observable (HTML output, JSON
  responses) and otherwise verified manually (see Post-Completion).
- No e2e framework (Playwright/Cypress) present — `make test` (`go test -race
  -vet=off -coverprofile=cover.out ./...`) is the full verification command.

## Decisions made during planning

- **Scope:** one plan, ordered tasks — checkbox/double-write fix first
  (tasks 1a-1b), then the algorithm rework, since later tasks depend on
  recording happening exactly once.
- **Confidence thresholds** (≥85 prefill / 65–84 suggest / <65 hide): hardcoded
  constants in `internal/txn/suggestions.go` for now; revisit with real data
  later rather than adding config surface pre-emptively.
- **Conflict resolution** on user correction: delete the old conflicting pattern
  row (same budget + fingerprint + payee, different category) rather than
  decaying its count — simpler and deterministic.
- **Fingerprint backfill migration:** a Go-coded goose migration (not raw SQL),
  registered via a blank import so it runs automatically on startup like the
  existing SQL migrations, tracked in `goose_db_version`. It calls the new
  `txn.Fingerprint()` function to recompute `normalized_description` for
  every existing row and merges rows that collide after re-fingerprinting by
  summing `occurrence_count` and keeping the newest `last_seen`.
- **`POST /bank-txns/{id}/save-inline`:** deleted entirely (handler, route,
  tests, task 1b). It only ever existed to let the checkbox record a pattern
  outside the accept flow — the exact behavior task 1a removes. Keeping it as
  dead code would leave a working endpoint that contradicts the new "only
  learn after a successful send" rule.
- **`UpdatePayeeLastCategory` stays unconditional; only `RecordPattern` is
  gated by the checkbox.** `Payee.LastCategoryID` feeds a separate, pre-existing
  feature — the YNAB-side fallback prefill in `applyYnabPayeeFallback`
  (`handlers.go:565`, `docs/plans/completed/20260627-payee-category-prefill-from-ynab.md`)
  — that has nothing to do with the learned-pattern engine this plan is fixing.
  Gating it behind "remember" too would silently regress that unrelated
  feature for anyone who leaves the box unchecked.
- **Testing approach:** TDD — write the failing test first, then the fix, for
  every task below.

## Development Approach
- TDD: write tests first, watch them fail for the right reason, then implement.
- Complete each task fully (including tests, including a passing `go test
  ./...` run) before starting the next — later tasks build on earlier ones
  (fingerprint → candidate search → confidence → ranking → conflict handling
  all touch the same pattern-matching path).
- Make small, focused commits per task.
- Update this plan file if scope changes during implementation (➕ / ⚠️ prefixes).

## Acceptance Criteria
- Toggling "Remember Selections" sends no network request by itself.
- Accepting a transaction with the checkbox unchecked leaves `payee_patterns`
  untouched for that transaction; checking it and accepting records the
  pattern exactly once (no double-increment).
- The YNAB-side "last used category" prefill (`applyYnabPayeeFallback`) keeps
  working regardless of the checkbox state.
- All eight review findings have a corresponding fix + test: candidate
  search, normalization/fingerprint, Jaccard correctness, confidence
  threshold (on every surface that auto-fills, including the imported-list
  view), description-aware category ranking, aggregated category evidence,
  correction invalidation, monotonic `last_seen`.

## Progress Tracking
- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.
- Update this plan if implementation deviates from the original scope.

## Testing Strategy
- Unit tests required for every task, success and error paths.
- No e2e framework in this repo; UI-only changes (template/JS) get a manual
  verification note in Post-Completion instead of an automated e2e test.
- Full suite: `make test` (`go test -race -vet=off -coverprofile=cover.out ./...`).

## What Goes Where
- **Implementation Steps** (`[ ]`): code, tests, docs achievable in this repo.
- **Post-Completion**: manual UI verification of the checkbox/JS changes,
  since there's no JS test runner or e2e suite to automate it.

## Implementation Steps

### Task 1a: Wire "remember" as an opt-in form field, gated on a successful Accept

The checkbox currently lives in `.detail-actions`, **outside** `<form
id="detail-form">` (the form closes at line 54; the checkbox is at lines
68-78). Simply adding a `name` to it and leaving it in place would NOT be
picked up by the Accept button's `hx-include="#detail-form, #sort-state"` —
htmx's `hx-include` walks descendants of the matched selector, and the
checkbox isn't one. Fix by widening `hx-include` on the button, not by
moving the checkbox (avoids a CSS/layout rework of `.detail-actions`).

**Files:**
- Modify: `ui/html/partials/txn-detail-panel.tmpl.html`
- Modify: `ui/static/js/detail-panel.js`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/handlers_test.go`

- [x] On `.remember-checkbox`: drop `hx-post`/`hx-include`/`hx-target`/
      `hx-swap`/`hx-trigger` entirely; add `name="remember_similar"
      value="true"`.
- [x] Widen the `Accept & Send to YNAB` button's `hx-include` from
      `"#detail-form, #sort-state"` to `"#detail-form, .remember-checkbox,
      #sort-state"` so the checkbox's value rides along on that request.
- [x] Update the label to "Use this payee and category for similar
      transactions" and add helper text "Future transactions with a similar
      bank description will use these values as suggestions." beneath it.
- [x] In `detail-panel.js`, delete `guardRememberToggle` and its
      `htmx:before:request` listener (there's no longer a request on this
      element to intercept). Fold the enable/disable logic into
      `updateRememberToggleState`: set the checkbox's `disabled` property (and
      force `checked = false`) when payee or category isn't set, instead of
      only toggling a CSS class.
- [x] In `uploadTxnToYnabHandler` (`handlers.go`): after `SaveToYnab` succeeds,
      read `remember := r.PostForm.Get("remember_similar") == "true"`. Leave
      the existing `UpdatePayeeLastCategory` call unconditional (see Decisions
      — it powers the separate YNAB fallback prefill). Gate only the
      name-lookup + `RecordPattern` block on `remember`; when false, skip it
      entirely — no pattern is written.
- [x] write test: render `"txn-detail-panel"` and assert the output contains
      `name="remember_similar"` and no longer contains `/save-inline` (catches
      a regression to the include-selector fix above, which no assertion on
      the handler alone would catch).
- [x] write test: POST to `/ynab-add-txn` with `remember_similar=true` and a
      fake `txn.YnabUploader` that succeeds → the `txn.PatternStorer` fake
      passed into `txn.NewSuggestionEngine` (same fake style as
      `suggestions_test.go`'s `mockPatternStore`) records `UpsertPattern`
      called exactly once.
- [x] write test: same request with `remember_similar` absent/`"false"` →
      `UpsertPattern` NOT called.
- [x] write test: fake `txn.YnabUploader` returns an error → `UpsertPattern`
      NOT called even with `remember_similar=true` (no learning from a failed
      send). Build minimal fakes for `txn.NewProcessor`'s `YnabUploader` /
      `TransactionStorer` / `BudgetFinder` params, following the
      `fakeYnabClient` / `fakeBudgetStorer` / `fakeHistoryStorer` pattern
      already used lower in `handlers_test.go` for `ynab.NewSyncer`.
- [x] run tests — must pass before task 1b.

### Task 1b: Delete the now-dead `save-inline` endpoint

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/handlers_test.go`
- Modify: `FEATURES.md`
- Modify: `http/api.http`

- [x] Delete `saveInlineTxnHandler` from `handlers.go` and its route
      (`r.Post("/{id}/save-inline", ...)`) from `server.go`.
- [x] Remove `TestDetailRoute_SaveInlineStillExists` and
      `TestSaveInlineTxnHandler_MissingPayeeOrCategory_SetsWarningTriggerAndReswapNone`
      from `handlers_test.go` (the endpoint they cover no longer exists).
- [x] write test: `chi.Walk` over `s.routes()` finds no `POST
      /bank-txns/{id}/save-inline` route (replaces the deleted "still exists"
      test with its inverse).
- [x] Update `FEATURES.md:31` — the "Remember toggle: fires `save-inline`..."
      line no longer describes the feature; replace with the new opt-in
      behavior.
- [x] Remove the `save-inline` request from `http/api.http:33-37`.
- [x] run tests — must pass before task 2.

### Task 2: Fix the broken Jaccard similarity

**Files:**
- Modify: `internal/txn/suggestions.go`
- Modify: `internal/txn/suggestions_test.go`

- [x] write test: `tokenSimilarity` with a repeated token in the second string
      (e.g. `"lidl warszawa"` vs `"lidl lidl lidl"`) never exceeds `1.0` —
      reproduces the current bug where only `s1` is deduplicated into a set.
- [x] write test: identical strings → `1.0`; disjoint strings → `0`; partial
      overlap → the expected exact ratio for a known unique-token-set example.
- [x] fix `tokenSimilarity` to build a unique token **set** from both `s1` and
      `s2` before computing intersection/union (currently only `tokens1` is
      deduplicated via `tokenSet`; `tokens2` is iterated with duplicates intact).
- [x] run tests — must pass before task 3.

### Task 3a: Replace weak normalization with a real fingerprint

**Files:**
- Modify: `internal/txn/suggestions.go` (new exported `Fingerprint()`,
  keep/rename existing diacritics logic as a helper it calls)
- Modify: `internal/txn/processor.go` (`RecordPattern` switches to
  `Fingerprint()`; `SuggestPayee`, at `processor.go:287`/`:291`, keeps using
  `normalize()` — it matches YNAB *payee names* like "Circle K 24" or
  "7-Eleven", where stripping digits would break matching and the existing
  `processor_test.go` coverage)
- Modify: `internal/txn/suggestions_test.go`

`Fingerprint()` must be **exported** (not `fingerprint()`) so the migration in
task 3b — which lives in a separate package — can call it.

- [x] write tests for `Fingerprint()` covering, at minimum: lowercasing;
      Polish-diacritic folding (existing behavior preserved); punctuation
      collapsed to spaces; repeated whitespace collapsed to one space; masked
      card numbers (`**** 1234`, `1234 5678 9012 3456`) stripped; date-like
      tokens (`2026-09-16`, `16.09.2026`) stripped; long numeric
      reference/transaction IDs (6+ consecutive digits) stripped; merchant
      tokens preserved (`"PAYMENT CARD 1234 LIDL WARSZAWA"` and `"PAYMENT CARD
      5678 LIDL KRAKOW"` both fingerprint down to a string containing `lidl`
      with the numbers gone).
- [x] implement `Fingerprint(s string) string` in `suggestions.go`.
- [x] switch `RecordPattern` (processor.go) and `SuggestionEngine.GetSuggestions`
      / `GetCategorySuggestions` (suggestions.go, currently calling
      `normalize()` at `:67`/`:142`/`:180`) to `Fingerprint()` for anything
      written to or queried against `normalized_description`.
- [x] run tests — must pass before task 3b.

### Task 3b: Go-coded migration to backfill existing patterns to the new fingerprint

The migrations directory currently holds only `.sql` files
(`internal/sqlite/migrations/*.sql`), embedded via `//go:embed
migrations/*.sql` in `internal/sqlite/sqlite.go:17` and applied through
`goose.Up(db, "migrations")`. A `.go` file dropped into that directory is
`package migrations` — nothing imports it, so its `init()`/
`goose.AddMigrationContext` registration never runs, and the embed directive
doesn't help (it only pulls `.sql` bytes). This has to be wired explicitly.

The table also has `idx_payee_patterns_unique` on `(budget_id,
normalized_description, payee_id, COALESCE(category_id, ''))`
(`00002_payee_patterns.sql`) — recomputing fingerprints row-by-row with a
naive `UPDATE` will hit that constraint the moment two old rows collapse to
the same fingerprint, which is exactly the case this migration exists to fix.

**Files:**
- Create: `internal/sqlite/migrations/go/00005_refingerprint_patterns.go`
  (a `.go` file must live outside the `//go:embed migrations/*.sql` tree or
  it's simply unused source in that package; placing Go migrations in a
  `migrations/go` subpackage keeps them separate from the embedded SQL set
  while still being registered the same way)
- Modify: `internal/sqlite/sqlite.go` (blank-import the new package so its
  `init()` runs before `goose.Up`)
- Modify: `internal/sqlite/migrations/README.md`
- Create: `internal/sqlite/refingerprint_migration_test.go` (or similar,
  testing through the real migration path)

- [x] write a migration test that opens a real `*sql.DB`, runs the actual
      `runMigrations`/`goose.Up` path (not the inline schema used by
      `pattern_store_test.go:setupPatternTestDB`) up to before `00005`, seeds
      a couple of old-format `normalized_description` rows that collide once
      re-fingerprinted (e.g. differing only by card number), then runs `00005`
      and asserts: exactly one surviving row, `occurrence_count` summed,
      `last_seen` is the newer of the two, and no unique-constraint error.
- [x] write a test asserting `00005` is actually registered — e.g. that
      `goose.GetDBVersion` reaches `5` after `runMigrations` on a fresh DB —
      so a missing blank-import (the exact failure mode described above)
      fails CI instead of silently no-op'ing in production.
- [x] implement `00005_refingerprint_patterns.go` (`package migrations`,
      registered via `goose.AddMigrationContext` per the pattern already
      documented in `migrations/README.md`'s Go-migration example): read all
      `payee_patterns` rows, compute each one's new fingerprint via
      `txn.Fingerprint(...)`, group by the resulting `(budget_id,
      normalized_description, payee_id, category_id)`, and for each group
      with more than one row: **delete all but one survivor first**, summing
      `occurrence_count` and taking `MAX(last_seen)` into the survivor,
      **then** `UPDATE` the survivor's `normalized_description` — in that
      order, so `idx_payee_patterns_unique` is never violated mid-migration.
      Give it an explicit `Down` that is a documented no-op (re-fingerprinting
      is lossy — the original per-card-number rows can't be reconstructed);
      state that rationale in a comment.
- [x] add the blank import (`_
      "github.com/oneils/ynab-helper/internal/sqlite/migrations/go"`) to
      `sqlite.go` above `runMigrations`.
- [x] update `internal/sqlite/migrations/README.md`: the line "For this
      project, we use SQL-only migrations for simplicity" is no longer true —
      replace it with the actual policy (SQL by default; a Go migration only
      when the transform needs Go logic, as here) and document the
      blank-import requirement so a future Go migration doesn't repeat this
      mistake.
- [x] run tests — must pass before task 4.

### Task 4: Replace the brittle substring-only candidate search

**Files:**
- Modify: `internal/sqlite/pattern_store.go`
- Modify: `internal/sqlite/pattern_store_test.go`

- [x] write test reproducing the exact scenario from the review: a stored
      pattern `"payment card lidl warszawa"` (post-fingerprint) is NOT found by
      today's `LIKE '%'||incoming||'%'` for an incoming `"payment card lidl
      krakow"`, but IS found once the fallback candidate search lands.
- [x] write test: an exact fingerprint match is still returned via the fast
      path (no candidate broadening needed) and ranks first.
- [x] write test: no candidates at all (empty budget / totally unrelated
      description) returns an empty slice, no error.
- [x] implement in `FindPatternsByDescription`: try an exact
      `normalized_description = ?` match first; if that returns nothing, fall
      back to a broadened search using the incoming fingerprint's significant
      tokens (skip short tokens and a named bank/payment stopword list —
      English: "payment", "card", "pos", "purchase", "transaction"; Polish:
      "platnosc", "karta", "transakcja", "przelew", "blik" — take the longest
      3 remaining) OR'd together as `normalized_description LIKE
      '%'||token||'%'`, keeping the existing `ORDER BY occurrence_count DESC,
      last_seen DESC` and capping at a reasonable limit (e.g. 50) for the
      caller's Go-side scoring (task 2's fixed `tokenSimilarity` + task 5's
      confidence threshold) to actually filter noise instead of leaning on
      SQL to do it.
- [x] run tests — must pass before task 5.

### Task 5a: Three-tier confidence threshold, applied on every auto-fill surface

There are **two** places that take a top suggestion unconditionally, not one:
`detailBankTxnHandler` (`handlers.go:551-563`, the detail panel) and
`enrichTransactionList` (`handlers.go:478-502`, the imported-transactions
list — it sets `rows[i].AutoFilled = true` from `payeeSuggestions[0]` /
`catSuggestions[0]` with no confidence check at all). Both need the floor.

**Files:**
- Modify: `internal/txn/suggestions.go`
- Modify: `internal/server/handlers.go` (`enrichTransactionList`,
  `detailBankTxnHandler`, `payeeSuggestionsHandler`,
  `categorySuggestionsHandler`)
- Modify: `internal/txn/suggestions_test.go`, `internal/server/handlers_test.go`

- [x] write tests for new exported constants/helper (e.g.
      `PrefillThreshold = 85`, `SuggestThreshold = 65`) and a helper that
      classifies a `PayeeSuggestion`/`CategorySuggestion` confidence into
      `prefill` / `suggest` / `hide`.
- [x] in `enrichTransactionList`, only set `sugPayeeID`/`rows[i].SugPayee`/
      `rows[i].SugCategory`/`rows[i].AutoFilled` from
      `payeeSuggestions[0]`/`catSuggestions[0]` when confidence is `>=
      PrefillThreshold`; below that, fall through to the existing YNAB-name
      fallback (`suggestPayee`) for payee, and leave `SugCategory` unset.
- [x] in `detailBankTxnHandler` (`handlers.go:551-563`), only set
      `patternPayeeID`/`patternCatID` from `payeeSugs[0]`/`catSugs[0]` when
      their confidence is `>= PrefillThreshold`; below that, leave the field
      empty so `applyYnabPayeeFallback` (or nothing) decides instead of a
      low-confidence guess.
- [x] in `categorySuggestionsHandler`/`payeeSuggestionsHandler`, filter the
      JSON `suggestions` array to `confidence >= SuggestThreshold` before
      returning it (a `< 65` "Possible match" should never reach the UI).
- [x] write test: `enrichTransactionList` leaves `AutoFilled` false and falls
      back to `suggestPayee` when the top learned suggestion is below
      `PrefillThreshold` (this function is already directly unit-tested, see
      `TestEnrichTransactionList_FallbackPayeeMatch` — add a case alongside
      it with an injected low-confidence `getSuggestions`).
- [x] write test: `detailBankTxnHandler` leaves `SugPayeeID` empty when the
      top payee suggestion is below `PrefillThreshold`.
- [x] write test: `categorySuggestionsHandler` omits a sub-`SuggestThreshold`
      suggestion from its JSON response.
- [x] run tests — must pass before task 5b.

### Task 5b: Show suggested categories above the full list, not instead of it

`SearchableSelect` hides the native `<select>` and builds its own dropdown
list from `select.options` (`detail-panel.js:15-19`); a `<optgroup>` in the
template would be flattened away by that code with its label lost, and the
suggestions are injected client-side by `setCategoryOptions` anyway — the
fix belongs entirely in JS, not the template.

**Files:**
- Modify: `ui/static/js/detail-panel.js`

- [x] change `setCategoryOptions`/`fetchCategorySuggestions` so that when
      suggestions come back, it renders a non-selectable "Suggested" header
      row followed by the suggested options, then an "All categories" header
      row followed by `fullCategoryOptions` in full — instead of replacing
      the list wholesale.
- [x] adjust `SearchableSelect`'s list-building (wherever it iterates
      `this.select.options`/renders the dropdown `<ul>`) to skip
      non-selectable header entries when navigating with arrow keys, so
      keyboard use still works.
- [x] manual test (skipped - not automatable; no JS test runner/e2e suite in
      this repo, see Post-Completion for the manual verification note).
- [x] run tests — must pass before task 6.

### Task 6: Category ranking uses description similarity, not just payee frequency

**Files:**
- Modify: `internal/txn/suggestions.go`
- Modify: `internal/txn/suggestions_test.go`

- [x] write test: for a payee with two categories used about equally often
      overall, but where the *current* description's tokens clearly match the
      pattern recorded under category A more closely than category B, A ranks
      first (today `GetCategorySuggestions`'s payee-based branch ignores
      `description` entirely — `suggestions.go:172-177`).
- [x] write test: aggregated evidence beats a single strong outlier — a
      category seen once with a very old/high-count single pattern should NOT
      outrank a category confirmed across five separate, more recent, only
      moderately-matching patterns (today `categoryScores` keeps only the
      *highest single pattern* per category, `suggestions.go:183-194`).
- [x] change the payee-based confidence calculation to also fold in
      `tokenSimilarity(Fingerprint(description), pattern.NormalizedDescription)`
      alongside frequency/recency (weighted, e.g. similarity worth up to 40,
      frequency up to 35, recency up to 25 — i.e. rank by payee match +
      similarity of the current description + category frequency, not
      frequency alone).
- [x] change category aggregation from "keep highest confidence per category"
      to summing/aggregating evidence per category (e.g. sum `occurrence_count`
      across all matching patterns for that category, weighted by each
      pattern's similarity to the current description) before ranking.
- [x] run tests — must pass before task 7.

### Task 7: Invalidate corrected patterns; make `last_seen` monotonic

**Files:**
- Modify: `internal/sqlite/pattern_store.go`
- Modify: `internal/sqlite/pattern_store_test.go`

- [x] write test: `UpsertPattern` for `(budget, fingerprint, payee)` with a
      *different* `category_id` than an existing row deletes the old row
      before inserting/updating the new one (so a stale category can't keep
      surfacing via its old `occurrence_count`).
- [x] write test: re-upserting the same `(budget, fingerprint, payee,
      category)` combination still increments `occurrence_count` as today
      (no regression).
- [x] write test: after a correction deletes the old conflicting row, a
      fresh `UpsertPattern` for the new category starts its
      `occurrence_count` back at 1 (pins the accepted trade-off from
      Decisions — one correction resets accumulated evidence for that
      description+payee — as an explicit, intentional behavior rather than
      an incidental side effect).
- [x] write test: `UpsertPattern` called with an older `last_seen` than the
      existing row's stored value leaves `last_seen` unchanged
      (`last_seen = MAX(existing, new)`), reproducing and fixing the
      re-import-goes-backwards bug at `pattern_store.go:65-73`.
- [x] implement: in `UpsertPattern`, before the existing exists/insert/update
      branch, delete any row matching `budget_id + normalized_description +
      payee_id` with a **different** `category_id` than the incoming pattern.
      In the update branch, use `last_seen = MAX(last_seen, ?)` (SQLite
      `MAX()` in the `SET` expression, or read-compare-write) instead of
      unconditionally overwriting.
- [x] run tests — must pass before task 8.

### Task 8: Realistic bank-description regression suite

**Files:**
- Create: `internal/txn/suggestions_realistic_test.go`

- [x] write end-to-end-style tests against `SuggestionEngine` (using the real
      `Fingerprint()`/`tokenSimilarity()`/`calculateConfidence()`, backed by
      an in-memory `PatternStorer` fake, not a real DB) covering the exact
      gaps the review called out as untested: two card-number variants of the
      same merchant description matching; a payee with genuinely competing
      categories (e.g. "Amazon" split between Groceries/Electronics) ranking
      correctly once a description hints at which; a below-`SuggestThreshold`
      pattern never surfacing; a corrected pattern's old category no longer
      appearing after a conflicting correction (task 7).
- [x] run tests — must pass before task 9.

### Task 9: Verify acceptance criteria
- [x] verify: toggling "remember" no longer sends any network request by
      itself (template no longer has `hx-post` on the checkbox).
- [x] verify: accepting a transaction with the checkbox unchecked leaves
      `payee_patterns` untouched for that transaction.
- [x] verify: accepting with the checkbox checked records the pattern exactly
      once (no double-increment).
- [x] verify all eight review findings have a corresponding fix + test:
      candidate search, normalization/fingerprint, Jaccard correctness,
      confidence threshold, description-aware category ranking, aggregated
      category evidence, correction invalidation, monotonic `last_seen`.
- [x] run full test suite: `make test`.

### Task 10: [Final] Update documentation
- [x] confirm `internal/sqlite/migrations/README.md` reflects the Go-migration
      policy change made in task 3b (not conditional — that edit is required
      by task 3b, this step just verifies it landed).
- [x] confirm `FEATURES.md:31` and `http/api.http` no longer reference
      `save-inline` (required by task 1b — verify here).
- [x] move this plan to `docs/plans/completed/`.

## Technical Details

**New/changed data flow:**
- `Accept & Send to YNAB` button already has `hx-include="#detail-form"`; the
  checkbox becomes a normal field of that form, so `remember_similar` arrives
  in `r.PostForm` on the *same* request as the YNAB upload — no second
  round-trip, no race between "toggle" and "accept".
- `uploadTxnToYnabHandler` order becomes: parse form → `SaveToYnab` → (if
  error, stop) → `UpdatePayeeLastCategory` (unconditional, unrelated YNAB
  fallback feature) → if `remember_similar == "true"`, name-lookup +
  `RecordPattern` → fetch/render updated list. Pattern *learning* is now
  strictly downstream of a confirmed YNAB write; the YNAB-side prefill is not.
- `Fingerprint(description)` replaces `normalize(description)` as the value
  stored in and queried against `payee_patterns.normalized_description`. No
  schema change to that column — same `TEXT` column, new content shape.
- `FindPatternsByDescription`: exact-match fast path, then token-broadened
  fallback, both still ranked/filtered in Go by `calculateConfidence`.

**Confidence tiers** (constants in `internal/txn/suggestions.go`):
- `>= 85`: eligible to prefill the form.
- `65–84`: shown as a suggestion, not auto-filled.
- `< 65`: dropped before it reaches the API response.

**Migration:** `internal/sqlite/migrations/go/00005_refingerprint_patterns.go`,
a Go-coded goose migration registered via a blank import in `sqlite.go`,
applied automatically at startup via the existing `runMigrations`/`goose.Up`
call — no new operational step, no manual command to remember, but the
blank import is required or it silently never runs (see Task 3b).

## Post-Completion

**Manual verification** (no JS test runner / e2e suite in this repo):
- Open a transaction's detail panel, confirm the checkbox is unchecked by
  default, shows the new label/helper copy, and is disabled until both payee
  and category are chosen.
- Confirm checking it and clicking `Accept & Send to YNAB` creates exactly one
  pattern row (check via the DB or a debug log) and that leaving it unchecked
  creates none.
- Confirm the category dropdown in the detail panel shows a "Suggested" group
  above the full "All categories" list rather than replacing it.
- Re-import a previously-imported transaction and confirm `last_seen` for its
  pattern doesn't regress.
