package profiles

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *RepositoryImpl) updateAccommodationPreferencesInTx(ctx context.Context, tx pgx.Tx, profileID uuid.UUID, prefs *AccommodationPreferences) error {
	// Convert preferences to JSONB
	filters := map[string]interface{}{
		"accommodation_type":    prefs.AccommodationType,
		"star_rating":           prefs.StarRating,
		"price_range_per_night": prefs.PriceRangePerNight,
		"amenities":             prefs.Amenities,
		"room_type":             prefs.RoomType,
		"chain_preference":      prefs.ChainPreference,
		"cancellation_policy":   prefs.CancellationPolicy,
		"booking_flexibility":   prefs.BookingFlexibility,
	}

	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE user_accommodation_preferences 
		 SET accommodation_filters = $2, updated_at = CURRENT_TIMESTAMP 
		 WHERE user_preference_profile_id = $1`,
		profileID, filtersJSON)
	return err
}

func (r *RepositoryImpl) updateDiningPreferencesInTx(ctx context.Context, tx pgx.Tx, profileID uuid.UUID, prefs *DiningPreferences) error {
	// Convert preferences to JSONB
	filters := map[string]interface{}{
		"cuisine_types":             prefs.CuisineTypes,
		"meal_types":                prefs.MealTypes,
		"service_style":             prefs.ServiceStyle,
		"price_range_per_person":    prefs.PriceRangePerPerson,
		"dietary_needs":             prefs.DietaryNeeds,
		"allergen_free":             prefs.AllergenFree,
		"michelin_rated":            prefs.MichelinRated,
		"local_recommendations":     prefs.LocalRecommendations,
		"chain_vs_local":            prefs.ChainVsLocal,
		"organic_preference":        prefs.OrganicPreference,
		"outdoor_seating_preferred": prefs.OutdoorSeatingPref,
	}

	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE user_dining_preferences 
		 SET dining_filters = $2, updated_at = CURRENT_TIMESTAMP 
		 WHERE user_preference_profile_id = $1`,
		profileID, filtersJSON)
	return err
}

func (r *RepositoryImpl) updateActivityPreferencesInTx(ctx context.Context, tx pgx.Tx, profileID uuid.UUID, prefs *ActivityPreferences) error {
	// Convert preferences to JSONB
	filters := map[string]interface{}{
		"activity_categories":        prefs.ActivityCategories,
		"physical_activity_level":    prefs.PhysicalActivityLevel,
		"indoor_outdoor_preference":  prefs.IndoorOutdoorPref,
		"cultural_immersion_level":   prefs.CulturalImmersionLevel,
		"must_see_vs_hidden_gems":    prefs.MustSeeVsHiddenGems,
		"educational_preference":     prefs.EducationalPreference,
		"photography_opportunities":  prefs.PhotoOpportunities,
		"season_specific_activities": prefs.SeasonSpecific,
		"avoid_crowds":               prefs.AvoidCrowds,
		"local_events_interest":      prefs.LocalEventsInterest,
	}

	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE user_activity_preferences 
		 SET activity_filters = $2, updated_at = CURRENT_TIMESTAMP 
		 WHERE user_preference_profile_id = $1`,
		profileID, filtersJSON)
	return err
}

func (r *RepositoryImpl) updateItineraryPreferencesInTx(ctx context.Context, tx pgx.Tx, profileID uuid.UUID, prefs *ItineraryPreferences) error {
	// Convert preferences to JSONB
	filters := map[string]interface{}{
		"planning_style":          prefs.PlanningStyle,
		"preferred_pace":          prefs.PreferredPace,
		"time_flexibility":        prefs.TimeFlexibility,
		"morning_vs_evening":      prefs.MorningVsEvening,
		"weekend_vs_weekday":      prefs.WeekendVsWeekday,
		"preferred_seasons":       prefs.PreferredSeasons,
		"avoid_peak_season":       prefs.AvoidPeakSeason,
		"adventure_vs_relaxation": prefs.AdventureVsRelaxation,
		"spontaneous_vs_planned":  prefs.SpontaneousVsPlanned,
	}

	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE user_itinerary_preferences 
		 SET itinerary_filters = $2, updated_at = CURRENT_TIMESTAMP 
		 WHERE user_preference_profile_id = $1`,
		profileID, filtersJSON)
	return err
}
