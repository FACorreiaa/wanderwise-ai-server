package types

import (
	"context"
	"strings"
	"testing"

	a "github.com/petar-dambovaliev/aho-corasick"
)

// Current regex implementation (from chat.go)
func detectDomainRegex(message string) DomainType {
	detector := &DomainDetector{}
	return detector.DetectDomain(context.Background(), message)
}

// Your 4-matcher approach
var (
	accommBuilder = a.NewAhoCorasickBuilder(a.Opts{
		AsciiCaseInsensitive: true,
		MatchOnlyWholeWords:  true,
	})
	accommMatcher = accommBuilder.Build([]string{"hotel", "hostel", "accommodation", "stay", "sleep", "room", "booking", "airbnb", "lodge", "resort", "guesthouse"})

	diningBuilder = a.NewAhoCorasickBuilder(a.Opts{
		AsciiCaseInsensitive: true,
		MatchOnlyWholeWords:  true,
	})
	diningMatcher = diningBuilder.Build([]string{"restaurant", "food", "eat", "dine", "meal", "cuisine", "drink", "cafe", "bar", "lunch", "dinner", "breakfast", "brunch"})

	activBuilder = a.NewAhoCorasickBuilder(a.Opts{
		AsciiCaseInsensitive: true,
		MatchOnlyWholeWords:  true,
	})
	activMatcher = activBuilder.Build([]string{"activity", "museum", "park", "attraction", "tour", "visit", "see", "do", "experience", "adventure", "shopping", "nightlife"})

	itinBuilder = a.NewAhoCorasickBuilder(a.Opts{
		AsciiCaseInsensitive: true,
		MatchOnlyWholeWords:  true,
	})
	itinMatcher = itinBuilder.Build([]string{"itinerary", "plan", "schedule", "trip", "day", "week", "journey", "route", "organize", "arrange"})
)

func hasMatch(m a.AhoCorasick, s string) bool {
	iter := m.Iter(s)
	return iter.Next() != nil
}

func detectDomainFourMatchers(message string) DomainType {
	message = strings.ToLower(message)

	switch true {
	case hasMatch(accommMatcher, message):
		return DomainAccommodation
	case hasMatch(diningMatcher, message):
		return DomainDining
	case hasMatch(activMatcher, message):
		return DomainActivities
	case hasMatch(itinMatcher, message):
		return DomainItinerary
	default:
		return DomainGeneral
	}
}

// Optimized single-matcher approach
var (
	singleMatcherBuilder = a.NewAhoCorasickBuilder(a.Opts{
		AsciiCaseInsensitive: true,
		MatchOnlyWholeWords:  true,
	})
	singleMatcher = singleMatcherBuilder.Build([]string{
		"hotel", "hostel", "accommodation", "stay", "sleep", "room",
		"booking", "airbnb", "lodge", "resort", "guesthouse",
		"restaurant", "food", "eat", "dine", "meal", "cuisine",
		"drink", "cafe", "bar", "lunch", "dinner", "breakfast", "brunch",
		"activity", "museum", "park", "attraction", "tour", "visit",
		"see", "do", "experience", "adventure", "shopping", "nightlife",
		"itinerary", "plan", "schedule", "trip", "day", "week",
		"journey", "route", "organize", "arrange",
	})

	keywordMap = map[string]DomainType{
		"hotel": DomainAccommodation, "hostel": DomainAccommodation,
		"accommodation": DomainAccommodation, "stay": DomainAccommodation,
		"sleep": DomainAccommodation, "room": DomainAccommodation,
		"booking": DomainAccommodation, "airbnb": DomainAccommodation,
		"lodge": DomainAccommodation, "resort": DomainAccommodation,
		"guesthouse": DomainAccommodation,
		"restaurant": DomainDining, "food": DomainDining,
		"eat": DomainDining, "dine": DomainDining,
		"meal": DomainDining, "cuisine": DomainDining,
		"drink": DomainDining, "cafe": DomainDining,
		"bar": DomainDining, "lunch": DomainDining,
		"dinner": DomainDining, "breakfast": DomainDining,
		"brunch": DomainDining,
		"activity": DomainActivities, "museum": DomainActivities,
		"park": DomainActivities, "attraction": DomainActivities,
		"tour": DomainActivities, "visit": DomainActivities,
		"see": DomainActivities, "do": DomainActivities,
		"experience": DomainActivities, "adventure": DomainActivities,
		"shopping": DomainActivities, "nightlife": DomainActivities,
		"itinerary": DomainItinerary, "plan": DomainItinerary,
		"schedule": DomainItinerary, "trip": DomainItinerary,
		"day": DomainItinerary, "week": DomainItinerary,
		"journey": DomainItinerary, "route": DomainItinerary,
		"organize": DomainItinerary, "arrange": DomainItinerary,
	}
)

func detectDomainSingleMatcher(message string) DomainType {
	message = strings.ToLower(message)
	matches := singleMatcher.FindAll(message)

	if len(matches) == 0 {
		return DomainGeneral
	}

	// Return first match's domain
	matchedWord := message[matches[0].Start():matches[0].End()]
	return keywordMap[matchedWord]
}

// Benchmark test cases
var testMessages = []string{
	"I need a hotel near the Eiffel Tower",                           // Accommodation (early match)
	"What are some good restaurants in Tokyo?",                        // Dining (early match)
	"I want to visit museums and parks tomorrow",                      // Activities (early match)
	"Help me plan my itinerary for next week",                         // Itinerary (early match)
	"This is a long message about my travel plans and I'm not sure", // General (no match, worst case)
}

func BenchmarkRegex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, msg := range testMessages {
			detectDomainRegex(msg)
		}
	}
}

func BenchmarkFourMatchers(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, msg := range testMessages {
			detectDomainFourMatchers(msg)
		}
	}
}

func BenchmarkSingleMatcher(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, msg := range testMessages {
			detectDomainSingleMatcher(msg)
		}
	}
}

// Test correctness
func TestDomainDetectionCorrectness(t *testing.T) {
	tests := []struct {
		message  string
		expected DomainType
	}{
		{"I need a hotel", DomainAccommodation},
		{"Where can I eat?", DomainDining},
		{"What museums should I visit?", DomainActivities},
		{"Help me plan my trip", DomainItinerary},
		{"Random message", DomainGeneral},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			regexResult := detectDomainRegex(tt.message)
			fourMatcherResult := detectDomainFourMatchers(tt.message)
			singleMatcherResult := detectDomainSingleMatcher(tt.message)

			if regexResult != tt.expected {
				t.Errorf("Regex: got %v, want %v", regexResult, tt.expected)
			}
			if fourMatcherResult != tt.expected {
				t.Errorf("FourMatchers: got %v, want %v", fourMatcherResult, tt.expected)
			}
			if singleMatcherResult != tt.expected {
				t.Errorf("SingleMatcher: got %v, want %v", singleMatcherResult, tt.expected)
			}
		})
	}
}
