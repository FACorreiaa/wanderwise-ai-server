package grpccacherequests

// non
//// Cache is a simple wrapper around a caching system (e.g., Redis).
//type Cache struct {
//	client *redis.Client
//	ttl    time.Duration
//}
//
//// NewCache initializes a new Redis-based cache.
//func NewCache() (*Cache, error) {
//	cfg, err := config.InitConfig()
//	if err != nil {
//		logger.Log.Error("failed to initialize config", zap.Error(err))
//		return nil, err
//	}
//
//	client := redis.NewClient(&redis.Options{
//		Addr: cfg.Repositories.Redis.Host + ":" + cfg.Repositories.Redis.Port,
//	})
//
//	// Test connection
//	_, err = client.Ping(context.Background()).Result()
//	if err != nil {
//		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
//	}
//	duration := time.Second * 60
//	if cfg.Repositories.Redis.TTL > 0 {
//		duration = cfg.Repositories.Redis.TTL
//	}
//
//	return &Cache{client: client, ttl: duration}, nil
//}
//
//// UnaryCachingInterceptor creates a new caching interceptor for unary gRPC calls.
//func UnaryCachingInterceptor(cache *Cache) grpc.UnaryServerInterceptor {
//	return func(
//		ctx context.Context,
//		req interface{},
//		info *grpc.UnaryServerInfo,
//		handler grpc.UnaryHandler,
//	) (interface{}, error) {
//		// Generate a cache key based on the method and request
//		cacheKey := generateCacheKey(info.FullMethod, req)
//
//		// Check if the response is already cached
//		if cachedResp, err := cache.Get(ctx, cacheKey); err == nil {
//			return cachedResp, nil
//		}
//
//		// Proceed with the gRPC handler
//		resp, err := handler(ctx, req)
//		if err != nil {
//			return nil, err
//		}
//
//		// Cache the response
//		if err := cache.Set(ctx, cacheKey, resp); err != nil {
//			// Log error, but don't fail the request
//			fmt.Printf("Failed to cache response: %v\n", err)
//		}
//
//		return resp, nil
//	}
//}
//
//// GenerateCacheKey generates a unique cache key based on the method and request.
//func generateCacheKey(method string, req interface{}) string {
//	reqJSON, _ := json.Marshal(req)
//	return fmt.Sprintf("%s:%x", method, reqJSON)
//}
//
//// Get retrieves a cached value for a given key.
//func (c *Cache) Get(ctx context.Context, key string) (interface{}, error) {
//	val, err := c.client.Get(ctx, key).Result()
//	if errors.Is(err, redis.Nil) {
//		return nil, status.Error(codes.NotFound, "cache miss")
//	}
//	if err != nil {
//		return nil, err
//	}
//
//	var result interface{}
//	if err := json.Unmarshal([]byte(val), &result); err != nil {
//		return nil, err
//	}
//	return result, nil
//}
//
//// Set stores a value in the cache with a TTL.
//func (c *Cache) Set(ctx context.Context, key string, value interface{}) error {
//	val, err := json.Marshal(value)
//	if err != nil {
//		return err
//	}
//	return c.client.Set(ctx, key, val, c.ttl).Err()
//}
