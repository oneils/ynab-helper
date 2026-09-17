package txn

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Confidence thresholds for suggestion surfaces.
const (
	// PrefillThreshold is the minimum confidence at which a suggestion is
	// trustworthy enough to auto-fill a form field.
	PrefillThreshold = 85.0
	// SuggestThreshold is the minimum confidence at which a suggestion is
	// worth showing to the user at all (e.g. in a suggestions list), even
	// though it isn't confident enough to auto-fill.
	SuggestThreshold = 65.0
)

// ConfidenceTier classifies a suggestion's confidence score.
type ConfidenceTier int

const (
	// TierHide means the confidence is too low to show the suggestion.
	TierHide ConfidenceTier = iota
	// TierSuggest means the confidence is high enough to show but not to auto-fill.
	TierSuggest
	// TierPrefill means the confidence is high enough to auto-fill a form field.
	TierPrefill
)

// ClassifyConfidence buckets a confidence score into a ConfidenceTier.
func ClassifyConfidence(confidence float64) ConfidenceTier {
	switch {
	case confidence >= PrefillThreshold:
		return TierPrefill
	case confidence >= SuggestThreshold:
		return TierSuggest
	default:
		return TierHide
	}
}

// PayeePattern represents a learned pattern for payee-category matching.
type PayeePattern struct {
	ID                    int64
	BudgetID              string
	NormalizedDescription string
	PayeeID               string
	PayeeName             string
	CategoryID            string
	CategoryName          string
	OccurrenceCount       int
	LastSeen              time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// PayeeSuggestion represents a suggested payee with confidence score.
type PayeeSuggestion struct {
	PayeeID      string  `json:"payee_id"`
	PayeeName    string  `json:"payee_name"`
	CategoryID   string  `json:"category_id"`
	CategoryName string  `json:"category_name"`
	Confidence   float64 `json:"confidence"`
	Reason       string  `json:"reason"`
}

// CategorySuggestion represents a suggested category with confidence score.
type CategorySuggestion struct {
	CategoryID   string  `json:"category_id"`
	CategoryName string  `json:"category_name"`
	Confidence   float64 `json:"confidence"`
	Reason       string  `json:"reason"`
	Source       string  `json:"source"` // "payee" or "description"
}

// SuggestionEngine generates intelligent payee/category suggestions.
type SuggestionEngine struct {
	patternStore PatternStorer
}

// PatternStorer defines pattern lookup operations.
type PatternStorer interface {
	FindPatternsByDescription(ctx context.Context, budgetID, normalizedDesc string, limit int) ([]PayeePattern, error)
	FindPatternsByPayeeID(ctx context.Context, budgetID, payeeID string, limit int) ([]PayeePattern, error)
	UpsertPattern(ctx context.Context, p PayeePattern) error
}

// NewSuggestionEngine creates a new suggestion engine.
func NewSuggestionEngine(patternStore PatternStorer) *SuggestionEngine {
	return &SuggestionEngine{patternStore: patternStore}
}

// GetSuggestions returns ranked payee suggestions for a transaction description.
func (e *SuggestionEngine) GetSuggestions(ctx context.Context,
	budgetID, description string, limit int) ([]PayeeSuggestion, error) {

	normalizedDesc := Fingerprint(description)

	// Find patterns matching the description
	patterns, err := e.patternStore.FindPatternsByDescription(ctx, budgetID, normalizedDesc, 20)
	if err != nil {
		return nil, err
	}

	if len(patterns) == 0 {
		return []PayeeSuggestion{}, nil
	}

	// Score patterns and deduplicate by payee
	payeeScores := make(map[string]*PayeeSuggestion)

	for _, pattern := range patterns {
		// Calculate confidence score
		confidence := calculateConfidence(normalizedDesc, pattern)

		// Keep highest confidence per payee
		existing, exists := payeeScores[pattern.PayeeID]
		if !exists || confidence > existing.Confidence {
			payeeScores[pattern.PayeeID] = &PayeeSuggestion{
				PayeeID:      pattern.PayeeID,
				PayeeName:    pattern.PayeeName,
				CategoryID:   pattern.CategoryID,
				CategoryName: pattern.CategoryName,
				Confidence:   confidence,
				Reason:       buildReason(pattern, confidence),
			}
		}
	}

	// Convert map to sorted slice
	var suggestions []PayeeSuggestion
	for _, sugg := range payeeScores {
		suggestions = append(suggestions, *sugg)
	}

	// Sort by confidence (descending)
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})

	// Limit results
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	return suggestions, nil
}

// RecordPattern records a payee-category pattern for learning.
func (e *SuggestionEngine) RecordPattern(ctx context.Context, pattern PayeePattern) error {
	return e.patternStore.UpsertPattern(ctx, pattern)
}

// GetCategorySuggestions returns ranked category suggestions.
// Prioritizes payee-based suggestions if payeeID is provided.
func (e *SuggestionEngine) GetCategorySuggestions(ctx context.Context,
	budgetID, description, payeeID string, limit int) ([]CategorySuggestion, error) {

	var patterns []PayeePattern
	var err error

	// Strategy 1: Payee-based suggestions (most accurate)
	if payeeID != "" {
		patterns, err = e.patternStore.FindPatternsByPayeeID(ctx, budgetID, payeeID, 50)
		if err != nil {
			return nil, err
		}
	}

	// Strategy 2: Description-based suggestions (fallback or supplement)
	if len(patterns) == 0 && description != "" {
		normalizedDesc := Fingerprint(description)
		patterns, err = e.patternStore.FindPatternsByDescription(ctx, budgetID, normalizedDesc, 20)
		if err != nil {
			return nil, err
		}

		// Filter to only patterns with categories
		var descPatterns []PayeePattern
		for _, p := range patterns {
			if p.CategoryID != "" {
				descPatterns = append(descPatterns, p)
			}
		}
		patterns = descPatterns
	}

	if len(patterns) == 0 {
		return []CategorySuggestion{}, nil
	}

	source := "description"
	if payeeID != "" {
		source = "payee"
	}

	normalizedInput := ""
	if description != "" {
		normalizedInput = Fingerprint(description)
	}

	// Aggregate evidence per category: sum similarity-weighted occurrence
	// counts across every matching pattern instead of keeping only the
	// highest single pattern, so a category confirmed across many
	// moderately-matching patterns beats one old, high-count outlier, and
	// fold in how well the current description matches so ties between
	// categories used with similar frequency/recency break in favor of the
	// category whose recorded descriptions actually resemble this one.
	type categoryAgg struct {
		categoryName       string
		rawOccurrence      int
		weightedOccurrence float64
		maxSimilarity      float64
		mostRecent         time.Time
	}
	aggregates := make(map[string]*categoryAgg)

	for _, pattern := range patterns {
		similarity := 1.0
		if normalizedInput != "" {
			similarity = tokenSimilarity(normalizedInput, pattern.NormalizedDescription)
		}

		agg, exists := aggregates[pattern.CategoryID]
		if !exists {
			agg = &categoryAgg{categoryName: pattern.CategoryName}
			aggregates[pattern.CategoryID] = agg
		}
		agg.rawOccurrence += pattern.OccurrenceCount
		agg.weightedOccurrence += float64(pattern.OccurrenceCount) * similarity
		if similarity > agg.maxSimilarity {
			agg.maxSimilarity = similarity
		}
		if pattern.LastSeen.After(agg.mostRecent) {
			agg.mostRecent = pattern.LastSeen
		}
	}

	// Score each category from its aggregated evidence: similarity worth up
	// to 40, frequency (similarity-weighted occurrence) up to 35, recency up
	// to 25.
	categoryScores := make(map[string]*CategorySuggestion)
	for categoryID, agg := range aggregates {
		similarityScore := agg.maxSimilarity * 40
		frequencyScore := math.Min(agg.weightedOccurrence*5, 35)
		daysSinceLastSeen := time.Since(agg.mostRecent).Hours() / 24
		recencyScore := math.Max(25-daysSinceLastSeen/10, 0)
		confidence := math.Min(similarityScore+frequencyScore+recencyScore, 100)

		categoryScores[categoryID] = &CategorySuggestion{
			CategoryID:   categoryID,
			CategoryName: agg.categoryName,
			Confidence:   confidence,
			Reason:       buildCategoryReason(agg.rawOccurrence, confidence, source),
			Source:       source,
		}
	}

	// Convert map to sorted slice
	var suggestions []CategorySuggestion
	for _, sugg := range categoryScores {
		suggestions = append(suggestions, *sugg)
	}

	// Sort by confidence (descending)
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})

	// Limit results
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	return suggestions, nil
}

// calculateConfidence computes a confidence score (0-100).
func calculateConfidence(inputDesc string, pattern PayeePattern) float64 {
	score := 0.0

	// 1. String similarity (simple token overlap)
	similarity := tokenSimilarity(inputDesc, pattern.NormalizedDescription)
	score += similarity * 50 // Max 50 points for similarity

	// 2. Frequency weight (more occurrences = higher confidence)
	frequencyScore := math.Min(float64(pattern.OccurrenceCount)*5, 30)
	score += frequencyScore // Max 30 points

	// 3. Recency weight (recent patterns weighted higher)
	daysSinceLastSeen := time.Since(pattern.LastSeen).Hours() / 24
	recencyScore := math.Max(20-daysSinceLastSeen/10, 0)
	score += recencyScore // Max 20 points

	return math.Min(score, 100)
}

// tokenSimilarity calculates overlap between two strings (Jaccard similarity).
func tokenSimilarity(s1, s2 string) float64 {
	tokens1 := tokenize(s1)
	tokens2 := tokenize(s2)

	if len(tokens1) == 0 || len(tokens2) == 0 {
		return 0
	}

	set1 := make(map[string]bool)
	for _, t := range tokens1 {
		set1[t] = true
	}

	set2 := make(map[string]bool)
	for _, t := range tokens2 {
		set2[t] = true
	}

	intersection := 0
	for t := range set1 {
		if set2[t] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	return float64(intersection) / float64(union)
}

// tokenize splits string into words.
func tokenize(s string) []string {
	return strings.Fields(strings.ToLower(s))
}

// buildReason creates a human-readable explanation.
func buildReason(pattern PayeePattern, confidence float64) string {
	if confidence > 90 {
		return fmt.Sprintf("Matched %d times before", pattern.OccurrenceCount)
	} else if confidence > 70 {
		return fmt.Sprintf("Similar match (%d occurrences)", pattern.OccurrenceCount)
	}
	return "Possible match"
}

// buildCategoryReason creates a human-readable explanation for category
// suggestions. occurrenceCount is the total (raw, un-weighted) occurrence
// count aggregated across every pattern that contributed to this category's
// score.
func buildCategoryReason(occurrenceCount int, confidence float64, source string) string {
	if source == "payee" {
		if confidence > 90 {
			return fmt.Sprintf("Used %d times with this payee", occurrenceCount)
		} else if confidence > 70 {
			return fmt.Sprintf("Often used with this payee (%d times)", occurrenceCount)
		}
		return "Sometimes used with this payee"
	}
	// Description-based
	if confidence > 90 {
		return fmt.Sprintf("Matched %d times before", occurrenceCount)
	} else if confidence > 70 {
		return fmt.Sprintf("Similar transactions (%d occurrences)", occurrenceCount)
	}
	return "Possible match"
}

// foldDiacritics replaces Polish diacritics with their ASCII equivalents and
// drops non-printable-ASCII characters.
func foldDiacritics(s string) string {
	replacements := map[rune]rune{
		'ą': 'a', 'ć': 'c', 'ę': 'e', 'ł': 'l', 'ń': 'n',
		'ó': 'o', 'ś': 's', 'ź': 'z', 'ż': 'z',
		'Ą': 'A', 'Ć': 'C', 'Ę': 'E', 'Ł': 'L', 'Ń': 'N',
		'Ó': 'O', 'Ś': 'S', 'Ź': 'Z', 'Ż': 'Z',
	}

	var result strings.Builder
	for _, char := range s {
		if replacement, ok := replacements[char]; ok {
			result.WriteRune(replacement)
		} else if char >= 0x20 && char <= 0x7E {
			result.WriteRune(char)
		}
	}
	return result.String()
}

// normalize removes Polish diacritics and non-ASCII characters. Used for
// matching against YNAB payee names, where stripping digits (as Fingerprint
// does) would break matches like "Circle K 24" or "7-Eleven".
func normalize(s string) string {
	return foldDiacritics(s)
}

var (
	fingerprintMaskedCardRe  = regexp.MustCompile(`\*{2,}[\s-]?\d{2,4}`)
	fingerprintCardNumberRe  = regexp.MustCompile(`\b\d{4}[\s-]\d{4}[\s-]\d{4}[\s-]\d{1,4}\b`)
	fingerprintCardRefRe     = regexp.MustCompile(`\bcard\s+\d{1,6}\b`)
	fingerprintISODateRe     = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	fingerprintEUDateRe      = regexp.MustCompile(`\b\d{2}[./]\d{2}[./]\d{4}\b`)
	fingerprintPunctuationRe = regexp.MustCompile(`[^a-zA-Z0-9\s]`)
	fingerprintLongDigitsRe  = regexp.MustCompile(`\d{6,}`)
	fingerprintWhitespaceRe  = regexp.MustCompile(`\s+`)
)

// Fingerprint produces a stable, comparable representation of a bank
// transaction description for pattern matching: lowercased, diacritics
// folded, card numbers/dates/long reference IDs stripped, punctuation
// collapsed to spaces, and whitespace normalized. Unlike normalize(), it
// deliberately discards digits that vary per-transaction (card numbers,
// dates, reference IDs) so that repeated purchases at the same merchant
// fingerprint identically.
func Fingerprint(s string) string {
	result := strings.ToLower(foldDiacritics(s))

	result = fingerprintMaskedCardRe.ReplaceAllString(result, " ")
	result = fingerprintCardNumberRe.ReplaceAllString(result, " ")
	result = fingerprintCardRefRe.ReplaceAllString(result, "card")
	result = fingerprintISODateRe.ReplaceAllString(result, " ")
	result = fingerprintEUDateRe.ReplaceAllString(result, " ")
	result = fingerprintPunctuationRe.ReplaceAllString(result, " ")
	result = fingerprintLongDigitsRe.ReplaceAllString(result, " ")

	result = fingerprintWhitespaceRe.ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}
