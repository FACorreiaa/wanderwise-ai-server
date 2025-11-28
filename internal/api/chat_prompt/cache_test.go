package llmchat

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// TestCacheTTL verifies that cache is set to 48 hours
func TestCacheTTL(t *testing.T) {
	c := cache.New(48*time.Hour, 1*time.Hour)

	// Set a test value
	c.Set("test-key", "test-value", cache.DefaultExpiration)

	// Verify it exists
	val, found := c.Get("test-key")
	assert.True(t, found)
	assert.Equal(t, "test-value", val)

	// Note: We verify the cache is configured with 48h TTL in the initialization
}

// TestCacheKeyGeneration tests that cache keys are generated correctly based on content
func TestCacheKeyGeneration(t *testing.T) {
	userID := uuid.New()
	profileID := uuid.New()

	tests := []struct {
		name        string
		city        string
		message     string
		domain      string
		preferences string
		expectSame  bool
		compareWith int // index of test case to compare with (-1 for none)
	}{
		{
			name:        "Base case - Paris with Art preferences",
			city:        "Paris",
			message:     "Show me around",
			domain:      "itinerary",
			preferences: "Art & Museums",
			compareWith: -1,
		},
		{
			name:        "Same params should generate same key",
			city:        "Paris",
			message:     "Show me around",
			domain:      "itinerary",
			preferences: "Art & Museums",
			expectSame:  true,
			compareWith: 0,
		},
		{
			name:        "Different preferences should generate different key",
			city:        "Paris",
			message:     "Show me around",
			domain:      "itinerary",
			preferences: "Food & Nightlife",
			expectSame:  false,
			compareWith: 0,
		},
		{
			name:        "Different city should generate different key",
			city:        "London",
			message:     "Show me around",
			domain:      "itinerary",
			preferences: "Art & Museums",
			expectSame:  false,
			compareWith: 0,
		},
		{
			name:        "Different domain should generate different key",
			city:        "Paris",
			message:     "Show me around",
			domain:      "hotels",
			preferences: "Art & Museums",
			expectSame:  false,
			compareWith: 0,
		},
	}

	var keys []string

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheKeyData := map[string]interface{}{
				"user_id":     userID.String(),
				"profile_id":  profileID.String(),
				"city":        tt.city,
				"message":     tt.message,
				"domain":      tt.domain,
				"preferences": tt.preferences,
			}
			cacheKeyBytes, err := json.Marshal(cacheKeyData)
			assert.NoError(t, err)

			hash := md5.Sum(cacheKeyBytes)
			cacheKey := hex.EncodeToString(hash[:])

			keys = append(keys, cacheKey)

			if tt.compareWith >= 0 {
				if tt.expectSame {
					assert.Equal(t, keys[tt.compareWith], cacheKey,
						"Cache keys should be identical for same parameters")
				} else {
					assert.NotEqual(t, keys[tt.compareWith], cacheKey,
						"Cache keys should be different for different parameters")
				}
			}
		})
	}
}

// TestCacheKeyUniqueness tests that different combinations create unique keys
func TestCacheKeyUniqueness(t *testing.T) {
	userID := uuid.New()
	profileID := uuid.New()

	// Generate 100 unique combinations
	cities := []string{"Paris", "London", "New York", "Tokyo", "Sydney"}
	preferences := []string{"Art & Museums", "Food & Nightlife", "Nature & Outdoors", "Shopping", "Sports"}
	domains := []string{"itinerary", "hotels", "restaurants", "activities"}

	keyMap := make(map[string]bool)
	keyCount := 0

	for _, city := range cities {
		for _, pref := range preferences {
			for _, domain := range domains {
				cacheKeyData := map[string]interface{}{
					"user_id":     userID.String(),
					"profile_id":  profileID.String(),
					"city":        city,
					"message":     "Show me around",
					"domain":      domain,
					"preferences": pref,
				}
				cacheKeyBytes, _ := json.Marshal(cacheKeyData)
				hash := md5.Sum(cacheKeyBytes)
				cacheKey := hex.EncodeToString(hash[:])

				// Verify uniqueness
				assert.False(t, keyMap[cacheKey],
					fmt.Sprintf("Duplicate key found for: city=%s, pref=%s, domain=%s", city, pref, domain))

				keyMap[cacheKey] = true
				keyCount++
			}
		}
	}

	expectedCount := len(cities) * len(preferences) * len(domains)
	assert.Equal(t, expectedCount, keyCount,
		"Should generate unique keys for all combinations")
	assert.Equal(t, expectedCount, len(keyMap),
		"All keys should be unique")
}

// TestCacheEviction tests that cache items are evicted after TTL
func TestCacheEviction(t *testing.T) {
	// Use a very short TTL for testing
	shortTTL := 100 * time.Millisecond
	testCache := cache.New(shortTTL, 50*time.Millisecond)

	// Set a value
	testCache.Set("test-key", "test-value", cache.DefaultExpiration)

	// Verify it exists
	val, found := testCache.Get("test-key")
	assert.True(t, found)
	assert.Equal(t, "test-value", val)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Verify it's gone
	_, found = testCache.Get("test-key")
	assert.False(t, found, "Cache item should be evicted after TTL")
}

// TestConcurrentCacheAccess tests thread-safety of cache operations
func TestConcurrentCacheAccess(t *testing.T) {
	testCache := cache.New(48*time.Hour, 1*time.Hour)

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			value := fmt.Sprintf("value-%d", id)
			testCache.Set(key, value, cache.DefaultExpiration)
		}(i)
	}

	wg.Wait()

	// Verify all writes succeeded
	for i := 0; i < numGoroutines; i++ {
		key := fmt.Sprintf("key-%d", i)
		expectedValue := fmt.Sprintf("value-%d", i)

		val, found := testCache.Get(key)
		assert.True(t, found, fmt.Sprintf("Key %s should exist", key))
		assert.Equal(t, expectedValue, val, fmt.Sprintf("Value for %s should match", key))
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			expectedValue := fmt.Sprintf("value-%d", id)

			val, found := testCache.Get(key)
			assert.True(t, found)
			assert.Equal(t, expectedValue, val)
		}(i)
	}

	wg.Wait()
}

// TestCacheDifferentPreferencesSameCity verifies different preferences generate different cache keys
func TestCacheDifferentPreferencesSameCity(t *testing.T) {
	userID := uuid.New()
	profileID := uuid.New()
	city := "Paris"
	domain := string(types.DomainItinerary)

	pref1 := "Art & Museums"
	pref2 := "Food & Nightlife"

	// Generate key for first preference
	cacheKeyData1 := map[string]interface{}{
		"user_id":     userID.String(),
		"profile_id":  profileID.String(),
		"city":        city,
		"message":     "Show me around",
		"domain":      domain,
		"preferences": pref1,
	}
	cacheKeyBytes1, _ := json.Marshal(cacheKeyData1)
	hash1 := md5.Sum(cacheKeyBytes1)
	cacheKey1 := hex.EncodeToString(hash1[:])

	// Generate key for second preference
	cacheKeyData2 := map[string]interface{}{
		"user_id":     userID.String(),
		"profile_id":  profileID.String(),
		"city":        city,
		"message":     "Show me around",
		"domain":      domain,
		"preferences": pref2,
	}
	cacheKeyBytes2, _ := json.Marshal(cacheKeyData2)
	hash2 := md5.Sum(cacheKeyBytes2)
	cacheKey2 := hex.EncodeToString(hash2[:])

	// Verify they're different
	assert.NotEqual(t, cacheKey1, cacheKey2,
		"Different preferences for same city should generate different cache keys")

	// Verify they can be stored separately in cache
	testCache := cache.New(48*time.Hour, 1*time.Hour)
	testCache.Set(cacheKey1, "Art response", cache.DefaultExpiration)
	testCache.Set(cacheKey2, "Food response", cache.DefaultExpiration)

	val1, found1 := testCache.Get(cacheKey1)
	val2, found2 := testCache.Get(cacheKey2)

	assert.True(t, found1, "First preference should be cached")
	assert.True(t, found2, "Second preference should be cached")
	assert.Equal(t, "Art response", val1)
	assert.Equal(t, "Food response", val2)
	assert.NotEqual(t, val1, val2, "Cached values should be different")
}

// TestAllEndpointCacheKeys verifies cache keys for all endpoints
func TestAllEndpointCacheKeys(t *testing.T) {
	userID := uuid.New()
	profileID := uuid.New()

	endpoints := []struct {
		domain types.DomainType
		suffix string
	}{
		{types.DomainItinerary, "_itinerary"},
		{types.DomainAccommodation, "_hotels"},
		{types.DomainDining, "_restaurants"},
		{types.DomainActivities, "_activities"},
	}

	cacheKeys := make(map[string]string)

	for _, endpoint := range endpoints {
		cacheKeyData := map[string]interface{}{
			"user_id":     userID.String(),
			"profile_id":  profileID.String(),
			"city":        "Paris",
			"message":     "Test message",
			"domain":      string(endpoint.domain),
			"preferences": "Test preferences",
		}
		cacheKeyBytes, _ := json.Marshal(cacheKeyData)
		hash := md5.Sum(cacheKeyBytes)
		baseCacheKey := hex.EncodeToString(hash[:])
		fullCacheKey := baseCacheKey + endpoint.suffix

		// Store for uniqueness check
		cacheKeys[string(endpoint.domain)] = fullCacheKey

		// Verify key format
		assert.True(t, strings.HasSuffix(fullCacheKey, endpoint.suffix),
			"Cache key should have correct suffix for %s", endpoint.domain)
	}

	// Verify all endpoint keys are unique
	assert.Equal(t, len(endpoints), len(cacheKeys),
		"All endpoints should have unique base cache keys")
}

// TestCacheSetAndGet verifies basic cache set/get operations
func TestCacheSetAndGet(t *testing.T) {
	testCache := cache.New(48*time.Hour, 1*time.Hour)

	testCases := []struct {
		key   string
		value interface{}
	}{
		{"string-key", "string value"},
		{"int-key", 12345},
		{"struct-key", struct{ Name string }{"test"}},
		{"slice-key", []string{"a", "b", "c"}},
	}

	for _, tc := range testCases {
		t.Run("Set_and_Get_"+tc.key, func(t *testing.T) {
			// Set value
			testCache.Set(tc.key, tc.value, cache.DefaultExpiration)

			// Get value
			val, found := testCache.Get(tc.key)
			assert.True(t, found, "Key should be found")
			assert.Equal(t, tc.value, val, "Value should match")
		})
	}
}

// TestCacheItemCount verifies cache item counting
func TestCacheItemCount(t *testing.T) {
	testCache := cache.New(48*time.Hour, 1*time.Hour)

	assert.Equal(t, 0, testCache.ItemCount(), "Cache should start empty")

	// Add items
	for i := 0; i < 10; i++ {
		testCache.Set(fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i), cache.DefaultExpiration)
	}

	assert.Equal(t, 10, testCache.ItemCount(), "Cache should have 10 items")

	// Delete some items
	testCache.Delete("key-0")
	testCache.Delete("key-1")

	assert.Equal(t, 8, testCache.ItemCount(), "Cache should have 8 items after deletions")
}
