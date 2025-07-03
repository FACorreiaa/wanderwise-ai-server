-- Create llm_poi table for storing LLM-generated POI data
CREATE TABLE llm_poi (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    llm_interaction_id UUID NOT NULL,
    city TEXT NOT NULL,
    name TEXT NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    category TEXT,
    description TEXT,
    address TEXT,
    website TEXT,
    phone_number TEXT,
    opening_hours JSONB,
    price_level TEXT,
    tags JSONB,
    images JSONB,
    rating DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add indexes for performance
CREATE INDEX idx_llm_poi_name_city ON llm_poi(LOWER(name), LOWER(city));
CREATE INDEX idx_llm_poi_location ON llm_poi(latitude, longitude);
CREATE INDEX idx_llm_poi_category ON llm_poi(category);
CREATE INDEX idx_llm_poi_interaction_id ON llm_poi(llm_interaction_id);

-- Add unique constraint to prevent duplicate POIs
ALTER TABLE llm_poi ADD CONSTRAINT unique_llm_poi_name_city UNIQUE (name, city);

-- Add foreign key constraint to llm_interactions (optional, but good practice)
ALTER TABLE llm_poi ADD CONSTRAINT fk_llm_poi_interaction 
    FOREIGN KEY (llm_interaction_id) REFERENCES llm_interactions(id) ON DELETE CASCADE;