# Near Me View Fix - Authentication Issue

## Problem

The "nearme" view was failing with a 401 Unauthorized error when trying to fetch nearby POIs.

### Error Logs
```
9:23PM ERR poi/poi_handler.go:1115 User ID not found in context HandlerImpl=GetPOIsByDistance
9:23PM INF logger/logger.go:40 Request completed method=GET path=/api/v1/pois/nearby status=401
```

## Root Cause

The `/nearby` endpoint was registered in **both** public and protected routes:

1. **Public Route** (`internal/router/router.go:232`):
   ```go
   // POIPublicRoutes - NO authentication middleware
   r.Get("/nearby", HandlerImpl.GetNearbyRecommendations)
   ```

2. **Protected Route** (`internal/router/router.go:260`):
   ```go
   // POIUserRoutes - WITH authentication middleware
   r.Get("/nearby", HandlerImpl.GetNearbyRecommendations)
   ```

However, the handler (`GetNearbyRecommendations` in `poi_handler.go:1073`) **always required** a user ID from the authentication context:

```go
// Old code - ALWAYS required auth
userIDStr, ok := auth.GetUserIDFromContext(ctx)
if !ok || userIDStr == "" {
    l.ErrorContext(ctx, "User ID not found in context")
    api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
    return
}
```

### The Issue
- When users accessed `/api/v1/pois/nearby` without authentication, it matched the **public route**
- No authentication middleware ran, so no user ID was set in the context
- Handler tried to extract user ID → failed → returned 401

## Solution

Modified the handler to make user ID **optional** (`poi_handler.go:1112-1128`):

```go
// Get user ID from context (optional - works for both authenticated and unauthenticated users)
var userID uuid.UUID
userIDStr, ok := auth.GetUserIDFromContext(ctx)
if ok && userIDStr != "" {
    parsedID, err := uuid.Parse(userIDStr)
    if err != nil {
        l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
        api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
        return
    }
    userID = parsedID
    l.DebugContext(ctx, "Fetching nearby POIs for authenticated user", slog.String("user_id", userID.String()))
} else {
    // Unauthenticated user - use nil UUID
    userID = uuid.Nil
    l.DebugContext(ctx, "Fetching nearby POIs for unauthenticated user")
}
```

### Behavior After Fix

- **Unauthenticated users**: `userID = uuid.Nil` → returns generic nearby POIs
- **Authenticated users**: `userID = <actual UUID>` → returns personalized nearby POIs (can include favorites, preferences, etc.)

## Related Issues

### Favourites Endpoint (404)

The `/api/v1/pois/favourites` endpoint returned 404.

**Root Cause**: Incorrect path in client code.

Protected POI routes are mounted at `/user/pois`, not `/pois` (`internal/router/router.go:107`):

```go
r.Mount("/user/pois", POIProtectedRoutes(cfg.PointsOfInterestHandler))
```

Inside `POIProtectedRoutes` (line 263):
```go
r.Get("/favourites", HandlerImpl.GetFavouritePOIsByUserID)
```

**Solution**: Update client to use correct paths:

| Feature | ❌ Wrong Path | ✅ Correct Path |
|---------|--------------|-----------------|
| Favourites (GET) | `/api/v1/pois/favourites` | `/api/v1/user/pois/favourites` |
| Favourites (POST) | `/api/v1/pois/favourites` | `/api/v1/user/pois/favourites` |
| Favourites (DELETE) | `/api/v1/pois/favourites` | `/api/v1/user/pois/favourites` |
| Nearby (public) | `/api/v1/pois/nearby` | ✅ Correct |
| Nearby (authenticated) | `/api/v1/pois/nearby` | ✅ Also works (dual registration) |
| Itineraries | `/api/v1/pois/itineraries` | `/api/v1/user/pois/itineraries` |

### Why This Design?

The router is organized into:

1. **Public POI Routes** (`/api/v1/pois/*`):
   - Search, nearby, discover
   - No authentication required
   - Read-only operations

2. **Protected POI Routes** (`/api/v1/user/pois/*`):
   - Favourites, itineraries, embeddings
   - Requires authentication
   - User-specific operations

## Testing

### Test Unauthenticated Access
```bash
curl "http://localhost:8000/api/v1/pois/nearby?lat=41.1579&lon=-8.6291&distance=5000"
# Should return 200 OK with generic nearby POIs
```

### Test Authenticated Access
```bash
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  "http://localhost:8000/api/v1/pois/nearby?lat=41.1579&lon=-8.6291&distance=5000"
# Should return 200 OK with personalized nearby POIs
```

## Lessons Learned

1. **Dual Route Registration**: Be careful when mounting the same handler on both public and protected routes
2. **Handler Flexibility**: Handlers should gracefully handle both authenticated and unauthenticated requests if they serve both routes
3. **Error Messages**: "User ID not found in context" was the key diagnostic message
4. **Authentication Middleware**: Only runs on protected routes - public routes have no user context

## Files Changed

- `internal/api/poi/poi_handler.go:1112-1128` - Made user ID optional in `GetNearbyRecommendations`

## Date

2025-11-26
