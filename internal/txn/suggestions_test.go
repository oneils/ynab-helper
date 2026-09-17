package txn

import (
	"context"
	"strings"
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

func TestGetCategorySuggestions_PayeeBased_DescriptionSimilarityBreaksTie(t *testing.T) {
	now := time.Now().Add(-24 * time.Hour)
	store := &mockPatternStore{
		patternsByPayeeID: []PayeePattern{
			{
				PayeeID: "payee1", CategoryID: "cat-a", CategoryName: "Groceries",
				NormalizedDescription: "lidl warszawa groceries",
				OccurrenceCount:       5, LastSeen: now,
			},
			{
				PayeeID: "payee1", CategoryID: "cat-b", CategoryName: "Electronics",
				NormalizedDescription: "electronics store krakow",
				OccurrenceCount:       5, LastSeen: now,
			},
		},
	}
	engine := NewSuggestionEngine(store)

	suggestions, err := engine.GetCategorySuggestions(context.Background(), "budget1", "lidl warszawa purchase card", "payee1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got: %+v", suggestions)
	}
	if suggestions[0].CategoryID != "cat-a" {
		t.Fatalf("expected the description-similar category (cat-a) to rank first, got %+v", suggestions)
	}
}

func TestGetCategorySuggestions_AggregatedEvidenceBeatsSingleOutlier(t *testing.T) {
	recent := time.Now().Add(-24 * time.Hour)
	old := time.Now().Add(-730 * 24 * time.Hour)

	patterns := []PayeePattern{
		{
			PayeeID: "payee1", CategoryID: "cat-outlier", CategoryName: "Outlier",
			NormalizedDescription: "lidl unrelated stuff here now",
			OccurrenceCount:       50, LastSeen: old,
		},
	}
	for i := 0; i < 5; i++ {
		patterns = append(patterns, PayeePattern{
			PayeeID: "payee1", CategoryID: "cat-confirmed", CategoryName: "Confirmed",
			NormalizedDescription: "lidl warszawa grocery run",
			OccurrenceCount:       3, LastSeen: recent,
		})
	}

	store := &mockPatternStore{patternsByPayeeID: patterns}
	engine := NewSuggestionEngine(store)

	suggestions, err := engine.GetCategorySuggestions(context.Background(), "budget1", "lidl warszawa grocery zakupy monday", "payee1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got: %+v", suggestions)
	}
	if suggestions[0].CategoryID != "cat-confirmed" {
		t.Fatalf("expected the category confirmed across 5 patterns to outrank the old high-count outlier, got %+v", suggestions)
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

func TestTokenSimilarity_RepeatedTokenInSecondString_NeverExceedsOne(t *testing.T) {
	similarity := tokenSimilarity("lidl warszawa", "lidl lidl lidl")
	if similarity > 1.0 {
		t.Fatalf("similarity must never exceed 1.0, got: %v", similarity)
	}
	// unique tokens: {lidl, warszawa} vs {lidl} -> intersection=1, union=2
	if similarity != 0.5 {
		t.Fatalf("expected 0.5, got: %v", similarity)
	}
}

func TestTokenSimilarity_IdenticalStrings_ReturnsOne(t *testing.T) {
	similarity := tokenSimilarity("lidl warszawa", "lidl warszawa")
	if similarity != 1.0 {
		t.Fatalf("expected 1.0, got: %v", similarity)
	}
}

func TestTokenSimilarity_DisjointStrings_ReturnsZero(t *testing.T) {
	similarity := tokenSimilarity("lidl warszawa", "biedronka krakow")
	if similarity != 0 {
		t.Fatalf("expected 0, got: %v", similarity)
	}
}

func TestTokenSimilarity_PartialOverlap_ReturnsExactRatio(t *testing.T) {
	// unique tokens: {payment, card, lidl, warszawa} vs {payment, card, lidl, krakow}
	// intersection={payment, card, lidl}=3, union=5
	similarity := tokenSimilarity("payment card lidl warszawa", "payment card lidl krakow")
	if similarity != 0.6 {
		t.Fatalf("expected 0.6, got: %v", similarity)
	}
}

func TestFingerprint_Lowercases(t *testing.T) {
	if got := Fingerprint("LIDL Warszawa"); got != "lidl warszawa" {
		t.Fatalf("expected lowercased, got: %q", got)
	}
}

func TestFingerprint_FoldsPolishDiacritics(t *testing.T) {
	if got := Fingerprint("Żabka Łódź"); got != "zabka lodz" {
		t.Fatalf("expected diacritics folded, got: %q", got)
	}
}

func TestFingerprint_CollapsesPunctuationToSpaces(t *testing.T) {
	if got := Fingerprint("LIDL, Warszawa."); got != "lidl warszawa" {
		t.Fatalf("expected punctuation collapsed, got: %q", got)
	}
}

func TestFingerprint_CollapsesRepeatedWhitespace(t *testing.T) {
	if got := Fingerprint("LIDL    Warszawa"); got != "lidl warszawa" {
		t.Fatalf("expected whitespace collapsed, got: %q", got)
	}
}

func TestFingerprint_StripsMaskedCardNumbers(t *testing.T) {
	if got := Fingerprint("PAYMENT CARD **** 1234 LIDL WARSZAWA"); got != "payment card lidl warszawa" {
		t.Fatalf("expected masked card number stripped, got: %q", got)
	}
}

func TestFingerprint_StripsFullCardNumbers(t *testing.T) {
	if got := Fingerprint("CARD 1234 5678 9012 3456 LIDL"); got != "card lidl" {
		t.Fatalf("expected full card number stripped, got: %q", got)
	}
}

func TestFingerprint_StripsISODates(t *testing.T) {
	if got := Fingerprint("LIDL WARSZAWA 2026-09-16"); got != "lidl warszawa" {
		t.Fatalf("expected ISO date stripped, got: %q", got)
	}
}

func TestFingerprint_StripsEUDates(t *testing.T) {
	if got := Fingerprint("LIDL WARSZAWA 16.09.2026"); got != "lidl warszawa" {
		t.Fatalf("expected EU date stripped, got: %q", got)
	}
}

func TestFingerprint_StripsLongReferenceNumbers(t *testing.T) {
	if got := Fingerprint("LIDL WARSZAWA REF 123456789"); got != "lidl warszawa ref" {
		t.Fatalf("expected long reference number stripped, got: %q", got)
	}
}

func TestFingerprint_PreservesMerchantTokensAcrossCardVariants(t *testing.T) {
	a := Fingerprint("PAYMENT CARD 1234 LIDL WARSZAWA")
	b := Fingerprint("PAYMENT CARD 5678 LIDL KRAKOW")

	if !strings.Contains(a, "lidl") || !strings.Contains(b, "lidl") {
		t.Fatalf("expected merchant token preserved: a=%q b=%q", a, b)
	}
	if strings.Contains(a, "1234") || strings.Contains(b, "5678") {
		t.Fatalf("expected card numbers stripped: a=%q b=%q", a, b)
	}
}

func TestClassifyConfidence_AtOrAbovePrefillThreshold_ReturnsTierPrefill(t *testing.T) {
	if got := ClassifyConfidence(85); got != TierPrefill {
		t.Fatalf("expected TierPrefill at threshold, got %v", got)
	}
	if got := ClassifyConfidence(100); got != TierPrefill {
		t.Fatalf("expected TierPrefill above threshold, got %v", got)
	}
}

func TestClassifyConfidence_BetweenThresholds_ReturnsTierSuggest(t *testing.T) {
	if got := ClassifyConfidence(65); got != TierSuggest {
		t.Fatalf("expected TierSuggest at suggest threshold, got %v", got)
	}
	if got := ClassifyConfidence(84.9); got != TierSuggest {
		t.Fatalf("expected TierSuggest just below prefill threshold, got %v", got)
	}
}

func TestClassifyConfidence_BelowSuggestThreshold_ReturnsTierHide(t *testing.T) {
	if got := ClassifyConfidence(64.9); got != TierHide {
		t.Fatalf("expected TierHide below suggest threshold, got %v", got)
	}
	if got := ClassifyConfidence(0); got != TierHide {
		t.Fatalf("expected TierHide at zero, got %v", got)
	}
}
