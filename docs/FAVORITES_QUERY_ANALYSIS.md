# Database Schema Analysis: User Favorites Query

## Problem Analysis

The query you provided is not returning results because it has incorrect table joins:

```sql
-- ❌ INCORRECT QUERY
SELECT
    p.id, p.name, ST_X(p.location) AS longitude, ST_Y(p.location) AS latitude,
    p.poi_type AS category, p.ai_summary AS description_poi
FROM points_of_interest p
INNER JOIN user_favorite_llm_pois uf ON p.id = uf.llm_poi_id;
```

**Issue**: You're joining `points_of_interest` with `user_favorite_llm_pois` using `llm_poi_id`, but `llm_poi_id` references the `llm_poi` table, not `points_of_interest`.

## Database Schema Structure

### Core Tables

#### 1. **points_of_interest** (Regular POIs)
```sql
CREATE TABLE points_of_interest (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    location GEOMETRY(Point, 4326),
    city_id UUID REFERENCES cities(id),
    address TEXT,
    category VARCHAR(100),
    poi_type VARCHAR(100),
    website VARCHAR(255),
    phone_number VARCHAR(20),
    opening_hours JSONB,
    average_rating DECIMAL(3,2),
    price_level INTEGER,
    -- ... other fields
);
```

#### 2. **llm_poi** (LLM-Generated POIs)
```sql
CREATE TABLE llm_poi (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    city VARCHAR(255),
    latitude DECIMAL(10, 8) NOT NULL,
    longitude DECIMAL(11, 8) NOT NULL,
    category VARCHAR(255),
    description TEXT,
    address TEXT,
    website VARCHAR(500),
    phone_number VARCHAR(20),
    opening_hours JSONB,
    rating DECIMAL(3,2),
    price_level VARCHAR(50),
    llm_interaction_id UUID REFERENCES llm_interactions(id)
);
```

### Favorites Tables

#### 1. **user_favorite_pois** (For Regular POIs)
```sql
CREATE TABLE user_favorite_pois (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    poi_id UUID NOT NULL REFERENCES points_of_interest(id) ON DELETE CASCADE,
    notes TEXT,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_user_poi UNIQUE (user_id, poi_id)
);
```

#### 2. **user_favorite_llm_pois** (For LLM POIs)
```sql
CREATE TABLE user_favorite_llm_pois (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    llm_poi_id UUID NOT NULL REFERENCES llm_poi(id) ON DELETE CASCADE,
    notes TEXT,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_user_favorite_llm_poi UNIQUE (user_id, llm_poi_id)
);
```

## Foreign Key Relationships

- **user_favorite_pois**:
  - `user_id` → `users(id)`
  - `poi_id` → `points_of_interest(id)`

- **user_favorite_llm_pois**:
  - `user_id` → `users(id)`
  - `llm_poi_id` → `llm_poi(id)` ⚠️ **NOT points_of_interest**

## ✅ CORRECT QUERIES

### 1. Get Regular POI Favorites
```sql
SELECT
    ufp.id as favorite_id,
    ufp.notes,
    ufp.added_at,
    poi.id,
    poi.name,
    ST_X(poi.location) AS longitude,
    ST_Y(poi.location) AS latitude,
    poi.poi_type AS category,
    poi.description AS description_poi,
    poi.address,
    poi.website,
    poi.phone_number,
    poi.opening_hours,
    poi.average_rating as rating,
    poi.price_level,
    'regular' as poi_source
FROM user_favorite_pois ufp
INNER JOIN points_of_interest poi ON ufp.poi_id = poi.id
WHERE ufp.user_id = $1
ORDER BY ufp.added_at DESC;
```

### 2. Get LLM POI Favorites
```sql
SELECT
    uflp.id as favorite_id,
    uflp.notes,
    uflp.added_at,
    llm_poi.id,
    llm_poi.name,
    llm_poi.longitude,
    llm_poi.latitude,
    llm_poi.category,
    llm_poi.description AS description_poi,
    llm_poi.address,
    llm_poi.website,
    llm_poi.phone_number,
    llm_poi.opening_hours,
    llm_poi.rating,
    llm_poi.price_level,
    'llm' as poi_source
FROM user_favorite_llm_pois uflp
INNER JOIN llm_poi ON uflp.llm_poi_id = llm_poi.id
WHERE uflp.user_id = $1
ORDER BY uflp.added_at DESC;
```

### 3. Get ALL User Favorites (Combined)
```sql
SELECT 
    favorite_id,
    notes,
    added_at,
    id,
    name,
    longitude,
    latitude,
    category,
    description_poi,
    address,
    website,
    phone_number,
    opening_hours,
    rating,
    price_level,
    poi_source
FROM (
    -- Regular POI favorites
    SELECT
        ufp.id as favorite_id,
        ufp.notes,
        ufp.added_at,
        poi.id,
        poi.name,
        ST_X(poi.location) AS longitude,
        ST_Y(poi.location) AS latitude,
        poi.poi_type AS category,
        poi.description AS description_poi,
        poi.address,
        poi.website,
        poi.phone_number,
        poi.opening_hours,
        poi.average_rating as rating,
        poi.price_level::text as price_level,
        'regular' as poi_source
    FROM user_favorite_pois ufp
    INNER JOIN points_of_interest poi ON ufp.poi_id = poi.id
    WHERE ufp.user_id = $1
    
    UNION ALL
    
    -- LLM POI favorites
    SELECT
        uflp.id as favorite_id,
        uflp.notes,
        uflp.added_at,
        llm_poi.id,
        llm_poi.name,
        llm_poi.longitude,
        llm_poi.latitude,
        llm_poi.category,
        llm_poi.description AS description_poi,
        llm_poi.address,
        llm_poi.website,
        llm_poi.phone_number,
        llm_poi.opening_hours,
        llm_poi.rating,
        llm_poi.price_level,
        'llm' as poi_source
    FROM user_favorite_llm_pois uflp
    INNER JOIN llm_poi ON uflp.llm_poi_id = llm_poi.id
    WHERE uflp.user_id = $1
) combined_favorites
ORDER BY added_at DESC;
```

### 4. Paginated Combined Favorites Query
```sql
WITH combined_favorites AS (
    -- Regular POI favorites
    SELECT
        ufp.id as favorite_id,
        ufp.notes,
        ufp.added_at,
        poi.id,
        poi.name,
        ST_X(poi.location) AS longitude,
        ST_Y(poi.location) AS latitude,
        poi.poi_type AS category,
        poi.description AS description_poi,
        poi.address,
        poi.website,
        poi.phone_number,
        poi.opening_hours,
        poi.average_rating as rating,
        poi.price_level::text as price_level,
        'regular' as poi_source
    FROM user_favorite_pois ufp
    INNER JOIN points_of_interest poi ON ufp.poi_id = poi.id
    WHERE ufp.user_id = $1
    
    UNION ALL
    
    -- LLM POI favorites
    SELECT
        uflp.id as favorite_id,
        uflp.notes,
        uflp.added_at,
        llm_poi.id,
        llm_poi.name,
        llm_poi.longitude,
        llm_poi.latitude,
        llm_poi.category,
        llm_poi.description AS description_poi,
        llm_poi.address,
        llm_poi.website,
        llm_poi.phone_number,
        llm_poi.opening_hours,
        llm_poi.rating,
        llm_poi.price_level,
        'llm' as poi_source
    FROM user_favorite_llm_pois uflp
    INNER JOIN llm_poi ON uflp.llm_poi_id = llm_poi.id
    WHERE uflp.user_id = $1
)
SELECT *
FROM combined_favorites
ORDER BY added_at DESC
LIMIT $2 OFFSET $3;

-- Count query for pagination
SELECT COUNT(*) FROM (
    SELECT ufp.id
    FROM user_favorite_pois ufp
    WHERE ufp.user_id = $1
    
    UNION ALL
    
    SELECT uflp.id
    FROM user_favorite_llm_pois uflp
    WHERE uflp.user_id = $1
) total_favorites;
```

## Key Points

1. **Separate Tables**: Regular POIs and LLM POIs are stored in different tables
2. **Separate Favorites**: Each POI type has its own favorites table
3. **Different Foreign Keys**: 
   - `user_favorite_pois.poi_id` → `points_of_interest.id`
   - `user_favorite_llm_pois.llm_poi_id` → `llm_poi.id`
4. **Union Required**: To get all favorites, you need a UNION query
5. **Field Differences**: Different field names/types between tables (e.g., `poi_type` vs `category`)

## Why Your Original Query Failed

```sql
-- ❌ This fails because:
FROM points_of_interest p
INNER JOIN user_favorite_llm_pois uf ON p.id = uf.llm_poi_id;
```

- `user_favorite_llm_pois.llm_poi_id` references `llm_poi.id`, not `points_of_interest.id`
- The foreign key constraint prevents this join from working
- You need to join with `llm_poi` table instead

## Migration History

- Migration `0015`: Created both favorites tables
- Migration `0021`: Created `llm_poi` table
- Migration `0027`: Fixed foreign key to reference `llm_poi` instead of `llm_suggested_pois`