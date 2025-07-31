package poi

import (
	"fmt"
)

func getRestaurantsNearbyPrompt(userLocation UserLocation) string {
	if userLocation.SearchRadiusKm == 0 {
		userLocation.SearchRadiusKm = 5.0
	}
	return fmt.Sprintf(`
        Generate a list of up to 10 restaurants within %.2f km of coordinates %.2f, %.2f.
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
    `, userLocation.SearchRadiusKm, userLocation.UserLat, userLocation.UserLon)
}

func getHotelsNeabyPrompt(userLocation UserLocation) string {
	return fmt.Sprintf(`
        Generate a list of maximum 10 hotels nearby the coordinates %0.2f , %0.2f.
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
    `, userLocation.UserLat, userLocation.UserLon, userLocation.SearchRadiusKm)
}

func getActivitiesNearbyPrompt(userLocation UserLocation) string {
	if userLocation.SearchRadiusKm == 0 {
		userLocation.SearchRadiusKm = 5.0
	}
	return fmt.Sprintf(`
        Generate a list of up to 10 open air activities people can do within %.2f km of coordinates %.2f, %.2f.
        Include a variety of restaurant categories to provide diverse options.
        The result must be in JSON format:
        {
            "activities": [
                {
                    "name": "Activity Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "category where it belong",
                    "description": "Brief description of the activity and its proximity to the user's location."
                }
            ]
        }
    `, userLocation.SearchRadiusKm, userLocation.UserLat, userLocation.UserLon)
}

func getAttractionsNeabyPrompt(userLocation UserLocation) string {
	if userLocation.SearchRadiusKm == 0 {
		userLocation.SearchRadiusKm = 5.0
	}
	return fmt.Sprintf(`
        Generate a list of up to 10 attractions people can do within %.2f km of coordinates %.2f, %.2f.
        Include a variety of restaurant categories to provide diverse options.
        The result must be in JSON format:
        {
            "attractions": [
                {
                    "name": "Attractions Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "category where it belong",
                    "description": "Brief description of the attractions and its proximity to the user's location."
                }
            ]
        }
    `, userLocation.SearchRadiusKm, userLocation.UserLat, userLocation.UserLon)
}

func getGeneralPOIByDistance(lat, lon, distance float64) string {
	return fmt.Sprintf(`
            Generate a list of points of interest that people usually see no matter. Could be points of interest, bars, restaurants, hotels, activities, etc.
            The user location is at latitude %0.2f and longitude %0.2f.
            Only include points of interest that are within %0.2f kilometers from the user's location.
            Return the response STRICTLY as a JSON object with:
            {
            "points_of_interest": [
                {
                "name": "Name of the Point of Interest",
                "latitude": <float>,
                "longitude": <float>,
                "category": "Primary category (e.g., Museum, Historical Site, Park, Restaurant, Bar)",
                "description_poi": "A 2-3 sentence description of this specific POI and why it's relevant."
                }
            ]
            }`, lat, lon, distance/1000)
}
