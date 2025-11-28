# Client Migration Checklist - Protected Routes

## 🚨 BREAKING CHANGES - Action Required

All routes now require authentication except `/auth/*`, `/about`, `/features`, `/pricing`.

## Quick Path Changes

### ✅ Simplified Paths (No More `/user` Prefix)

| Old Path | New Path |
|----------|----------|
| ❌ `/api/v1/user/pois/favourites` | ✅ `/api/v1/pois/favourites` |
| ❌ `/api/v1/user/pois/itineraries` | ✅ `/api/v1/pois/itineraries` |
| ❌ `/api/v1/user/pois/nearby` | ✅ `/api/v1/pois/nearby` |

### 🔒 Now Require Authentication (were public)

- `/api/v1/pois/search`
- `/api/v1/pois/nearby`
- `/api/v1/pois/discover/*`
- `/api/v1/cities/*`
- `/api/v1/discover/*`
- `/api/v1/statistics/*`
- `/api/v1/llm/chat/stream/free` (was free, now requires auth)

## Client-Side Tasks

### 1. Update API Client

```typescript
// Find and replace all API calls
const apiClient = {
  async request(endpoint: string, options?: RequestInit) {
    const token = getAccessToken(); // Get from cookie/localStorage

    return fetch(`/api/v1${endpoint}`, {
      ...options,
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        ...options?.headers,
      },
    });
  }
};

// Usage
await apiClient.request('/pois/nearby?lat=41.1579&lon=-8.6291&distance=5000');
```

### 2. Update Path References

**Search & Replace in codebase:**

```bash
# Find old paths
grep -r "/api/v1/user/pois" src/

# Replace with new paths
/api/v1/user/pois/favourites → /api/v1/pois/favourites
/api/v1/user/pois/itineraries → /api/v1/pois/itineraries
```

### 3. Add Auth Guards

```typescript
// Protect all routes except public ones
const publicRoutes = ['/login', '/register', '/about', '/features', '/pricing'];

export function middleware(request: NextRequest) {
  const isPublic = publicRoutes.some(route =>
    request.nextUrl.pathname.startsWith(route)
  );

  if (!isPublic && !hasValidToken()) {
    return NextResponse.redirect('/login');
  }
}
```

### 4. Handle 401 Responses

```typescript
// Global error handler
async function handleResponse(response: Response) {
  if (response.status === 401) {
    // Token expired or invalid
    clearTokens();
    router.push('/login');
    throw new Error('Authentication required');
  }
  return response.json();
}
```

### 5. Update Views That Were Public

#### Near Me View
```typescript
// Before: Could access without login
useEffect(() => {
  fetchNearbyPOIs(); // Would work unauthenticated
}, []);

// After: Must be logged in
useEffect(() => {
  if (!isAuthenticated) {
    router.push('/login');
    return;
  }
  fetchNearbyPOIs(); // Now requires auth header
}, [isAuthenticated]);
```

#### Discover View
```typescript
// Before: Public discover page
<DiscoverPage /> // Anyone could access

// After: Protected
<AuthGuard>
  <DiscoverPage /> {/* Requires login */}
</AuthGuard>
```

## Testing Checklist

- [ ] All API calls include `Authorization` header
- [ ] 401 responses redirect to login
- [ ] Unauthenticated users can only access:
  - [ ] `/login`
  - [ ] `/register`
  - [ ] `/about`
  - [ ] `/features`
  - [ ] `/pricing`
- [ ] Authenticated users can access all features
- [ ] Token refresh works correctly
- [ ] Logout clears tokens and redirects to login

## API Endpoints Reference

### Public (No Auth Required)
```
POST /api/v1/auth/register
POST /api/v1/auth/login
GET  /api/v1/auth/google
GET  /api/v1/auth/google/callback
POST /api/v1/auth/refresh
GET  /api/v1/about (TODO)
GET  /api/v1/features (TODO)
GET  /api/v1/pricing (TODO)
```

### Protected (Auth Required) - Everything Else
```
# POI Routes
GET    /api/v1/pois/search
GET    /api/v1/pois/nearby
GET    /api/v1/pois/favourites
POST   /api/v1/pois/favourites
DELETE /api/v1/pois/favourites

# Discover Routes
GET /api/v1/discover
GET /api/v1/discover/trending
GET /api/v1/discover/featured

# City Routes
GET /api/v1/cities
GET /api/v1/cities/{cityID}

# Statistics Routes
GET /api/v1/statistics/*

# LLM Routes (ALL now require auth)
POST /api/v1/llm/chat/stream
POST /api/v1/llm/chat/stream/free

# User Routes
GET /api/v1/user/profile
PUT /api/v1/user/profile
```

## Common Errors & Solutions

### Error: 401 Unauthorized
**Cause**: Missing or invalid JWT token
**Solution**: Add `Authorization: Bearer <token>` header

### Error: 404 Not Found on `/api/v1/user/pois/*`
**Cause**: Using old path with `/user` prefix
**Solution**: Remove `/user` → use `/api/v1/pois/*` directly

### Error: CORS on auth requests
**Cause**: Credentials not included
**Solution**: Ensure cookies are sent with requests

## Deployment Order

1. ✅ Deploy server changes (already done)
2. ⬜ Deploy client changes (you're doing this now)
3. ⬜ Monitor 401 error rates
4. ⬜ Verify user flows work end-to-end

## Support

If you encounter issues:
1. Check server logs for authentication errors
2. Verify JWT token is valid and not expired
3. Ensure Authorization header format is correct
4. Check CORS settings if requests are blocked

## Timeline

- **Server deployed**: 2025-11-26
- **Client migration**: TBD
- **Full cutover**: After client testing complete
