# Proto Schemas Index

Complete index of all proto schemas generated for ConnectRPC migration.

## Schema Overview

| Proto File | Package | Services | Messages | Purpose |
|------------|---------|----------|----------|---------|
| common.proto | loci.common | 0 | 7 | Shared types (Response, Pagination, GeoPoint, etc.) |
| auth.proto | loci.auth | AuthService | 14 | Authentication & authorization |
| user.proto | loci.user | UserService | 5 | User profile management |
| city.proto | loci.city | CityService | 6 | City information & search |
| poi.proto | loci.poi | POIService | 9 | Points of interest |
| interest.proto | loci.interest | InterestService | 12 | User interests & tags |
| profile.proto | loci.profile | ProfileService | 14 | User preference profiles |
| itinerary.proto | loci.itinerary | ItineraryService | 21 | Lists & itineraries |
| chat.proto | loci.chat | ChatService | 28 | Chat sessions & LLM interactions |
| discover.proto | loci.discover | DiscoverService | 12 | Discovery features |

## Services & RPC Methods

### AuthService (auth.proto)
- Login
- Register
- RefreshToken
- ValidateSession
- ChangePassword
- ChangeEmail
- Logout

### UserService (user.proto)
- GetUserProfile
- UpdateUserProfile

### CityService (city.proto)
- GetCity
- SearchCities

### POIService (poi.proto)
- SearchPOI
- GetPOI

### InterestService (interest.proto)
- GetInterests
- GetUserInterests
- CreateInterest
- UpdateInterest
- AddInterestToUser
- UpdatePreferenceLevel

### ProfileService (profile.proto)
- GetUserPreferenceProfiles
- CreateUserPreferenceProfile
- UpdateUserPreferenceProfile

### ItineraryService (itinerary.proto)
- CreateList
- UpdateList
- GetList
- GetUserLists
- AddListItem
- UpdateListItem
- GetUserItineraries
- UpdateItinerary
- BookmarkItinerary

### ChatService (chat.proto)
- StartChat
- ContinueChat
- GetChatSession
- GetChatSessions
- GetRecentInteractions
- EndSession
- StreamChat (streaming RPC)

### DiscoverService (discover.proto)
- GetDiscoverPage
- GetTrending
- GetFeatured
- GetRecentDiscoveries
- GetCategoryResults

## Message Types Summary

### Authentication (auth.proto)
- UserAuth, LoginRequest, LoginResponse
- RegisterRequest, TokenResponse
- ChangePasswordRequest, ChangeEmailRequest
- Claims (JWT structure)

### User (user.proto)
- UserProfile, UserStats
- UpdateProfileParams

### City (city.proto)
- CityDetail, GeneralCityData

### POI (poi.proto)
- POIDetailedInfo, HotelDetailedInfo, RestaurantDetailedInfo
- SearchPOIRequest, POIFilters

### Interests (interest.proto)
- Interest, Tags
- CreateInterestRequest, UpdateInterestRequest

### Profile (profile.proto)
- UserPreferenceProfile
- AccommodationPreferences, DiningPreferences
- ActivityPreferences, ItineraryPreferences
- Enums: DayPreference, SearchPace, TransportPreference

### Itinerary (itinerary.proto)
- List, ListItem, ListItemWithContent
- UserSavedItinerary
- Enum: ContentType

### Chat (chat.proto)
- ChatSession, ConversationMessage
- LlmInteraction, AiCityResponse
- SessionMetrics (Performance, Content, Engagement)
- StreamEvent, NavigationData
- Enums: MessageRole, MessageType, SessionStatus, IntentType, DomainType

### Discover (discover.proto)
- TrendingDiscovery, FeaturedCollection
- DiscoverResult, DiscoverPageData

## Enums

### profile.proto
- DayPreference: ANY, DAY, NIGHT
- SearchPace: ANY, RELAXED, MODERATE, FAST
- TransportPreference: ANY, WALK, PUBLIC, CAR

### itinerary.proto
- ContentType: POI, RESTAURANT, HOTEL, ITINERARY

### chat.proto
- MessageRole: USER, ASSISTANT, SYSTEM
- MessageType: INITIAL_REQUEST, MODIFICATION_REQUEST, RESPONSE, etc.
- SessionStatus: ACTIVE, EXPIRED, CLOSED
- IntentType: 16 different intent types
- DomainType: GENERAL, ACCOMMODATION, DINING, ACTIVITIES, ITINERARY, TRANSPORT

## Import Dependencies

```
common.proto (base - imported by most files)
  ↓
├─ auth.proto
├─ user.proto
├─ city.proto
├─ interest.proto
│   └─ profile.proto
│       └─ chat.proto
│           └─ discover.proto
└─ poi.proto
    └─ itinerary.proto
        └─ chat.proto
```

## File Sizes

- common.proto: ~1 KB
- auth.proto: ~2.7 KB
- user.proto: ~2.5 KB
- city.proto: ~1.6 KB
- poi.proto: ~3.5 KB
- interest.proto: ~2.4 KB
- profile.proto: ~6.9 KB
- itinerary.proto: ~6.1 KB
- chat.proto: ~9.2 KB
- discover.proto: ~3.4 KB

**Total: ~40 KB of proto definitions**

## Key Features

✅ Full type safety with protobuf
✅ Streaming support (StreamChat)
✅ Proper nullable fields (optional keyword)
✅ Rich enum types
✅ Nested message structures
✅ Pagination support
✅ Common types reuse
✅ Service definitions for ConnectRPC
✅ Compatible with gRPC

## Coverage

These proto schemas cover:
- ✅ Authentication & Authorization
- ✅ User Management
- ✅ City Information
- ✅ Points of Interest (POI, Hotels, Restaurants)
- ✅ User Interests & Tags
- ✅ User Preference Profiles
- ✅ Lists & Itineraries
- ✅ Chat Sessions & LLM Interactions
- ✅ Discovery Features
- ✅ Metrics & Analytics

Based on your internal/types/*.go files.
