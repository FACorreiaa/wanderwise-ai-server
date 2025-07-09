# TODO: Discover Endpoints Analysis and Recommendations

## Current Architecture Analysis

### Route Structure
```
/api/v1/pois/nearby                      -> GetNearbyRecommendations (LLM-based)
/api/v1/pois/discover/restaurants        -> GetNearbyRestaurants (Location-based)
/api/v1/pois/discover/activities         -> GetNearbyActivities (Location-based)
/api/v1/pois/discover/hotels            -> GetNearbyHotels (Location-based)
/api/v1/pois/discover/attractions       -> GetNearbyAttractions (Location-based)
```

### Current Implementation Differences

#### GetNearbyRecommendations (`poi_handler.go:993`)
- **Primary Logic**: Falls back to LLM when database has no results
- **Workflow**: 
  1. Check cache for existing POIs
  2. Query database with PostGIS spatial queries
  3. **If no results**: Generate POIs using LLM (`generatePOIsFromLLM`)
  4. Cache and return results
- **Source**: Mixed (database → LLM fallback)

#### Domain-Specific Endpoints (`poi_handler.go:1068-1262`)
- **Primary Logic**: Database-first with basic LLM fallback
- **Workflow**:
  1. Check cache
  2. Query database with category filters (`GetPOIsByLocationAndDistanceWithCategory`)
  3. Apply domain-specific filters
  4. **If no results**: Basic LLM generation (placeholder implementation)
- **Source**: Primarily database-based

## Key Implementation Differences

### Service Layer Analysis (`poi_service.go`)

1. **GetGeneralPOIByDistance** (line 591):
   - Robust LLM integration with `generatePOIsFromLLM`
   - Sophisticated caching strategy
   - Full LLM interaction logging
   - Enrichment and filtering of LLM responses

2. **Domain-Specific Methods** (lines 821-1042):
   - Database queries with category filtering
   - Basic domain-specific filters
   - **Placeholder LLM functions** (lines 1137-1159):
     ```go
     func (s *ServiceImpl) generateRestaurantsFromLLM(...) (*types.GenAIResponse, error) {
         // For now, use the general LLM generation with modified prompt
         // This would be enhanced with restaurant-specific prompts
         return s.generatePOIsFromLLM(ctx, userID, lat, lon, distance)
     }
     ```

## Issues Identified

### 1. Inconsistent Data Sources
- **GetNearbyRecommendations**: Intelligent LLM fallback with user context
- **Domain endpoints**: Basic database queries without sophisticated LLM integration

### 2. Limited LLM Utilization in Domain Endpoints
- Domain-specific LLM functions are **placeholders**
- Missing specialized prompts for restaurants, hotels, activities, attractions
- No user preference integration in domain-specific LLM calls

### 3. Filter Logic Inconsistency
- Domain endpoints use basic string matching for filters
- No semantic understanding of user preferences
- Limited personalization capabilities

## Recommendations

### Option 1: Enhance Domain Endpoints with LLM Integration ✅ **RECOMMENDED**

#### Benefits:
- **Consistent user experience** across all discovery endpoints
- **Personalized recommendations** based on user context and preferences
- **Better data quality** through LLM-generated content when database is sparse
- **Semantic understanding** of complex queries (e.g., "romantic restaurants", "family-friendly activities")

#### Implementation:
1. **Implement specialized LLM prompts** for each domain:
   ```go
   func (s *ServiceImpl) generateRestaurantsFromLLM(..., cuisineType, priceRange string) {
       prompt := fmt.Sprintf(`Generate restaurant recommendations for coordinates %f,%f 
                              within %f meters. Focus on %s cuisine with %s price range...`, 
                              lat, lon, distance, cuisineType, priceRange)
       // Enhanced LLM call with domain-specific context
   }
   ```

2. **Add user preference integration**:
   - Fetch user's dietary restrictions, cuisine preferences
   - Include past favorites and search history
   - Consider time of day, weather, season

3. **Enhance filtering logic**:
   - Semantic matching for categories
   - Price range intelligence
   - Rating and review analysis

#### Code Changes Required:
- `poi_service.go:1137-1159`: Implement proper domain-specific LLM functions
- `poi_service.go:821-1042`: Enhance domain methods with better LLM integration
- Add user preference models and queries
- Create domain-specific prompt templates

### Option 2: Simplify GetNearbyRecommendations to Match Domain Endpoints ❌ **NOT RECOMMENDED**

#### Why Not:
- **Reduces functionality** rather than improving it
- **Loses LLM capabilities** that provide better user experience
- **Inconsistent with product vision** of AI-powered recommendations

### Option 3: Hybrid Approach with Smart Routing

#### Implementation:
1. **GetNearbyRecommendations**: Keep as intelligent, context-aware general discovery
2. **Domain endpoints**: Enhance with LLM but maintain category focus
3. **Smart routing**: Frontend determines when to use general vs. domain-specific based on query complexity

## Technical Architecture Recommendations

### 1. Unified LLM Service Layer
```go
type DomainLLMService interface {
    GenerateRestaurants(ctx context.Context, userID uuid.UUID, location Location, filters RestaurantFilters) (*types.GenAIResponse, error)
    GenerateActivities(ctx context.Context, userID uuid.UUID, location Location, filters ActivityFilters) (*types.GenAIResponse, error)
    GenerateHotels(ctx context.Context, userID uuid.UUID, location Location, filters HotelFilters) (*types.GenAIResponse, error)
    GenerateAttractions(ctx context.Context, userID uuid.UUID, location Location, filters AttractionFilters) (*types.GenAIResponse, error)
}
```

### 2. Enhanced User Context Integration
```go
type UserContext struct {
    UserID          uuid.UUID
    Preferences     UserPreferences
    SearchHistory   []SearchQuery
    FavoriteVenues  []POI
    DietaryRestrictions []string
    BudgetRange     string
}
```

### 3. Intelligent Fallback Strategy
1. **Primary**: Check database with semantic similarity
2. **Secondary**: Generate with LLM using user context
3. **Tertiary**: Cache and learn from user interactions

## Conclusion

**The domain-specific endpoints should be enhanced to match the intelligence and personalization of GetNearbyRecommendations.** This approach will:

- Provide consistent, high-quality recommendations across all discovery endpoints
- Leverage AI capabilities for better user experience
- Maintain the specialized nature of domain-specific searches
- Enable future enhancements like multi-modal queries and real-time personalization

The current implementation leaves significant value on the table by not utilizing LLMs effectively in domain-specific discovery, resulting in a fragmented user experience where general discovery is intelligent but category-specific discovery is basic.