1. right now on the /discover view when I try to add a favourite I get the following error message:

{
"error": "Failed to save itinerary: failed to insert favorite LLM POI: failed to insert into user_favorite_llm_pois: ERROR: insert or update on table \"user_favorite_llm_pois\" violates foreign key constraint \"user_favorite_llm_pois_llm_poi_id_fkey\" (SQLSTATE 23503)",
"request_id": "MacBook-Pro-von-Fernando.local/vG6ASPuaiy-000126",
"success": false
}

This used to be working, I dont know what ruined it.
Request URL
http://localhost:8000/api/v1/pois/favourites
Request Method
POST
Status Code
500 Internal Server Error
Remote Address
[::1]:8000
Referrer Policy
strict-origin-when-cross-origin


2. When searching for an Itinerary or any kind of conversation with the llm (itinerary, activities, hotels, restaurants), 
each new interaction should be a new chat session. the same when the user clicks on the /chat. Should be a new chat session.
What happens when I search for a city now and try to bookmark a city is that I still get the previous city state.
This is causing issues on deleting a bookmark because the state isnt synced. Right now there is another problem:

Request URL
http://localhost:8000/api/v1/llm/prompt-response/bookmark
Request Method
POST
Status Code
500 Internal Server Error
Remote Address
[::1]:8000
Referrer Policy
strict-origin-when-cross-origin

{
"error": "Failed to save itinerary: failed to insert user_saved_itineraries: ERROR: insert or update on table \"user_saved_itineraries\" violates foreign key constraint \"user_saved_itineraries_source_llm_interaction_id_fkey\" (SQLSTATE 23503)",
"request_id": "MacBook-Pro-von-Fernando.local/vG6ASPuaiy-000141",
"success": false
}

I dont know what is causing this now. 

3. On the recents /recents when clicking on an interaction, the recent activity card should have more detailed information and the interactoons and places should be filled as well. Places should have the POIS and Itinerary points. 

Fix all this. 