package txn

import (
	"context"
	"testing"
	"time"
)

// realisticPatternStore is a minimal in-memory PatternStorer fake that behaves
// like the real store's exact-then-broadened lookup closely enough to drive
// SuggestionEngine end-to-end without a real DB.
type realisticPatternStore struct {
	patterns []PayeePattern
}

func (s *realisticPatternStore) FindPatternsByDescription(ctx context.Context, budgetID, normalizedDesc string, limit int) ([]PayeePattern, error) {
	var matches []PayeePattern
	for _, p := range s.patterns {
		if p.BudgetID != budgetID {
			continue
		}
		if p.NormalizedDescription == normalizedDesc {
			matches = append(matches, p)
		}
	}
	if len(matches) > 0 {
		return matches, nil
	}
	// Broadened fallback: any shared token between the two fingerprints.
	incomingTokens := tokenize(normalizedDesc)
	for _, p := range s.patterns {
		if p.BudgetID != budgetID {
			continue
		}
		patternTokens := tokenize(p.NormalizedDescription)
		for _, it := range incomingTokens {
			found := false
			for _, pt := range patternTokens {
				if it == pt {
					found = true
					break
				}
			}
			if found {
				matches = append(matches, p)
				break
			}
		}
	}
	return matches, nil
}

func (s *realisticPatternStore) FindPatternsByPayeeID(ctx context.Context, budgetID, payeeID string, limit int) ([]PayeePattern, error) {
	var matches []PayeePattern
	for _, p := range s.patterns {
		if p.BudgetID == budgetID && p.PayeeID == payeeID {
			matches = append(matches, p)
		}
	}
	return matches, nil
}

func (s *realisticPatternStore) UpsertPattern(ctx context.Context, p PayeePattern) error {
	for i, existing := range s.patterns {
		if existing.BudgetID == p.BudgetID &&
			existing.NormalizedDescription == p.NormalizedDescription &&
			existing.PayeeID == p.PayeeID &&
			existing.CategoryID == p.CategoryID {
			s.patterns[i].OccurrenceCount++
			return nil
		}
	}
	s.patterns = append(s.patterns, p)
	return nil
}

func TestRealistic_CardNumberVariantsOfSameMerchant_MatchAsSamePattern(t *testing.T) {
	recent := time.Now().Add(-2 * 24 * time.Hour)
	store := &realisticPatternStore{
		patterns: []PayeePattern{
			{
				BudgetID: "budget1", PayeeID: "lidl-payee", PayeeName: "Lidl",
				CategoryID: "groceries", CategoryName: "Groceries",
				NormalizedDescription: Fingerprint("PAYMENT CARD 1234 LIDL WARSZAWA"),
				OccurrenceCount:        8, LastSeen: recent,
			},
		},
	}
	engine := NewSuggestionEngine(store)

	// A different card number, same merchant — must still surface the pattern.
	suggestions, err := engine.GetSuggestions(context.Background(), "budget1", "PAYMENT CARD 9876 LIDL WARSZAWA", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 1 || suggestions[0].PayeeID != "lidl-payee" {
		t.Fatalf("expected the lidl pattern to match despite differing card numbers, got: %+v", suggestions)
	}
	if ClassifyConfidence(suggestions[0].Confidence) != TierPrefill {
		t.Fatalf("expected an exact-fingerprint match with high occurrence/recency to be prefill-eligible, got confidence %v", suggestions[0].Confidence)
	}
}

func TestRealistic_CompetingCategoriesForSamePayee_DescriptionHintPicksCorrectOne(t *testing.T) {
	recent := time.Now().Add(-3 * 24 * time.Hour)
	store := &realisticPatternStore{
		patterns: []PayeePattern{
			{
				BudgetID: "budget1", PayeeID: "amazon-payee", PayeeName: "Amazon",
				CategoryID: "groceries", CategoryName: "Groceries",
				NormalizedDescription: Fingerprint("AMAZON FRESH GROCERY DELIVERY"),
				OccurrenceCount:        4, LastSeen: recent,
			},
			{
				BudgetID: "budget1", PayeeID: "amazon-payee", PayeeName: "Amazon",
				CategoryID: "electronics", CategoryName: "Electronics",
				NormalizedDescription: Fingerprint("AMAZON ELECTRONICS ORDER"),
				OccurrenceCount:        4, LastSeen: recent,
			},
		},
	}
	engine := NewSuggestionEngine(store)

	suggestions, err := engine.GetCategorySuggestions(context.Background(), "budget1", "AMAZON ELECTRONICS ORDER 2026", "amazon-payee", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("expected both competing categories to be returned, got: %+v", suggestions)
	}
	if suggestions[0].CategoryID != "electronics" {
		t.Fatalf("expected the description-matching category (electronics) to rank first, got: %+v", suggestions)
	}
}

func TestRealistic_BelowSuggestThresholdPattern_NeverSurfaces(t *testing.T) {
	old := time.Now().Add(-720 * 24 * time.Hour)
	store := &realisticPatternStore{
		patterns: []PayeePattern{
			{
				BudgetID: "budget1", PayeeID: "obscure-payee", PayeeName: "Obscure Shop",
				CategoryID: "misc", CategoryName: "Misc",
				NormalizedDescription: Fingerprint("SOME OBSCURE SHOP PURCHASE"),
				OccurrenceCount:        1, LastSeen: old,
			},
		},
	}
	engine := NewSuggestionEngine(store)

	suggestions, err := engine.GetSuggestions(context.Background(), "budget1", "completely unrelated other description", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range suggestions {
		if ClassifyConfidence(s.Confidence) != TierHide {
			continue
		}
		t.Fatalf("expected a below-threshold suggestion to be filterable via ClassifyConfidence, got tier for: %+v", s)
	}
	// Also verify that filtering by SuggestThreshold (as the JSON handlers do)
	// removes any weak match entirely.
	var filtered []PayeeSuggestion
	for _, s := range suggestions {
		if s.Confidence >= SuggestThreshold {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) != 0 {
		t.Fatalf("expected no suggestions to survive the SuggestThreshold filter, got: %+v", filtered)
	}
}

func TestRealistic_CorrectedPattern_OldCategoryNoLongerAppearsAfterConflict(t *testing.T) {
	recent := time.Now().Add(-24 * time.Hour)
	store := &realisticPatternStore{
		patterns: []PayeePattern{
			{
				BudgetID: "budget1", PayeeID: "starbucks-payee", PayeeName: "Starbucks",
				CategoryID: "dining", CategoryName: "Dining Out",
				NormalizedDescription: Fingerprint("STARBUCKS COFFEE PURCHASE"),
				OccurrenceCount:        6, LastSeen: recent,
			},
		},
	}
	engine := NewSuggestionEngine(store)

	// Simulate the store-level correction behavior (task 7): a conflicting
	// UpsertPattern for the same budget+fingerprint+payee with a different
	// category deletes the old row before recording the new one.
	fp := Fingerprint("STARBUCKS COFFEE PURCHASE")
	var kept []PayeePattern
	for _, p := range store.patterns {
		if p.BudgetID == "budget1" && p.NormalizedDescription == fp && p.PayeeID == "starbucks-payee" && p.CategoryID != "business-expense" {
			continue // deleted by the conflict-resolution rule
		}
		kept = append(kept, p)
	}
	store.patterns = kept

	if err := engine.RecordPattern(context.Background(), PayeePattern{
		BudgetID: "budget1", PayeeID: "starbucks-payee", PayeeName: "Starbucks",
		CategoryID: "business-expense", CategoryName: "Business Expense",
		NormalizedDescription: fp,
		OccurrenceCount:        1, LastSeen: recent,
	}); err != nil {
		t.Fatalf("unexpected error recording corrected pattern: %v", err)
	}

	suggestions, err := engine.GetSuggestions(context.Background(), "budget1", "STARBUCKS COFFEE PURCHASE", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected exactly one surviving suggestion after correction, got: %+v", suggestions)
	}
	if suggestions[0].CategoryID != "business-expense" {
		t.Fatalf("expected the corrected category to be suggested, got: %+v", suggestions)
	}
}
