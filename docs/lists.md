# Lists & Itineraries API

This document outlines the functionality of the Lists and Itineraries API, which allows users to create, manage, and share collections of points of interest (POIs), itineraries, and other content.

## Core Concepts

*   **List**: A generic container that can hold various types of content. A list can be a top-level entity or a sub-list within another list.
*   **Itinerary**: A special type of list that represents a travel plan, often with day-by-day scheduling.
*   **List Item**: An entry in a list, which can be a POI, another list (sub-itinerary), a restaurant, a hotel, etc.

## Endpoints

### List Management

#### `POST /api/v1/itineraries/lists`

*   **Description**: Creates a new top-level list or itinerary.
*   **Handler**: `CreateTopLevelListHandler`
*   **Service Method**: `CreateTopLevelList`
*   **Repository Method**: `CreateList`
*   **Request Body**:
    *   `name` (string, required): The name of the list.
    *   `description` (string): A description of the list.
    *   `is_public` (boolean): Whether the list is public or private.
    *   `is_itinerary` (boolean): Whether the list is an itinerary.
    *   `city_id` (UUID, optional): The ID of the city associated with the list.
*   **Response**: The newly created list object.

#### `POST /api/v1/itineraries/lists/{parentListID}/itineraries`

*   **Description**: Creates a new itinerary within a parent list.
*   **Handler**: `CreateItineraryForListHandler`
*   **Service Method**: `CreateItineraryForList`
*   **Repository Method**: `CreateList`
*   **Request Body**:
    *   `name` (string, required): The name of the itinerary.
    *   `description` (string): A description of the itinerary.
    *   `is_public` (boolean): Whether the itinerary is public or private.
*   **Response**: The newly created itinerary object.

#### `GET /api/v1/itineraries/lists`

*   **Description**: Retrieves all lists for the authenticated user.
*   **Handler**: `GetUserListsHandler`
*   **Service Method**: `GetUserLists`
*   **Repository Method**: `GetUserLists`
*   **Query Parameters**:
    *   `type` (string, optional): Filter by list type (`itinerary` or `collection`).
*   **Response**: An array of list objects.

#### `GET /api/v1/itineraries/lists/{listID}`

*   **Description**: Retrieves the details of a specific list, including its items.
*   **Handler**: `GetListDetailsHandler`
*   **Service Method**: `GetListDetails`
*   **Repository Methods**: `GetList`, `GetListItems`
*   **Response**: A list object with an array of its items.

#### `PUT /api/v1/itineraries/lists/{listID}`

*   **Description**: Updates the details of a specific list.
*   **Handler**: `UpdateListDetailsHandler`
*   **Service Method**: `UpdateListDetails`
*   **Repository Methods**: `GetList`, `UpdateList`
*   **Request Body**:
    *   `name` (string, optional): The new name of the list.
    *   `description` (string, optional): The new description of the list.
    *   `is_public` (boolean, optional): The new privacy setting.
    *   `city_id` (UUID, optional): The new city ID.
*   **Response**: The updated list object.

#### `DELETE /api/v1/itineraries/lists/{listID}`

*   **Description**: Deletes a specific list.
*   **Handler**: `DeleteListHandler`
*   **Service Method**: `DeleteUserList`
*   **Repository Methods**: `GetList`, `DeleteList`
*   **Response**: `204 No Content`

### List Item Management

#### `POST /api/v1/itineraries/lists/{listID}/items`

*   **Description**: Adds an item to a list.
*   **Handler**: `AddListItemHandler`
*   **Service Method**: `AddListItem`
*   **Repository Methods**: `GetList`, `AddListItem`
*   **Request Body**:
    *   `item_id` (UUID, required): The ID of the item to add.
    *   `content_type` (string, required): The type of content being added (e.g., `poi`, `itinerary`, `restaurant`, `hotel`).
    *   `position` (integer, optional): The position of the item in the list.
    *   `notes` (string, optional): Notes about the item.
    *   `day_number` (integer, optional): The day number for itinerary items.
    *   `time_slot` (time, optional): The time slot for itinerary items.
    *   `duration_minutes` (integer, optional): The duration in minutes for itinerary items.
*   **Response**: The newly created list item object.

#### `PUT /api/v1/itineraries/lists/{listID}/items/{itemID}`

*   **Description**: Updates an item in a list.
*   **Handler**: `UpdateListItemHandler`
*   **Service Method**: `UpdateListItem`
*   **Repository Methods**: `GetList`, `GetListItemByID`, `UpdateListItem`
*   **Request Body**: Same as the request body for adding an item.
*   **Response**: The updated list item object.

#### `DELETE /api/v1/itineraries/lists/{listID}/items/{itemID}`

*   **Description**: Removes an item from a list.
*   **Handler**: `RemoveListItemHandler`
*   **Service Method**: `RemoveListItem`
*   **Repository Methods**: `GetList`, `DeleteListItemByID`
*   **Response**: `204 No Content`

### Saved Lists

#### `POST /api/v1/itineraries/lists/{listID}/save`

*   **Description**: Saves a public list to the user's saved lists.
*   **Handler**: `SaveListHandler`
*   **Service Method**: `SaveList`
*   **Repository Methods**: `GetList`, `SaveList`
*   **Response**: A success message.

#### `DELETE /api/v1/itineraries/lists/{listID}/save`

*   **Description**: Unsaves a list from the user's saved lists.
*   **Handler**: `UnsaveListHandler`
*   **Service Method**: `UnsaveList`
*   **Repository Method**: `UnsaveList`
*   **Response**: `204 No Content`

#### `GET /api/v1/itineraries/lists/saved`

*   **Description**: Retrieves all lists saved by the authenticated user.
*   **Handler**: `GetUserSavedListsHandler`
*   **Service Method**: `GetUserSavedLists`
*   **Repository Method**: `GetUserSavedLists`
*   **Response**: An array of list objects.

### Search and Filtering

#### `GET /api/v1/itineraries/lists/search`

*   **Description**: Searches public lists.
*   **Handler**: `SearchListsHandler`
*   **Service Method**: `SearchLists`
*   **Repository Method**: `SearchLists`
*   **Query Parameters**:
    *   `search` (string, optional): A search term to match against list names and descriptions.
    *   `category` (string, optional): A category to filter by.
    *   `content_type` (string, optional): A content type to filter by.
    *   `theme` (string, optional): A theme to filter by.
    *   `city_id` (UUID, optional): A city ID to filter by.
*   **Response**: An array of list objects that match the search criteria.

### Content-Type Specific Endpoints

These endpoints are provided for convenience to retrieve items of a specific content type from a list.

*   `GET /api/v1/itineraries/lists/{listID}/items/restaurants`: Retrieves all restaurant items from a list.
*   `GET /api/v1/itineraries/lists/{listID}/items/hotels`: Retrieves all hotel items from a list.
*   `GET /api/v1/itineraries/lists/{listID}/items/itineraries`: Retrieves all itinerary items from a list.

### Legacy POI-Specific Endpoints

These endpoints are maintained for backward compatibility.

*   `POST /api/v1/itineraries/{itineraryID}/items`: Adds a POI to an itinerary.
*   `PUT /api/v1/itineraries/{itineraryID}/items/{poiID}`: Updates a POI in an itinerary.
*   `DELETE /api/v1/itineraries/{itineraryID}/items/{poiID}`: Removes a POI from an itinerary.
