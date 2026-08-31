package ynab

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFixture_Default(t *testing.T) {
	fixture, err := LoadFixture("")
	if err != nil {
		t.Fatalf("LoadFixture(\"\") returned error: %v", err)
	}
	if len(fixture.Budgets) == 0 {
		t.Fatal("expected at least one budget in default fixture")
	}
}

func TestLoadFixture_CustomFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.json")
	custom := `{
		"budgets": [{"id": "custom-budget", "name": "Custom Budget"}],
		"categories": {},
		"payees": {}
	}`
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatalf("failed to write custom fixture file: %v", err)
	}

	fixture, err := LoadFixture(path)
	if err != nil {
		t.Fatalf("LoadFixture(%q) returned error: %v", path, err)
	}
	if len(fixture.Budgets) != 1 || fixture.Budgets[0].ID != "custom-budget" {
		t.Fatalf("expected custom fixture data, got %+v", fixture.Budgets)
	}
}

func TestLoadFixture_MissingFile(t *testing.T) {
	_, err := LoadFixture(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatal("expected error for missing fixture file, got nil")
	}
}

func TestLoadFixture_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("failed to write invalid fixture file: %v", err)
	}

	_, err := LoadFixture(path)
	if err == nil {
		t.Fatal("expected error for invalid fixture JSON, got nil")
	}
}
