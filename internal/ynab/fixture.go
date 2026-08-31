package ynab

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
)

//go:embed fixtures/default.json
var defaultFixtureJSON []byte

// Fixture is the data model used by FakeClient to serve mock YNAB data.
type Fixture struct {
	Budgets    []Budget                `json:"budgets"`
	Categories map[string]CategoryData `json:"categories"` // keyed by budget ID
	Payees     map[string]PayeeData    `json:"payees"`     // keyed by budget ID
}

// LoadFixture loads a Fixture. If path is empty, the built-in default
// fixture is used. Otherwise, the fixture is read and parsed from path.
func LoadFixture(path string) (Fixture, error) {
	data := defaultFixtureJSON
	if path != "" {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return Fixture{}, fmt.Errorf("reading mock fixture file %q: %w", path, err)
		}
	}

	var fixture Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		src := "embedded default fixture"
		if path != "" {
			src = fmt.Sprintf("mock fixture file %q", path)
		}
		return Fixture{}, fmt.Errorf("parsing %s: %w", src, err)
	}

	return fixture, nil
}
