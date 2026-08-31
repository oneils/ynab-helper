package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/oneils/ynab-helper/internal/sqlite"
)

func TestNew_MockYnab_SyncsFromFixtureNotNetwork(t *testing.T) {
	cfg := Config{
		MockYnab: true,
		YnabAPI:  "http://127.0.0.1:1", // unroutable; real HTTP calls would hang/fail
		SQLite:   sqlite.Config{Path: filepath.Join(t.TempDir(), "test.db")},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = app.Close(context.Background()) })

	ctx := context.Background()
	if err := app.Server.Syncer.SyncBudgets(ctx); err != nil {
		t.Fatalf("SyncBudgets() error = %v (should succeed using fake client, never touching %s)", err, cfg.YnabAPI)
	}

	budgets, err := app.Server.Syncer.FetchBudgets(ctx)
	if err != nil {
		t.Fatalf("FetchBudgets() error = %v", err)
	}
	if len(budgets) == 0 {
		t.Fatalf("FetchBudgets() returned no budgets, expected fixture data to be synced into the store")
	}
}

func TestNew_RealClient_ConstructsWithoutNetworkCall(t *testing.T) {
	cfg := Config{
		MockYnab: false,
		YnabAPI:  "http://127.0.0.1:1",
		SQLite:   sqlite.Config{Path: filepath.Join(t.TempDir(), "test.db")},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = app.Close(context.Background()) })
}
