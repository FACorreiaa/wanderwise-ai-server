package llmChat

import (
	"fmt"
	"strings"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

func getUserPreferencesPrompt(searchProfile *types.UserPreferenceProfileResponse) string {
	// Base preferences
	basePrefs := fmt.Sprintf(`
BASIC PREFERENCES:
    - Profile Name: %s
    - Search Radius: %.1f km
    - Preferred Time: %s
    - Budget Level: %d (0=any, 1=cheap, 4=expensive)
    - Prefers Outdoor Seating: %t
    - Prefers Dog Friendly: %t
    - Preferred Dietary Needs: [%s]
    - Preferred Pace: %s
    - Prefers Accessible POIs: %t
    - Preferred Vibes: [%s]
    - Preferred Transport: %s`,
		searchProfile.ProfileName, searchProfile.SearchRadiusKm, searchProfile.PreferredTime, searchProfile.BudgetLevel,
		searchProfile.PreferOutdoorSeating, searchProfile.PreferDogFriendly, strings.Join(searchProfile.DietaryNeeds, ", "),
		searchProfile.PreferredPace, searchProfile.PreferAccessiblePOIs, strings.Join(searchProfile.PreferredVibes, ", "),
		searchProfile.PreferredTransport)

	// User location if available
	if searchProfile.UserLatitude != nil && searchProfile.UserLongitude != nil {
		basePrefs += fmt.Sprintf(`
    - User Location: %.4f, %.4f`, *searchProfile.UserLatitude, *searchProfile.UserLongitude)
	}

	// Interests
	if len(searchProfile.Interests) > 0 {
		interests := make([]string, len(searchProfile.Interests))
		for i, interest := range searchProfile.Interests {
			interests[i] = interest.Name
		}
		basePrefs += fmt.Sprintf(`
    - Interests: [%s]`, strings.Join(interests, ", "))
	}

	// Tags to avoid
	if len(searchProfile.Tags) > 0 {
		tags := make([]string, len(searchProfile.Tags))
		for i, tag := range searchProfile.Tags {
			tags[i] = tag.Name
		}
		basePrefs += fmt.Sprintf(`
    - Tags to Avoid: [%s]`, strings.Join(tags, ", "))
	}

	// Accommodation preferences
	if searchProfile.AccommodationPreferences != nil {
		accom := searchProfile.AccommodationPreferences
		basePrefs += `

ACCOMMODATION PREFERENCES:`

		if len(accom.AccommodationType) > 0 {
			basePrefs += fmt.Sprintf(`
    - Accommodation Types: [%s]`, strings.Join(accom.AccommodationType, ", "))
		}

		if accom.StarRating != nil {
			minStar := "any"
			maxStar := "any"
			if accom.StarRating.Min != nil {
				minStar = fmt.Sprintf("%.0f", *accom.StarRating.Min)
			}
			if accom.StarRating.Max != nil {
				maxStar = fmt.Sprintf("%.0f", *accom.StarRating.Max)
			}
			basePrefs += fmt.Sprintf(`
    - Star Rating: %s - %s stars`, minStar, maxStar)
		}

		if accom.PriceRangePerNight != nil {
			minPrice := "any"
			maxPrice := "any"
			if accom.PriceRangePerNight.Min != nil {
				minPrice = fmt.Sprintf("%.0f", *accom.PriceRangePerNight.Min)
			}
			if accom.PriceRangePerNight.Max != nil {
				maxPrice = fmt.Sprintf("%.0f", *accom.PriceRangePerNight.Max)
			}
			basePrefs += fmt.Sprintf(`
    - Price Range Per Night: %s - %s`, minPrice, maxPrice)
		}

		if len(accom.Amenities) > 0 {
			basePrefs += fmt.Sprintf(`
    - Required Amenities: [%s]`, strings.Join(accom.Amenities, ", "))
		}

		if len(accom.RoomType) > 0 {
			basePrefs += fmt.Sprintf(`
    - Room Types: [%s]`, strings.Join(accom.RoomType, ", "))
		}

		if accom.ChainPreference != "" {
			basePrefs += fmt.Sprintf(`
    - Chain Preference: %s`, accom.ChainPreference)
		}
	}

	// Dining preferences
	if searchProfile.DiningPreferences != nil {
		dining := searchProfile.DiningPreferences
		basePrefs += `

DINING PREFERENCES:`

		if len(dining.CuisineTypes) > 0 {
			basePrefs += fmt.Sprintf(`
    - Cuisine Types: [%s]`, strings.Join(dining.CuisineTypes, ", "))
		}

		if len(dining.MealTypes) > 0 {
			basePrefs += fmt.Sprintf(`
    - Meal Types: [%s]`, strings.Join(dining.MealTypes, ", "))
		}

		if len(dining.ServiceStyle) > 0 {
			basePrefs += fmt.Sprintf(`
    - Service Style: [%s]`, strings.Join(dining.ServiceStyle, ", "))
		}

		if dining.PriceRangePerPerson != nil {
			minPrice := "any"
			maxPrice := "any"
			if dining.PriceRangePerPerson.Min != nil {
				minPrice = fmt.Sprintf("%.0f", *dining.PriceRangePerPerson.Min)
			}
			if dining.PriceRangePerPerson.Max != nil {
				maxPrice = fmt.Sprintf("%.0f", *dining.PriceRangePerPerson.Max)
			}
			basePrefs += fmt.Sprintf(`
    - Price Range Per Person: %s - %s`, minPrice, maxPrice)
		}

		if len(dining.AllergenFree) > 0 {
			basePrefs += fmt.Sprintf(`
    - Allergen Free: [%s]`, strings.Join(dining.AllergenFree, ", "))
		}

		if dining.MichelinRated {
			basePrefs += `
    - Michelin Rated: Preferred`
		}

		if dining.LocalRecommendations {
			basePrefs += `
    - Local Recommendations: Preferred`
		}

		if dining.ChainVsLocal != "" {
			basePrefs += fmt.Sprintf(`
    - Chain vs Local: %s`, dining.ChainVsLocal)
		}

		if dining.OrganicPreference {
			basePrefs += `
    - Organic Preference: Yes`
		}

		if dining.OutdoorSeatingPref {
			basePrefs += `
    - Outdoor Seating: Preferred`
		}
	}

	// Activity preferences
	if searchProfile.ActivityPreferences != nil {
		activity := searchProfile.ActivityPreferences
		basePrefs += `

ACTIVITY PREFERENCES:`

		if len(activity.ActivityCategories) > 0 {
			basePrefs += fmt.Sprintf(`
    - Activity Categories: [%s]`, strings.Join(activity.ActivityCategories, ", "))
		}

		if activity.PhysicalActivityLevel != "" {
			basePrefs += fmt.Sprintf(`
    - Physical Activity Level: %s`, activity.PhysicalActivityLevel)
		}

		if activity.IndoorOutdoorPref != "" {
			basePrefs += fmt.Sprintf(`
    - Indoor/Outdoor Preference: %s`, activity.IndoorOutdoorPref)
		}

		if activity.CulturalImmersionLevel != "" {
			basePrefs += fmt.Sprintf(`
    - Cultural Immersion Level: %s`, activity.CulturalImmersionLevel)
		}

		if activity.MustSeeVsHiddenGems != "" {
			basePrefs += fmt.Sprintf(`
    - Must-See vs Hidden Gems: %s`, activity.MustSeeVsHiddenGems)
		}

		if activity.EducationalPreference {
			basePrefs += `
    - Educational Preference: Yes`
		}

		if activity.PhotoOpportunities {
			basePrefs += `
    - Photography Opportunities: Important`
		}

		if len(activity.SeasonSpecific) > 0 {
			basePrefs += fmt.Sprintf(`
    - Season Specific: [%s]`, strings.Join(activity.SeasonSpecific, ", "))
		}

		if activity.AvoidCrowds {
			basePrefs += `
    - Avoid Crowds: Yes`
		}

		if len(activity.LocalEventsInterest) > 0 {
			basePrefs += fmt.Sprintf(`
    - Local Events Interest: [%s]`, strings.Join(activity.LocalEventsInterest, ", "))
		}
	}

	// Itinerary preferences
	if searchProfile.ItineraryPreferences != nil {
		itinerary := searchProfile.ItineraryPreferences
		basePrefs += `

ITINERARY PREFERENCES:`

		if itinerary.PlanningStyle != "" {
			basePrefs += fmt.Sprintf(`
    - Planning Style: %s`, itinerary.PlanningStyle)
		}

		if itinerary.TimeFlexibility != "" {
			basePrefs += fmt.Sprintf(`
    - Time Flexibility: %s`, itinerary.TimeFlexibility)
		}

		if itinerary.MorningVsEvening != "" {
			basePrefs += fmt.Sprintf(`
    - Morning vs Evening: %s`, itinerary.MorningVsEvening)
		}

		if itinerary.WeekendVsWeekday != "" {
			basePrefs += fmt.Sprintf(`
    - Weekend vs Weekday: %s`, itinerary.WeekendVsWeekday)
		}

		if len(itinerary.PreferredSeasons) > 0 {
			basePrefs += fmt.Sprintf(`
    - Preferred Seasons: [%s]`, strings.Join(itinerary.PreferredSeasons, ", "))
		}

		if itinerary.AvoidPeakSeason {
			basePrefs += `
    - Avoid Peak Season: Yes`
		}

		if itinerary.AdventureVsRelaxation != "" {
			basePrefs += fmt.Sprintf(`
    - Adventure vs Relaxation: %s`, itinerary.AdventureVsRelaxation)
		}

		if itinerary.SpontaneousVsPlanned != "" {
			basePrefs += fmt.Sprintf(`
    - Spontaneous vs Planned: %s`, itinerary.SpontaneousVsPlanned)
		}
	}

	return basePrefs
}

func getPOIDetailsPrompt(city string, lat, lon float64) string {
	return fmt.Sprintf(`
		Generate details for the following POI on the city of %s with the coordinates %0.2f , %0.2f.
		The result should be in the following JSON format:
		{
			"name": "Name of the Point of Interest",
			"description": "Detailed description of the POI and why it's relevant to the user's interest.",
    		"address": "address of the point of interest",
    		"website": "website of the POI if available",
    		"phone_number": "phone number of the POI if available",
    		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
    		"price_range": "price level if available",
            "category": "Primary category (e.g., Museum, Historical Site, Park, Restaurant, Bar)",
            "tags": ["tag1", "tag2", ...], -- Tags related to the POI
            "images": ["image_url_1", "image_url_2", ...], // images from wikipedia or pininterest
            "rating": <float> -- Average rating if available
            "stars": type of stars if available (e.g., "3 stars", "5 stars")

		}
	`, city, lat, lon)
}

func getHotelsByPreferencesPrompt(city string, lat, lon float64, userPreferences types.HotelUserPreferences) string {
	return fmt.Sprintf(`
        Generate a list of maximum 5 hotels in the city of %s, near the coordinates %0.2f , %0.2f.
        The hotels should be relevant to the user's interest.
        The result should be tailored to the user's preferences:
        - Preferred Category: %s
        - Preferred Tags: %s
        - Max Price range: %s
        - Preferred Rating: %0.2f
        - Number of Guests: %d
        - Number of Nights: %d
        - Number of Rooms: %d
        - Preferred Check-In Date: %s
        - Preferred Check-Out Date: %s
        - Distance: %0.2f km (if provided, otherwise use default radius of 5km)
        The result should be in the following JSON format:
        {
            "hotels": [
                {
                    "name": "Name of the Hotel",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Primary category (e.g., Hotel, Hostel, Guesthouse)",
                    "description": "A brief description of this hotel and why it's relevant to the user's interest."
                }
            ]
        }
    `, city, lat, lon, userPreferences.PreferredCategories, userPreferences.PreferredTags,
		userPreferences.MaxPriceRange, userPreferences.MinRating,
		userPreferences.NumberOfGuests, userPreferences.NumberOfNights, userPreferences.NumberOfRooms,
		userPreferences.PreferredCheckIn.Format("2006-01-02"), userPreferences.PreferredCheckOut.Format("2006-01-02"),
		userPreferences.SearchRadiusKm)
}

func getHotelsNeabyPrompt(city string, userLocation types.UserLocation) string {
	return fmt.Sprintf(`
        Generate a list of maximum 5 hotels nearby the coordinates %0.2f , %0.2f in the city of %s.
        the hotels can be around %0.2f km radius from the user's location or if nothing provided, use the default radius of 5km.
        The hotels should be relevant to the user's interest.
        The result should be in the following JSON format:
        {
            "hotels": [
                {
                    "name": "Name of the Hotel",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Primary category (e.g., Hotel, Hostel, Guesthouse)",
                    "description": "A brief description of this hotel and why it's relevant to the user's interest."
                }
            ]
        }
    `, userLocation.UserLat, userLocation.UserLon, city, userLocation.SearchRadiusKm)
}

func getRestaurantsByPreferencesPrompt(city string, lat, lon float64, userPreferences types.RestaurantUserPreferences) string {
	return fmt.Sprintf(`
        Generate a list of up to 10 restaurants in the city of %s, near coordinates %.2f, %.2f.
        Tailor the results to the user's preferences:
        - Preferred Cuisine: %s
        - Preferred Price Range: %s
        - Dietary Restrictions: %s
        - Ambiance: %s
        - Special Features: %s
        The result must be in JSON format:
        {
            "restaurants": [
                {
                    "name": "Restaurant Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Restaurant|Bar|Cafe",
                    "description": "Brief description of the restaurant and why it matches preferences."
                }
            ]
        }
    `, city, lat, lon, userPreferences.PreferredCuisine, userPreferences.PreferredPriceRange,
		userPreferences.DietaryRestrictions, userPreferences.Ambiance, userPreferences.SpecialFeatures)
}

func getRestaurantsNearbyPrompt(city string, userLocation types.UserLocation) string {
	if userLocation.SearchRadiusKm == 0 {
		userLocation.SearchRadiusKm = 5.0 // Default radius
	}
	return fmt.Sprintf(`
        Generate a list of up to 10 restaurants in the city of %s, within %.2f km of coordinates %.2f, %.2f.
        Include a variety of restaurant categories to provide diverse options.
        The result must be in JSON format:
        {
            "restaurants": [
                {
                    "name": "Restaurant Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Restaurant|Bar|Cafe",
                    "description": "Brief description of the restaurant and its proximity to the user's location."
                }
            ]
        }
    `, city, userLocation.SearchRadiusKm, userLocation.UserLat, userLocation.UserLon)
}

func generatedContinuedConversationPrompt(poi, city string) string {
	return fmt.Sprintf(
		`Provide detailed information about "%s" in %s.
        If user writes "Restaurant" add "cuisine_type" to final response and hide "description_poi"
        If user writes "Hotel" add "star_rating" to final response and hide "description_poi"
		Analise this POI (The user can insert a POI name, a Restaurant name or an Hotel/Hostel name) and return the following JSON structure:
    {
        "name": "string (the POI name)",
        "latitude": number (approximate latitude as float),
        "longitude": number (approximate longitude as float),
        "category": "string (e.g., Museum, Park, Historical Site)",
        "description_poi": "string (50-100 words description)"
        "cuisine_type": "string (for Restaurant)",
        "star_rating": "number (for Hotel/Hostel)"
    }

    If the POI is not found, return: {"name": "", "latitude": 0, "longitude": 0, "category": "", "description_poi": ""}`,
		poi, city)
}

// getCityDescriptionPrompt generates a prompt for city data
func getCityDescriptionPrompt(cityName string) string {
	return fmt.Sprintf(`
        Provide detailed information about the city %s in JSON format with the following structure:
        {
            "city_name": "%s",
            "country": "Country name",
            "state_province": "State or province, if applicable",
            "description": "A detailed description of the city",
            "center_latitude": float64,
            "center_longitude": float64
        }
    `, cityName, cityName)
}

// GetUnifiedChatPrompt generates context-based prompts for the unified chat system
func GetUnifiedChatPrompt(context, cityName string, lat, lon float64, searchProfile *types.UserPreferenceProfileResponse) string {
	basePreferences := ""
	if searchProfile != nil {
		basePreferences = getUserPreferencesPrompt(searchProfile)
	}

	switch context {
	case "traveling", "itinerary":
		return fmt.Sprintf(`
You are a travel planning assistant. Create a personalized itinerary for %s based on the user's location (%.4f, %.4f) and preferences.

USER PREFERENCES:
%s

Generate a comprehensive travel response in JSON format with the following structure:
{
    "data": {
        "general_city_data": {
            "city": "%s",
            "country": "Country name",
            "state_province": "State/Province if applicable",
            "description": "Detailed city description (100-150 words)",
            "center_latitude": %.4f,
            "center_longitude": %.4f,
            "population": "",
            "area": "",
            "timezone": "",
            "language": "",
            "weather": "",
            "attractions": "",
            "history": ""
        },
        "points_of_interest": [
            {
                "name": "POI Name",
                "latitude": <float>,
                "longitude": <float>,
                "category": "Category (e.g., Museum, Historical Site)",
                "description_poi": "",
                "address": "",
                "website": "",
                "opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"

            }
        ],
        "itinerary_response": {
            "itinerary_name": "Creative itinerary name based on user preferences",
            "overall_description": "Detailed description of the itinerary (100-150 words)",
            "points_of_interest": [
                {
                    "name": "POI Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "",
                    "description_poi": "",
                    "address": "",
                    "website": "",
                    "opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"

                }
            ]
        }
    }
}

Focus on creating an itinerary that matches the user's preferences, dietary needs, preferred pace, and transportation method.`,
			cityName, lat, lon, basePreferences, cityName, lat, lon)

	case "accommodation":
		return fmt.Sprintf(`
You are a hotel recommendation assistant. Find suitable accommodation in %s near coordinates %.4f, %.4f.

USER PREFERENCES:
%s

Generate a hotel response in JSON format:
{
    "hotels": [
        {
            "city": "%s",
            "name": "Hotel Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Hotel|Hostel|Guesthouse|Apartment",
            "description": "Description matching user preferences and budget level",
            "address": "",
            "phone_number": null,
            "website": null,
            "opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
            "price_range": null,
            "rating": 0,
            "tags": null,
            "images": null
        }
    ]
}

Consider the user's budget level, preferred amenities, and accessibility needs when selecting accommodation.`,
			cityName, lat, lon, basePreferences, cityName)

	case "dining":
		return fmt.Sprintf(`
You are a restaurant recommendation assistant. Find dining options in %s near coordinates %.4f, %.4f.

USER PREFERENCES:
%s

Generate a restaurant response in JSON format:
{
    "restaurants": [
        {
            "city": "%s",
            "name": "Restaurant Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Fine Dining|Casual Dining|Fast Food|Cafe|Bar",
            "description": "Description matching user dietary needs and preferences",
            "address": "Complete address",
            "website": "Official website URL (if available)",
            "phone_number": "Phone number (if available)",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",

            "price_level": "$|$$|$$$|$$$$",
            "cuisine_type": "Cuisine type",
            "tags": ["tag1", "tag2"],
            "images": [],
            "rating": 0
        }
    ]
}

Pay special attention to dietary needs, budget level, cuisine preferences, and accessibility options.`,
			cityName, lat, lon, basePreferences, cityName)

	case "activities":
		return fmt.Sprintf(`
You are an activity recommendation assistant. Find activities and attractions in %s near coordinates %.4f, %.4f.

USER PREFERENCES:
%s

Generate an activities response in JSON format:
{
    "activities": [
        {
            "city": "%s",
            "name": "Activity/Attraction Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Museum|Outdoor Activity|Entertainment|Cultural|Sports",
            "description": "Description matching user activity preferences",
            "address": "Complete address",
            "website": "Official website URL (if available)",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",

            "price_range": "Free|$|$$|$$$",
            "rating": 0,
            "tags": ["tag1", "tag2"],
            "images": []
        }
    ]
}

Consider the user's physical activity level, cultural interests, and accessibility needs when selecting activities.`,
			cityName, lat, lon, basePreferences, cityName)

	default:
		// Default to itinerary if context is not recognized
		return GetUnifiedChatPrompt("traveling", cityName, lat, lon, searchProfile)
	}
}

/*
  Testing Fan in Fan out prompt
*/

func getCityDataPrompt(cityName string) string {
	return fmt.Sprintf(`
You are a travel assistant. Provide detailed information about %s.

CRITICAL: You MUST respond with valid JSON only. No text, explanations, or status messages outside the JSON structure.

Return accurate information about %s:

{
    "city": "%s",
    "country": "Country name",
    "state_province": "State/Province if applicable",
    "description": "Detailed city description (100-150 words)",
    "center_latitude": 40.4168,
    "center_longitude": -3.7038,
    "population": "Population number or estimate",
    "area": "City area in km²",
    "timezone": "Time zone (e.g., UTC+1)",
    "language": "Primary language spoken",
    "weather": "Brief climate description",
    "attractions": "Brief mention of main attractions",
    "history": "Brief historical overview"
}`, cityName, cityName, cityName)
}

func getGeneralPOIPrompt(cityName string) string {
	return fmt.Sprintf(`
You are a travel assistant. Generate a list of top points of interest in %s.

CRITICAL: You MUST respond with valid JSON only. No text, explanations, or status messages outside the JSON structure.

Return exactly 5-8 real points of interest in %s with accurate coordinates:

{
    "points_of_interest": [
        {
            "name": "Real POI Name",
            "latitude": 40.4168,
            "longitude": -3.7038,
            "category": "museum/monument/park/attraction/etc",
            "description_poi": "Detailed description of this place",
            "address": "Complete street address",
            "website": "https://example.com",
            "opening_hours": {"monday": "9:00-18:00", "tuesday": "9:00-18:00"}
        }
    ]
}`, cityName, cityName)
}

func getPersonalizedItineraryPrompt(cityName, basePreferences string) string {
	return fmt.Sprintf(`
You are a travel assistant. Generate a detailed personalized itinerary for %s with specific places and activities.

USER PREFERENCES:
%s

IMPORTANT: You MUST respond with valid JSON in the exact format shown below. Do not provide explanations, status updates, or any text outside the JSON structure.

Return exactly 3-5 real points of interest in %s with accurate coordinates:

{
    "itinerary_name": "Personalized %s Experience",
    "overall_description": "A detailed 100-150 word description of this personalized itinerary",
    "points_of_interest": [
        {
            "name": "Real POI Name",
            "latitude": 40.4168,
            "longitude": -3.7038,
            "category": "museum/restaurant/attraction/etc",
            "description_poi": "Detailed description of this specific place",
            "address": "Complete street address",
            "website": "https://example.com",
            "opening_hours": {"monday": "9:00-18:00", "tuesday": "9:00-18:00"},
            "distance": 0.5
        }
    ]
}`, cityName, basePreferences, cityName, cityName)
}

func getGeneralizedItineraryPrompt(cityName string) string {
	return fmt.Sprintf(`
You are a travel assistant. Generate a complete itinerary for %s with exactly 5 diverse activities and attractions.

CRITICAL: You MUST respond with valid JSON only. No text, explanations, or status messages outside the JSON structure.

Return exactly 5 real points of interest in %s with accurate coordinates:

{
    "itinerary_name": "Complete %s Experience",
    "overall_description": "A detailed 100-150 word description of this general itinerary covering diverse activities",
    "points_of_interest": [
        {
            "name": "Real POI Name",
            "latitude": 40.4168,
            "longitude": -3.7038,
            "category": "museum/restaurant/attraction/park/etc",
            "description_poi": "Detailed description of this specific place",
            "address": "Complete street address",
            "website": "https://example.com",
            "opening_hours": {"monday": "9:00-18:00", "tuesday": "9:00-18:00"},
            "distance": 0.5
        }
    ]
}`, cityName, cityName, cityName)
}

func getAccommodationPrompt(cityName string, lat, lon float64, basePreferences string) string {
	return fmt.Sprintf(`
You are a hotel recommendation assistant. Find suitable accommodation in %s near coordinates %.4f, %.4f.
USER PREFERENCES:
%s
Respond with JSON:
{
    "hotels": [
        {
            "city": "%s",
            "name": "Hotel Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Hotel|Hostel|Guesthouse|Apartment",
            "description": "Description matching preferences",
            "address": "",
            "phone_number": null,
            "website": null,
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",
            "price_range": null,
            "rating": 0,
            "tags": null,
            "images": null,
            "distance": <float>
        }
    ]
}`, cityName, lat, lon, basePreferences, cityName)
}

func getGeneralAccommodationPrompt(cityName string) string {
	return fmt.Sprintf(`
You are a hotel recommendation assistant. Find a max of 5 suitable accommodation in %s.
Respond with JSON:
{
    "hotels": [
        {
            "city": "%s",
            "name": "Hotel Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Hotel|Hostel|Guesthouse|Apartment",
            "description": "Description matching preferences",
            "address": "",
            "phone_number": null,
            "website": null,
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",
            "price_range": null,
            "rating": 0,
            "tags": null,
            "images": null,
            "distance": <float>
        }
    ]
}`, cityName, cityName)
}

func getDiningPrompt(cityName string, lat, lon float64, basePreferences string) string {
	return fmt.Sprintf(`
You are a restaurant recommendation assistant. Find dining options in %s near coordinates %.4f, %.4f.
USER PREFERENCES:
%s
Respond with JSON:
{
    "restaurants": [
        {
            "city": "%s",
            "name": "Restaurant Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Fine Dining|Casual Dining|Fast Food|Cafe|Bar",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
            "phone_number": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_level": "$|$$|$$$|$$$$",
            "cuisine_type": "",
            "tags": [],
            "images": [],
            "rating": 0,
            "distance": <float>
        }
    ]
}`, cityName, lat, lon, basePreferences, cityName)
}

func getGeneralDiningPrompt(cityName string) string {
	return fmt.Sprintf(`
You are a restaurant recommendation assistant. Find a max of 5 dining options in %s.
Respond with JSON:
{
    "restaurants": [
        {
            "city": "%s",
            "name": "Restaurant Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Fine Dining|Casual Dining|Fast Food|Cafe|Bar",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
            "phone_number": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_level": "$|$|$$|$$",
            "cuisine_type": "",
            "tags": [],
            "images": [],
            "rating": 0,
            "distance": <float>
        }
    ]
}`, cityName, cityName)
}

func getActivitiesPrompt(cityName string, lat, lon float64, basePreferences string) string {
	return fmt.Sprintf(`
You are an activity recommendation assistant. Find activities in %s near coordinates %.4f, %.4f.
USER PREFERENCES:
%s
Respond with JSON:
{
    "activities": [
        {
            "city": "%s",
            "name": "Activity Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Museum|Outdoor Activity|Entertainment|Cultural|Sports",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_range": "Free|$|$$|$$$",
            "rating": 0,
            "tags": [],
            "images": [],
            "distance": <float>
        }
    ]
}`, cityName, lat, lon, basePreferences, cityName)
}

func getGeneralActivitiesPrompt(cityName string) string {
	return fmt.Sprintf(`
You are an activity recommendation assistant. Find a max of 5 activities in %s.
Respond with JSON:
{
    "activities": [
        {
            "city": "%s",
            "name": "Activity Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Museum|Outdoor Activity|Entertainment|Cultural|Sports",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_range": "Free|$|$$|$$$",
            "rating": 0,
            "tags": [],
            "images": [],
            "distance": <float>
        }
    ]
}`, cityName, cityName)
}
