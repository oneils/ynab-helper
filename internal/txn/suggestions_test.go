package txn

import (
	"context"
	"testing"
	"time"
)

type mockPatternStore struct {
	byPayeeIDCalled       bool
	byDescriptionCalled   bool
	patternsByPayeeID     []PayeePattern
	patternsByDescription []PayeePattern
}

func (m *mockPatternStore) FindPatternsByDescription(ctx context.Context, budgetID, normalizedDesc string, limit int) ([]PayeePattern, error) {
	m.byDescriptionCalled = true
	return m.patternsByDescription, nil
}

func (m *mockPatternStore) FindPatternsByPayeeID(ctx context.Context, budgetID, payeeID string, limit int) ([]PayeePattern, error) {
	m.byPayeeIDCalled = true
	return m.patternsByPayeeID, nil
}

func (m *mockPatternStore) UpsertPattern(ctx context.Context, p PayeePattern) error {
	return nil
}

func TestGetCategorySuggestions_WithPayeeID_CallsFindPatternsByPayeeID(t *testing.T) {
	store := &mockPatternStore{
		patternsByPayeeID: []PayeePattern{
			{PayeeID: "payee1", CategoryID: "cat1", CategoryName: "Groceries", OccurrenceCount: 5, LastSeen: time.Now()},
		},
	}
	engine := NewSuggestionEngine(store)

	suggestions, err := engine.GetCategorySuggestions(context.Background(), "budget1", "some desc", "payee1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.byPayeeIDCalled {
		t.Fatal("expected FindPatternsByPayeeID to be called")
	}
	if store.byDescriptionCalled {
		t.Fatal("expected FindPatternsByDescription NOT to be called when payee patterns found")
	}
	if len(suggestions) != 1 || suggestions[0].CategoryID != "cat1" {
		t.Fatalf("unexpected suggestions: %+v", suggestions)
	}
}

func TestGetCategorySuggestions_EmptyPayeeID_SkipsStrategy1(t *testing.T) {
	store := &mockPatternStore{
		patternsByDescription: []PayeePattern{
			{PayeeID: "payee2", CategoryID: "cat2", CategoryName: "Dining", OccurrenceCount: 2, LastSeen: time.Now()},
		},
	}
	engine := NewSuggestionEngine(store)

	suggestions, err := engine.GetCategorySuggestions(context.Background(), "budget1", "some desc", "", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.byPayeeIDCalled {
		t.Fatal("expected FindPatternsByPayeeID NOT to be called with empty payeeID")
	}
	if !store.byDescriptionCalled {
		t.Fatal("expected FindPatternsByDescription to be called as fallback")
	}
	if len(suggestions) != 1 || suggestions[0].CategoryID != "cat2" {
		t.Fatalf("unexpected suggestions: %+v", suggestions)
	}
}

func TestGetCategorySuggestions_PayeeIDNoMatch_FallsBackToDescription(t *testing.T) {
	store := &mockPatternStore{
		patternsByDescription: []PayeePattern{
			{PayeeID: "payee3", CategoryID: "cat3", CategoryName: "Utilities", OccurrenceCount: 1, LastSeen: time.Now()},
		},
	}
	engine := NewSuggestionEngine(store)

	suggestions, err := engine.GetCategorySuggestions(context.Background(), "budget1", "some desc", "payee1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.byPayeeIDCalled {
		t.Fatal("expected FindPatternsByPayeeID to be called")
	}
	if !store.byDescriptionCalled {
		t.Fatal("expected FindPatternsByDescription to be called as fallback when payee patterns empty")
	}
	if len(suggestions) != 1 || suggestions[0].CategoryID != "cat3" {
		t.Fatalf("unexpected suggestions: %+v", suggestions)
	}
}

func TestGetCategorySuggestions_NoPatterns_ReturnsEmpty(t *testing.T) {
	store := &mockPatternStore{}
	engine := NewSuggestionEngine(store)

	suggestions, err := engine.GetCategorySuggestions(context.Background(), "budget1", "some desc", "payee1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("expected empty suggestions, got: %+v", suggestions)
	}
}
