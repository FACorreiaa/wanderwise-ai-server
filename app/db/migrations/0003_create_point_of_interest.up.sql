-- +migrate Up
CREATE TABLE cities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4 (),
    name TEXT,
    state_province TEXT, -- e.g., California, Bavaria
    country TEXT NOT NULL, -- e.g., USA, Germany
    -- Unique constraint on name/state/country to avoid duplicates
    CONSTRAINT unique_city_location UNIQUE (name, state_province, country),
    center_location GEOMETRY (Point, 4326), -- Optional: Center point for map display
    bounding_box GEOMETRY (Polygon, 4326), -- Optional: Bounding box for spatial queries
    ai_summary TEXT, -- AI-generated summary of the city
    embedding VECTOR (768), -- Optional: Embedding vector for the city (adjust dimension)
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for faster city lookups
CREATE INDEX idx_cities_name_country ON cities (LOWER(name), country);

CREATE INDEX idx_cities_center_location ON cities USING GIST (center_location);

-- Trigger to update 'updated_at' timestamp
CREATE TRIGGER trigger_set_cities_updated_at
BEFORE UPDATE ON cities
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE points_of_interest (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4 (),
    name TEXT NOT NULL,
    description TEXT,
    location GEOMETRY (Point, 4326) NOT NULL, -- Uses PostGIS geometry type (SRID 4326 for WGS84)
    city_id UUID REFERENCES cities (id) ON DELETE SET NULL, -- Link POI to a city (optional, but recommended)
    address TEXT, -- Formatted address string
    poi_type TEXT, -- Simple text type or FK to a poi_types table
    website TEXT,
    phone_number TEXT,
    opening_hours JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)
    category TEXT, -- e.g., "restaurant", "museum", "park"
    price_level INTEGER CHECK (
        price_level >= 1
        AND price_level <= 4
    ), -- e.g., 1 (cheap) to 4 (expensive)
    average_rating NUMERIC(3, 2), -- For Phase 2 reviews (e.g., 4.50)
    rating_count INTEGER DEFAULT 0, -- For Phase 2 reviews
    source poi_source NOT NULL DEFAULT 'loci_ai', -- Where did this data come from?
    source_id TEXT, -- Original ID from the source system (e.g., OSM Node ID)
    is_verified BOOLEAN NOT NULL DEFAULT FALSE, -- Has this been manually verified?
    is_sponsored BOOLEAN NOT NULL DEFAULT FALSE, -- Is this a paid placement?
    ai_summary TEXT, -- AI-generated summary specific to the POI
    embedding VECTOR (768), -- Uses pgvector type (adjust dimension as needed)
    tags TEXT [], -- Array of simple text tags (e.g., {"historic", "good for kids", "romantic"}) - Alternatively use a join table
    accessibility_info TEXT, -- Description of accessibility features
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (city_id) REFERENCES cities (id) ON DELETE CASCADE
);

-- Core spatial index for location-based queries (finding nearby POIs)
CREATE INDEX idx_poi_location ON points_of_interest USING GIST (location);
-- Index for filtering by city
CREATE INDEX idx_points_of_interest_city_name ON points_of_interest (city_id, name);

CREATE INDEX idx_poi_city_id ON points_of_interest (city_id);
-- Index for filtering by type
CREATE INDEX idx_poi_type ON points_of_interest (poi_type);
-- Index for text search on name/description (consider more advanced FTS later)
CREATE INDEX idx_poi_name ON points_of_interest USING GIN (to_tsvector('english', name));

CREATE INDEX idx_poi_description ON points_of_interest USING GIN (
    to_tsvector('english', description)
);
-- Index for embeddings (Choose one - HNSW is often faster for high-dimensional data)
-- Needs pgvector installed. Create AFTER inserting some data if using IVFFlat.
-- CREATE INDEX idx_poi_embedding_hnsw ON points_of_interest USING HNSW (embedding vector_cosine_ops); -- Example using Cosine distance

-- Trigger to update 'updated_at' timestamp
CREATE TRIGGER trigger_set_poi_updated_at
BEFORE UPDATE ON points_of_interest
FOR EACH ROW EXECUTE FUNCTION set_updated_at();