package types

type MainPageStatistics struct {
	TotalUsersCount       int64 `json:"total_users_count"`
	TotalItinerariesSaved int64 `json:"total_itineraries_saved"`
	TotalUniquePOIs       int64 `json:"total_unique_pois"`
}

type DetailedPOIStatistics struct {
	GeneralPOIs   int64 `json:"general_pois"`
	SuggestedPOIs int64 `json:"suggested_pois"`
	Hotels        int64 `json:"hotels"`
	Restaurants   int64 `json:"restaurants"`
	TotalPOIs     int64 `json:"total_pois"`
}

type LandingPageUserStats struct {
	SavedPlaces    int `json:"saved_places"`
	Itineraries    int `json:"itineraries"`
	CitiesExplored int `json:"cities_explored"`
	Discoveries    int `json:"discoveries"`
}
