write a todo-discover.md on my server after analysing my routes
r.Get("/nearby", HandlerImpl.GetNearbyRecommendations)
// GET
http://localhost:8000/api/v1/llm/prompt-response/poi/nearby

// Domain-specific discover routes
r.Get("/discover/restaurants", HandlerImpl.GetNearbyRestaurants)
// GET http://localhost:8000/api/v1/pois/discover/restaurants
r.Get("/discover/activities", HandlerImpl.GetNearbyActivities)
// GET http://localhost:8000/api/v1/pois/discover/activities
r.Get("/discover/hotels", HandlerImpl.GetNearbyHotels)
// GET http://localhost:8000/api/v1/pois/discover/hotels
r.Get("/discover/attractions", HandlerImpl.GetNearbyAttractions)
// GET http://localhost:8000/api/v1/pois/discover/attractions and
the services that it calls. The question is
GetNearbyRecommendations gets the data from the LLM. The other
endpoints get the data based on location. wouldn't it make sense
for the other endpoints to also get the data from LLMs instead of
just querying the location with postgis ?

On the client, Create a nav bar item for Bookmarks that must have the itineraries saved on user_saved_itineraries and follow the same structure as the favourites with the same type of layout but with data each bookmarked item (itinerary, resturant, hotel etc)
I already have the method to add to bookmark, AddChatToBookmark(ctx context.Context, itinerary *types.UserSavedItinerary) (uuid.UUID, error), create repository and handler to list all them and 
display them under the new bookmark with the paginator working too. 
