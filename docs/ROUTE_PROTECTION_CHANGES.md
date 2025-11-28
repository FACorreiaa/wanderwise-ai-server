# Route Protection Changes - All Routes Now Require Authentication

## Date
2025-11-26

## Change Summary

**All API routes now require authentication except for:**
- `/api/v1/auth/*` - Authentication endpoints
- `/api/v1/about` - About page (TODO: implement)
- `/api/v1/features` - Features page (TODO: implement)
- `/api/v1/pricing` - Pricing page (TODO: implement)

## Motivation

The application is transitioning to a fully authenticated model where users must be logged in to access any functionality. This provides:
- Better user tracking and analytics
- Personalized experiences
- Security and data protection
- Monetization opportunities

## Routes Changed

### Previously Public, Now Protected

#### 1. POI Routes (`/api/v1/pois/*`)
All POI endpoints now require authentication:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/pois/search` | GET | Search POIs by query & city |
| `/api/v1/pois/search/semantic` | GET | Semantic search |
| `/api/v1/pois/search/semantic/city` | GET | Semantic search by city |
| `/api/v1/pois/search/hybrid` | GET | Hybrid search |
| `/api/v1/pois/nearby` | GET | Get nearby POIs |
| `/api/v1/pois/discover/restaurants` | GET | Discover restaurants |
| `/api/v1/pois/discover/activities` | GET | Discover activities |
| `/api/v1/pois/discover/hotels` | GET | Discover hotels |
| `/api/v1/pois/discover/attractions` | GET | Discover attractions |
| `/api/v1/pois/city/{cityID}` | GET | Get POIs by city |
| `/api/v1/pois/itineraries` | GET | Get user itineraries |
| `/api/v1/pois/favourites` | GET/POST/DELETE | Manage favorites |

#### 2. City Routes (`/api/v1/cities/*`)
All city endpoints now require authentication:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/cities` | GET | List all cities |
| `/api/v1/cities/{cityID}` | GET | Get city by ID |

#### 3. Discover Routes (`/api/v1/discover/*`)
All discover endpoints now require authentication:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/discover` | GET | Get discover page data |
| `/api/v1/discover/trending` | GET | Get trending discoveries |
| `/api/v1/discover/featured` | GET | Get featured collections |
| `/api/v1/discover/recent` | GET | Get recent discoveries |
| `/api/v1/discover/category/{category}` | GET | Get category results |

#### 4. Statistics Routes (`/api/v1/statistics/*`)
All statistics endpoints now require authentication:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/statistics/main-page` | GET | Get main page statistics |

#### 5. LLM Routes (`/api/v1/llm/*`)
All LLM interaction endpoints now require authentication (including the previously free endpoint):

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/llm/chat/stream` | POST | Start chat stream (authenticated) |
| `/api/v1/llm/chat/stream/free` | POST | **Now requires auth** (was public) |
| All other LLM endpoints | Various | Require authentication |

### Still Public

#### Authentication Routes (`/api/v1/auth/*`)

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/auth/register` | POST | User registration |
| `/api/v1/auth/login` | POST | User login |
| `/api/v1/auth/google` | GET | Google OAuth login |
| `/api/v1/auth/google/callback` | GET | Google OAuth callback |
| `/api/v1/auth/refresh` | POST | Refresh access token |

#### Marketing/Info Routes (TODO - Not Yet Implemented)

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/about` | GET | About page |
| `/api/v1/features` | GET | Features page |
| `/api/v1/pricing` | GET | Pricing page |

## Code Changes

### File: `internal/router/router.go`

#### Before (lines 68-86):
```go
r.Group(func(r chi.Router) {
    // Public routes
    r.Post("/auth/register", cfg.AuthHandler.Register)
    r.Post("/auth/login", cfg.AuthHandler.Login)
    // ... other auth routes

    // Public city routes
    r.Mount("/cities", CityRoutes(cfg.CityHandler))
    r.Mount("/statistics", StatisticsRoutes(cfg.StatisticsHandler))
    r.Mount("/discover", DiscoverRoutes(cfg.DiscoverHandler))
    r.Mount("/pois", POIPublicRoutes(cfg.PointsOfInterestHandler))
})
```

#### After (lines 68-81):
```go
r.Group(func(r chi.Router) {
    // ONLY auth, about, features, and pricing are public
    r.Post("/auth/register", cfg.AuthHandler.Register)
    r.Post("/auth/login", cfg.AuthHandler.Login)
    // ... other auth routes

    // Marketing/informational routes (TODO: implement)
    // r.Get("/about", cfg.InfoHandler.GetAbout)
    // r.Get("/features", cfg.InfoHandler.GetFeatures)
    // r.Get("/pricing", cfg.InfoHandler.GetPricing)
})
```

#### Protected Routes (lines 85-119):
```go
r.Group(func(r chi.Router) {
    r.Use(cfg.AuthenticateMiddleware)

    // All routes now under authentication:
    r.Mount("/pois", POIProtectedRoutes(...))
    r.Mount("/cities", CityRoutes(...))
    r.Mount("/discover", DiscoverRoutes(...))
    r.Mount("/statistics", StatisticsRoutes(...))
    r.Mount("/llm", LLMInteractionRoutes(...))
    r.Mount("/itineraries", ItineraryListRoutes(...))
    r.Mount("/recents", RecentsRoutes(...))
})
```

### POI Routes Consolidation

**Before**: Two separate functions
- `POIPublicRoutes()` - For unauthenticated access
- `POIProtectedRoutes()` - For authenticated access

**After**: Single function (lines 229-269)
- `POIProtectedRoutes()` - All POI routes require authentication

### Path Changes

| Old Path | New Path | Notes |
|----------|----------|-------|
| `/api/v1/pois/favourites` | `/api/v1/pois/favourites` | ✅ Path unchanged, now requires auth |
| `/api/v1/pois/nearby` | `/api/v1/pois/nearby` | ✅ Path unchanged, now requires auth |
| `/api/v1/pois/search` | `/api/v1/pois/search` | ✅ Path unchanged, now requires auth |
| `/api/v1/user/pois/*` | `/api/v1/pois/*` | ⚠️ Simplified - removed `/user` prefix |

## Client-Side Migration Guide

### 1. Add Authentication Headers

All API requests (except auth, about, features, pricing) must include JWT token:

```typescript
// Before (some routes allowed without auth)
fetch('/api/v1/pois/nearby?lat=41.1579&lon=-8.6291&distance=5000')

// After (all routes require auth)
fetch('/api/v1/pois/nearby?lat=41.1579&lon=-8.6291&distance=5000', {
  headers: {
    'Authorization': `Bearer ${accessToken}`
  }
})
```

### 2. Update Protected Paths

Routes previously at `/api/v1/user/pois/*` are now at `/api/v1/pois/*`:

```typescript
// Before
GET /api/v1/user/pois/favourites

// After
GET /api/v1/pois/favourites
```

### 3. Handle 401 Unauthorized

All routes now return 401 if not authenticated:

```typescript
try {
  const response = await fetch('/api/v1/pois/nearby', {
    headers: { 'Authorization': `Bearer ${token}` }
  });

  if (response.status === 401) {
    // Redirect to login
    router.push('/login');
  }
} catch (error) {
  // Handle error
}
```

### 4. Implement Auth Guard

Protect all client routes except:
- `/login`
- `/register`
- `/about`
- `/features`
- `/pricing`

```typescript
// Example Next.js middleware
export function middleware(request: NextRequest) {
  const publicPaths = ['/login', '/register', '/about', '/features', '/pricing'];
  const isPublic = publicPaths.some(path => request.nextUrl.pathname.startsWith(path));

  if (!isPublic && !request.cookies.get('accessToken')) {
    return NextResponse.redirect(new URL('/login', request.url));
  }
}
```

## Testing

### Test Authentication Required

```bash
# Should return 401 Unauthorized
curl http://localhost:8000/api/v1/pois/nearby?lat=41.1579&lon=-8.6291&distance=5000

# Should return 200 OK with data
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  http://localhost:8000/api/v1/pois/nearby?lat=41.1579&lon=-8.6291&distance=5000
```

### Test Public Routes Still Work

```bash
# Should return 200 OK
curl -X POST http://localhost:8000/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'

# Should return 200 OK
curl -X POST http://localhost:8000/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"new@example.com","password":"password123","username":"newuser"}'
```

## Rollback Plan

If needed to rollback, revert commit and redeploy. The old public routes are preserved in git history.

```bash
git revert <commit-hash>
git push origin main
```

## Performance Considerations

- **Authentication overhead**: JWT validation adds ~1-2ms per request
- **Caching**: User-specific responses can now be cached more effectively
- **Analytics**: All requests are now tied to authenticated users

## Security Benefits

1. **User tracking**: All actions tied to user accounts
2. **Rate limiting**: Per-user rate limits instead of IP-based
3. **Data protection**: No anonymous access to sensitive data
4. **Abuse prevention**: Harder to scrape or abuse APIs

## Monitoring

Watch for:
- Increase in 401 responses (expected during client migration)
- Auth endpoint load (may increase)
- User complaints about login requirements

## Next Steps

1. ✅ Server-side changes complete
2. ⬜ Client-side migration (update all API calls)
3. ⬜ Implement `/about`, `/features`, `/pricing` endpoints
4. ⬜ Update API documentation
5. ⬜ Monitor error rates and user feedback

## Related Files

- `internal/router/router.go` - Main routing configuration
- `internal/api/poi/poi_handler.go` - POI handlers (user ID now always available)
- `internal/middleware/auth.go` - Authentication middleware

## Breaking Changes

⚠️ **BREAKING CHANGE**: All previously public routes now require authentication. Client applications must:
1. Ensure users are authenticated before accessing any feature
2. Include JWT token in all API requests
3. Handle 401 responses by redirecting to login
4. Update hardcoded paths from `/api/v1/user/pois/*` to `/api/v1/pois/*`
