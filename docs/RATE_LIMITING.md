# Rate Limiting Documentation

## Overview

The Loci API implements a sophisticated rate limiting system to prevent abuse, ensure fair usage, and protect LLM endpoints from excessive consumption. The system uses a token bucket algorithm with different tiers for different user types.

## Rate Limit Tiers

### Public Tier (Unauthenticated Users)
- **General Endpoints**: 30 requests/minute, 500 requests/hour, 2000 requests/day
- **LLM Endpoints**: 2 requests/minute, 10 requests/hour, 50 requests/day
- **Use Case**: Anonymous users, demos, public access

### Free Tier (Authenticated Free Users)
- **General Endpoints**: 10 requests/minute, 100 requests/hour, 500 requests/day
- **LLM Endpoints**: 5 requests/minute, 30 requests/hour, 100 requests/day
- **Use Case**: Free registered users

### Premium Tier (Paid Subscribers)
- **General Endpoints**: 100 requests/minute, 2000 requests/hour, 10000 requests/day
- **LLM Endpoints**: 30 requests/minute, 500 requests/hour, 2000 requests/day
- **Use Case**: Paying customers

### Admin Tier (System Administrators)
- **General Endpoints**: 1000 requests/minute, 10000 requests/hour, 50000 requests/day
- **LLM Endpoints**: 100 requests/minute, 2000 requests/hour, 10000 requests/day
- **Use Case**: Administrative operations

## Endpoint Categories

### LLM Endpoints (Stricter Limits)
- `/api/v1/llm/chat/stream/free`
- `/api/v1/llm/prompt-response/chat/sessions/stream/{profileID}`
- `/api/v1/llm/prompt-response/chat/sessions/{sessionID}/continue`
- Any endpoint containing `/llm/`, `/chat/`, or `/prompt-response/`

### General Endpoints (Standard Limits)
- All other API endpoints

## Configuration

### Environment Variables

```bash
# Disable rate limiting entirely (for development)
RATE_LIMIT_DISABLED=true

# Override free tier limits
RATE_LIMIT_FREE_RPM=10        # Requests per minute
RATE_LIMIT_FREE_RPH=100       # Requests per hour
RATE_LIMIT_FREE_RPD=500       # Requests per day

# Override premium tier limits
RATE_LIMIT_PREMIUM_RPM=100
RATE_LIMIT_PREMIUM_RPH=2000
RATE_LIMIT_PREMIUM_RPD=10000

# Override LLM limits for free tier
RATE_LIMIT_LLM_FREE_RPM=5
RATE_LIMIT_LLM_FREE_RPH=30
RATE_LIMIT_LLM_FREE_RPD=100

# Override LLM limits for premium tier
RATE_LIMIT_LLM_PREMIUM_RPM=30
RATE_LIMIT_LLM_PREMIUM_RPH=500
RATE_LIMIT_LLM_PREMIUM_RPD=2000
```

### Configuration File

Create `config/rate_limits.json` to override default settings:

```json
{
  "enabled": true,
  "default_limits": {
    "free": {
      "requests_per_minute": 15,
      "requests_per_hour": 150,
      "requests_per_day": 750,
      "burst_limit": 7,
      "window_size": "1m"
    }
  },
  "llm_limits": {
    "free": {
      "requests_per_minute": 3,
      "requests_per_hour": 20,
      "requests_per_day": 80,
      "burst_limit": 1,
      "window_size": "1m"
    }
  }
}
```

## HTTP Headers

### Response Headers

All API responses include rate limiting information:

```
X-RateLimit-Limit: 10          # Requests allowed per window
X-RateLimit-Remaining: 7        # Requests remaining in current window
X-RateLimit-Reset: 1640995200   # Unix timestamp when window resets
```

### Rate Limit Exceeded Response

When rate limit is exceeded, the API returns:

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 60
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1640995200

{
  "error": "Rate limit exceeded. Please try again later.",
  "code": 429,
  "timestamp": "2023-12-31T23:59:59Z"
}
```

## Client Implementation

### JavaScript/TypeScript

```typescript
async function makeAPIRequest(url: string, options: RequestInit = {}) {
  const response = await fetch(url, options);
  
  // Check rate limit headers
  const limit = response.headers.get('X-RateLimit-Limit');
  const remaining = response.headers.get('X-RateLimit-Remaining');
  const reset = response.headers.get('X-RateLimit-Reset');
  
  if (response.status === 429) {
    const retryAfter = response.headers.get('Retry-After');
    throw new Error(`Rate limit exceeded. Retry after ${retryAfter} seconds.`);
  }
  
  return response;
}
```

### Rate Limit Monitoring

```typescript
class RateLimitMonitor {
  private remaining: number = 0;
  private resetTime: number = 0;
  
  updateFromHeaders(headers: Headers) {
    this.remaining = parseInt(headers.get('X-RateLimit-Remaining') || '0');
    this.resetTime = parseInt(headers.get('X-RateLimit-Reset') || '0');
  }
  
  canMakeRequest(): boolean {
    return this.remaining > 0 || Date.now() / 1000 > this.resetTime;
  }
  
  getSecondsUntilReset(): number {
    return Math.max(0, this.resetTime - Date.now() / 1000);
  }
}
```

## Admin Management

### Get Current Configuration

```bash
GET /admin/rate-limits/config
Authorization: Bearer <admin-token>
```

### Update Configuration

```bash
PUT /admin/rate-limits/config
Authorization: Bearer <admin-token>
Content-Type: application/json

{
  "enabled": true,
  "default_limits": {
    "free": {
      "requests_per_minute": 20,
      "requests_per_hour": 200,
      "requests_per_day": 1000,
      "burst_limit": 10,
      "window_size": "1m"
    }
  }
}
```

### Get Rate Limit Statistics

```bash
GET /admin/rate-limits/stats
Authorization: Bearer <admin-token>
```

## Development

### Disabling Rate Limits

For development, you can disable rate limiting:

```bash
export RATE_LIMIT_DISABLED=true
# or
echo "RATE_LIMIT_DISABLED=true" >> .env
```

### Testing Rate Limits

```bash
# Test with curl
curl -H "Authorization: Bearer <token>" \
     -w "%{http_code} %header{X-RateLimit-Remaining}\n" \
     http://localhost:8080/api/v1/llm/chat/stream/free

# Rapid fire requests to test limiting
for i in {1..20}; do
  curl -s -H "Authorization: Bearer <token>" \
       -w "%{http_code}\n" \
       http://localhost:8080/api/v1/pois/search
done
```

## Monitoring and Alerting

### Metrics to Monitor

1. **Rate limit violations per endpoint**
2. **Top rate-limited users/IPs**
3. **Success rate after rate limiting implementation**
4. **Average requests per user by tier**

### Recommended Alerts

1. **High rate limit violation rate** (>10% of requests blocked)
2. **Specific users hitting limits frequently**
3. **LLM endpoint abuse patterns**
4. **Sudden spikes in anonymous traffic**

## Best Practices

### For API Consumers

1. **Monitor rate limit headers** in responses
2. **Implement exponential backoff** when rate limited
3. **Cache responses** when possible
4. **Batch requests** where the API supports it
5. **Consider upgrading to premium** for higher limits

### For API Developers

1. **Set conservative defaults** and increase as needed
2. **Monitor rate limit effectiveness** regularly
3. **Adjust limits based on** usage patterns and server capacity
4. **Provide clear error messages** when limits are exceeded
5. **Consider user experience** when setting limits

## Troubleshooting

### Common Issues

1. **"Rate limit exceeded" on first request**
   - Check if user tier is correctly identified
   - Verify configuration is loaded properly

2. **Rate limits not working**
   - Check if `RATE_LIMIT_DISABLED=true` is set
   - Verify middleware is properly mounted in router

3. **Different limits than expected**
   - Check environment variables override defaults
   - Verify config file is being loaded

### Debug Mode

Enable debug logging to see rate limit decisions:

```go
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
```

This will show detailed rate limit checks in the logs.