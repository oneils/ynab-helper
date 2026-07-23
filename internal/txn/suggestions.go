package txn

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

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

	normalizedDesc := normalize(description)

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
		normalizedDesc := normalize(description)
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

	// Score and deduplicate by category
	categoryScores := make(map[string]*CategorySuggestion)
	source := "description"
	if payeeID != "" {
		source = "payee"
	}

	for _, pattern := range patterns {
		// Calculate confidence score
		var confidence float64
		if payeeID != "" {
			// Payee-based: frequency and recency matter most
			frequencyScore := math.Min(float64(pattern.OccurrenceCount)*10, 70)
			daysSinceLastSeen := time.Since(pattern.LastSeen).Hours() / 24
			recencyScore := math.Max(30-daysSinceLastSeen/10, 0)
			confidence = math.Min(frequencyScore+recencyScore, 100)
		} else {
			// Description-based: use full scoring
			confidence = calculateConfidence(normalize(description), pattern)
		}

		// Keep highest confidence per category
		existing, exists := categoryScores[pattern.CategoryID]
		if !exists || confidence > existing.Confidence {
			categoryScores[pattern.CategoryID] = &CategorySuggestion{
				CategoryID:   pattern.CategoryID,
				CategoryName: pattern.CategoryName,
				Confidence:   confidence,
				Reason:       buildCategoryReason(pattern, confidence, source),
				Source:       source,
			}
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

	intersection := 0
	tokenSet := make(map[string]bool)

	for _, t := range tokens1 {
		tokenSet[t] = true
	}

	for _, t := range tokens2 {
		if tokenSet[t] {
			intersection++
		}
	}

	union := len(tokens1) + len(tokens2) - intersection
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

// buildCategoryReason creates a human-readable explanation for category suggestions.
func buildCategoryReason(pattern PayeePattern, confidence float64, source string) string {
	if source == "payee" {
		if confidence > 90 {
			return fmt.Sprintf("Used %d times with this payee", pattern.OccurrenceCount)
		} else if confidence > 70 {
			return fmt.Sprintf("Often used with this payee (%d times)", pattern.OccurrenceCount)
		}
		return "Sometimes used with this payee"
	}
	// Description-based
	if confidence > 90 {
		return fmt.Sprintf("Matched %d times before", pattern.OccurrenceCount)
	} else if confidence > 70 {
		return fmt.Sprintf("Similar transactions (%d occurrences)", pattern.OccurrenceCount)
	}
	return "Possible match"
}

// normalize removes Polish diacritics and non-ASCII characters.
func normalize(s string) string {
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
