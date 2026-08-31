# Mock YNAB Test Mode

## Overview

Add a local "test mode" that swaps the real YNAB HTTP client for an in-memory
fake client backed by a JSON fixture. This lets the app's UI and main
features (budgets, accounts, categories, payees, transaction upload) be
exercised end-to-end on a developer machine without ever calling the real
YNAB API.

Test mode is opt-in via a new config flag/env var. When enabled:
- `app.New()` wires an `ynab.FakeClient` instead of `ynab.NewClient(...)`.
- The fake client serves data from a built-in default JSON fixture, or from
  a custom fixture file if one is provided.
- Uploads are no-ops (logged, always succeed).
- The UI shows a small "TEST MODE" banner so it's obvious mock data is in
  use.

This integrates cleanly with the existing architecture: `ynab.Syncer`
already depends on the `ynab.YnabClient` interface and `txn.Processor`
already depends on `txn.YnabUploader` — both are satisfied by any type with
matching methods, so no changes are needed to `internal/ynab/sync.go` or
`internal/txn/processor.go`.

## Context (from discovery)

- `internal/ynab/client.go`: concrete `ynab.Client`, makes real HTTP calls
  to the YNAB API (`FetchBudgets`, `FetchAccounts`, `FetchCategories`,
  `FetchPayees`, `Upload`).
- `internal/ynab/sync.go`: defines `YnabClient` interface (already the seam
  used by unit test mocks) and `Syncer`, which wraps the client + storage.
- `internal/txn/processor.go`: defines `YnabUploader` interface (subset:
  `Upload` only) and `Processor`.
- `internal/app/app.go`: `App.New(cfg Config)` wires everything together;
  currently constructs `ynab.NewClient(...)` directly and passes it to both
  `ynab.NewSyncer(...)` and `txn.NewProcessor(...)`.
- `cmd/ynab-helper/main.go`: parses `app.Config` via `jessevdk/go-flags`
  (struct tags `long`/`env`/`default`/`description`), so new config fields
  automatically become CLI flags and env vars.
- `internal/server/handlers.go`: `NewTemplateCache()` builds the template
  cache via `template.New(name).ParseFS(ui.Files, patterns...)` — no
  `FuncMap` currently. `ui/html/index.tmpl.html` defines the shared `base`
  template (sidebar, `<body>`, etc.) that every page renders through.
- Existing unit tests already build hand-rolled mocks named `mockYnabClient`
  etc. inside `_test.go` files (`internal/ynab/sync_test.go`,
  `internal/txn/processor_test.go`). The new fixture-backed fake client is
  a *runtime* component (used by the real binary in test mode), so it lives
  in a regular (non-`_test.go`) file, named `FakeClient` to distinguish it
  from those test-only mocks.
- No `internal/app` tests exist today; `app.New()` wiring is otherwise
  untested.
- `Makefile` has a `run` target (`make run`) that runs the app locally.

## Development Approach

- **Testing approach**: Regular (implement, then add/update tests).
- Complete each task fully before moving to the next.
- Make small, focused changes.
- Every task includes new/updated tests; all tests must pass before moving
  to the next task.
- Update this plan file if scope changes during implementation.

## Testing Strategy

- **Unit tests**: `internal/ynab/fixture_test.go` and
  `internal/ynab/fake_client_test.go` cover fixture loading and the fake
  client's behavior (success + error paths).
- **Wiring test**: `internal/app/app_test.go` verifies `app.New()` wires the
  fake client when mock mode is enabled, by pointing `YnabAPI` at an
  unroutable address and confirming a fetch still succeeds using fixture
  data (proving the real HTTP client was never used).
- **Template test**: extend `internal/server/handlers_test.go` to check the
  `base` template renders the TEST MODE banner when mock mode is on, and
  omits it when off.
- No Playwright/Cypress e2e suite exists in this project; manual UI
  verification is listed under Post-Completion.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.

## Implementation Steps

### Task 1: Add fixture data model + default JSON fixture

**Files:**
- Create: `internal/ynab/fixtures/default.json`
- Create: `internal/ynab/fixture.go`
- Create: `internal/ynab/fixture_test.go`

- [x] define `Fixture` struct in `fixture.go`:
  ```go
  type Fixture struct {
      Budgets    []Budget                `json:"budgets"`
      Categories map[string]CategoryData `json:"categories"` // keyed by budget ID
      Payees     map[string]PayeeData    `json:"payees"`      // keyed by budget ID
  }
  ```
  (accounts are read from each `Budget.Accounts`, so they aren't duplicated
  in a separate map)
- [x] `//go:embed fixtures/default.json` into a package-level `[]byte`
- [x] write `internal/ynab/fixtures/default.json` with realistic sample
  data: 1 budget (`mock-budget-1`, e.g. "Mock Household Budget") with 2
  accounts (checking, savings) nested under `accounts`; 2 category groups
  with 2-3 categories each; 3-4 payees — enough to click through budget
  selection, account list, category/payee dropdowns, and suggestions
- [x] implement `LoadFixture(path string) (Fixture, error)`:
  - `path == ""` → `json.Unmarshal` the embedded default bytes
  - `path != ""` → `os.ReadFile(path)` then `json.Unmarshal`, wrapping
    read/parse errors with context (file path)
- [x] write tests: loading the embedded default succeeds and contains at
  least one budget; loading a valid custom fixture file (via `t.TempDir()`)
  returns its data instead of the default; loading a missing file path
  returns a wrapped error; loading a file with invalid JSON returns a
  wrapped error
- [x] run tests - must pass before task 2

### Task 2: Add FakeClient implementing the YnabClient interface

**Files:**
- Create: `internal/ynab/fake_client.go`
- Create: `internal/ynab/fake_client_test.go`

- [x] `NewFakeClient(fixture Fixture) *FakeClient` storing the fixture
- [x] `FetchBudgets() ([]Budget, error)` returns `fixture.Budgets`, logs via
  `slog.Info` (mirroring the real client's logging)
- [x] `FetchAccounts(req SyncReq) (AccountData, error)`: find the budget by
  `req.BudgetID` in `fixture.Budgets`; return
  `AccountData{Accounts: budget.Accounts}`; if the budget ID isn't found,
  return a descriptive error (e.g. `fmt.Errorf("mock: unknown budget id %q", req.BudgetID)`)
- [x] `FetchCategories(req SyncReq) (CategoryData, error)`: same budget-ID
  lookup pattern against `fixture.Categories`; unknown budget ID → error;
  known budget ID with no fixture entry → empty `CategoryData{}`
- [x] `FetchPayees(req SyncReq) (PayeeData, error)`: same pattern against
  `fixture.Payees`
- [x] `Upload(txn TxnReq) error`: `slog.Info` logging the txn's budget/
  account/amount, always returns `nil` (no-op success, per plan decision)
- [x] add a small unexported helper, e.g. `findBudget(budgets []Budget, id string) (Budget, bool)`,
  used by `FetchAccounts`/`FetchCategories`/`FetchPayees` to validate
  `req.BudgetID` against `fixture.Budgets` *before* looking anything up in
  `fixture.Categories`/`fixture.Payees` — this is what actually
  distinguishes "unknown budget" (helper returns `false` → error) from
  "known budget, no fixture entry for it" (helper returns `true`, map
  lookup misses → return the zero-value `CategoryData{}`/`PayeeData{}`,
  not an error)
- [x] write tests: each `Fetch*` method returns the expected fixture data
  for a known budget ID; each returns an error for an unknown budget ID;
  a known budget ID with no `Categories`/`Payees` map entry returns an
  empty (not error) result; `Upload` always returns `nil` regardless of
  input
- [x] run tests - must pass before task 3

### Task 3: Wire mock mode into app config and startup

**Files:**
- Modify: `internal/app/app.go`
- Create: `internal/app/app_test.go`

- [x] add to `Config` in `app.go`:
  ```go
  MockYnab     bool   `long:"mock-ynab" env:"MOCK_YNAB" description:"Use an in-memory mock YNAB client instead of the real YNAB API (for local UI testing)"`
  MockDataFile string `long:"mock-data-file" env:"MOCK_DATA_FILE" description:"Path to a JSON fixture overriding the built-in mock YNAB data (only used with --mock-ynab)"`
  ```
- [x] in `New(cfg Config)`, replace the direct `ynab.NewClient(...)` call
  with a branch: if `cfg.MockYnab`, call `ynab.LoadFixture(cfg.MockDataFile)`
  and `ynab.NewFakeClient(fixture)` (return a wrapped error if the fixture
  fails to load); otherwise construct the real `ynab.NewClient(...)` as
  today. Hold the result in an `ynab.YnabClient`-typed variable and pass it
  to both `ynab.NewSyncer(...)` and `txn.NewProcessor(...)` (both already
  accept it structurally)
- [x] log at startup which mode was chosen (`slog.Info("YNAB client mode", "mock", cfg.MockYnab)`)
- [x] load the fixture (`ynab.LoadFixture`) before calling `sqlite.New(...)`
  so a bad fixture path fails fast without leaking an open DB connection
- [x] write `internal/app/app_test.go`: call `New()` with
  `MockYnab: true`, `YnabAPI` set to an unroutable address (e.g.
  `http://127.0.0.1:1`), and `SQLite.Path` under `t.TempDir()`; then call
  `app.Server.Syncer.SyncBudgets(ctx)` — the method that actually calls
  `client.FetchBudgets()` (unlike `Syncer.FetchBudgets`, which only reads
  the local DB and would pass trivially against an empty store) — and
  assert it returns no error; then call `app.Server.Syncer.FetchBudgets(ctx)`
  and assert the fixture's budget ID(s) came back from the store,
  confirming the sync round-tripped through the fake client and never
  touched `http://127.0.0.1:1`; also test the non-mock path still
  constructs successfully with `MockYnab: false` (no network call needed
  for construction itself)
- [x] run tests - must pass before task 4

### Task 4: Show a "TEST MODE" banner in the UI

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `internal/app/app.go`
- Modify: `ui/html/index.tmpl.html`
- Modify: `ui/static/css/main.css`
- Modify: `internal/server/handlers_test.go`

- [x] change `NewTemplateCache()` to `NewTemplateCache(mockMode bool) (map[string]*template.Template, error)`, adding
  `.Funcs(template.FuncMap{"mockMode": func() bool { return mockMode }})`
  before `.ParseFS(...)` on the template builder
- [x] update the call site in `app.go` to
  `server.NewTemplateCache(cfg.MockYnab)`
- [x] in `ui/html/index.tmpl.html`, add the banner inside `<div class="main-wrapper">`,
  immediately *before* `<main class="main-content">` (not above
  `.app-container`/`<body>` — `.sidebar` is `position: sticky; height: 100vh`
  and a sibling of `.main-wrapper` inside the flex `.app-container`; a bar
  placed above `.app-container` would push the whole shell taller than the
  viewport and detach the sidebar from the top). Content inside
  `.main-wrapper` (itself `display:flex; flex-direction:column`) can grow
  normally without affecting the sidebar:
  ```html
  {{if mockMode}}<div class="mock-mode-banner">🧪 TEST MODE — YNAB API is mocked, no real data will be synced</div>{{end}}
  <main class="main-content">{{template "main" .}}</main>
  ```
- [x] add a small, visually distinct `.mock-mode-banner` style to
  `ui/static/css/main.css` (e.g. a full-width amber/yellow bar)
- [x] update `internal/server/handlers_test.go` call sites of
  `NewTemplateCache()` to `NewTemplateCache(false)`
- [x] write a test rendering `cache["about.tmpl.html"]`'s `base` template
  (the `about` page's `main` block and its partials render with `nil`
  data, so it's safe to execute without constructing a full data struct)
  with `NewTemplateCache(true)` and asserting the output contains the
  banner text; render the same template from `NewTemplateCache(false)`
  and assert it does not
- [x] run tests - must pass before task 5

### Task 5: [Final] Update documentation

**Files:**
- Modify: `README.md`
- Modify: `Makefile`

- [x] add a `run-mock` Makefile target that points at a **separate** SQLite
  file so mock data never lands in the real dev database:
  `go run -ldflags "-X main.revision=dev" ./cmd/ynab-helper --addr=:5002 --mock-ynab --sqlite-path=./data/ynab-mock.db`
  (the startup sync in `ynab.Scheduler.Start` runs unconditionally and
  upserts every fixture budget/account/category/payee into whatever
  `--sqlite-path` points at, so without this the fixture data — and any
  transactions "uploaded" through the no-op mock — would be written into
  `./data/ynab.db`, the same file `make run` uses)
- [x] add `MOCK_YNAB` and `MOCK_DATA_FILE` rows to the existing
  Configuration table in `README.md`, plus a short "Test mode" subsection
  near "Running locally" pointing at `make run-mock` and explaining the
  separate `ynab-mock.db` file and how to write a custom fixture (point at
  the `Fixture` struct shape / the default JSON as an example)
- [x] verify all requirements from Overview are implemented
- [x] run full test suite: `make test`
- [x] run `make lint`
- [x] move this plan to `docs/plans/completed/`

## Technical Details

### `Fixture` JSON shape (`internal/ynab/fixtures/default.json`)

```json
{
  "budgets": [
    {
      "id": "mock-budget-1",
      "name": "Mock Household Budget",
      "last_modified_on": "2026-08-01T00:00:00Z",
      "first_month": "2026-01-01",
      "last_month": "2026-12-01",
      "date_format": { "format": "DD/MM/YYYY" },
      "currency_format": {
        "iso_code": "USD",
        "example_format": "123,456.78",
        "decimal_digits": 2,
        "decimal_separator": ".",
        "symbol_first": true,
        "group_separator": ",",
        "currency_symbol": "$",
        "display_symbol": true
      },
      "accounts": [
        { "id": "mock-acc-checking", "name": "Checking", "type": "checking", "on_budget": true, "balance": 500000, "cleared_balance": 500000, "uncleared_balance": 0 },
        { "id": "mock-acc-savings", "name": "Savings", "type": "savings", "on_budget": true, "balance": 1200000, "cleared_balance": 1200000, "uncleared_balance": 0 }
      ]
    }
  ],
  "categories": {
    "mock-budget-1": {
      "category_groups": [
        {
          "id": "mock-cg-1",
          "name": "Everyday Expenses",
          "categories": [
            { "id": "mock-cat-groceries", "category_group_id": "mock-cg-1", "name": "Groceries", "budgeted": 50000, "activity": -12000, "balance": 38000 },
            { "id": "mock-cat-transport", "category_group_id": "mock-cg-1", "name": "Transport", "budgeted": 20000, "activity": -5000, "balance": 15000 }
          ]
        },
        {
          "id": "mock-cg-2",
          "name": "Bills",
          "categories": [
            { "id": "mock-cat-rent", "category_group_id": "mock-cg-2", "name": "Rent", "budgeted": 150000, "activity": -150000, "balance": 0 }
          ]
        }
      ]
    }
  },
  "payees": {
    "mock-budget-1": {
      "payees": [
        { "id": "mock-payee-grocer", "name": "Local Grocer" },
        { "id": "mock-payee-landlord", "name": "Landlord" }
      ]
    }
  }
}
```

`CategoryData` and `PayeeData` already have `ServerKnowledge int64` fields;
omitting them in the fixture JSON leaves them at the zero value, which is
fine since `Syncer` only compares/stores it, never divides by it.

### Config additions (`internal/app/app.go`)

- `MockYnab bool` — default `false`, so production behavior is unchanged
  unless explicitly opted in.
- `MockDataFile string` — default `""`, meaning "use the embedded default
  fixture".

### Wiring flow

```
app.New(cfg)
  -> if cfg.MockYnab:
       fixture, err := ynab.LoadFixture(cfg.MockDataFile)
       ynabClient = ynab.NewFakeClient(fixture)
     else:
       ynabClient = ynab.NewClient(cfg.YnabToken, cfg.YnabAPI, httpClient)
  -> ynab.NewSyncer(ynabClient, ...)
  -> txn.NewProcessor(..., ynabClient, ...)
  -> server.NewTemplateCache(cfg.MockYnab)
```

Handlers read budgets/accounts/categories/payees from SQLite, not directly
from the client — so fixture data only shows up in the UI after
`ynab.Scheduler.Start` runs its unconditional startup sync (`Syncer.SyncBudgets`
etc.), which pulls it from `FakeClient` through `Syncer` into the store,
same as a real sync would.

## Post-Completion

**Manual verification**:
- run `make run-mock`, open the UI, and click through: home page budget
  list, accounts page, importing a sample bank CSV, previewing/uploading a
  transaction (confirm no real network call is made and the "TEST MODE"
  banner is visible throughout), settings/parser-mapping pages, payee and
  category suggestion autocomplete.
- confirm `make run` (without `--mock-ynab`) still requires/uses a real
  `YNAB_TOKEN` and behaves exactly as before (no regression).
- after a `make run-mock` session, confirm `./data/ynab.db` (the real
  dev database used by plain `make run`) is untouched — mock data should
  only ever land in `./data/ynab-mock.db`.
