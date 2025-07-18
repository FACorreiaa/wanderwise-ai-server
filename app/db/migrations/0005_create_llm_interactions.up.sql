-- +migrate Up
-- Table to log interactions with the LLM (Gemini) for debugging, analysis, history
CREATE TABLE llm_interactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4 (),
    user_id UUID REFERENCES users (id) ON DELETE SET NULL, -- Link to user if applicable
    session_id TEXT, -- Optional: Group related interactions
    city_name TEXT,
    city_id UUID REFERENCES cities (id) ON DELETE SET NULL,
    prompt TEXT NOT NULL, -- The final prompt sent
    request_payload JSONB, -- Full request body sent to Gemini API (optional)
    response TEXT, -- The final generated text response
    response_payload JSONB, -- Full response body from Gemini API (incl. function calls, safety ratings)
    model_name TEXT, -- e.g., 'gemini-1.5-pro'
    prompt_tokens INTEGER,
    completion_tokens INTEGER,
    total_tokens INTEGER,
    latitude DOUBLE PRECISION, -- LLM provided latitude
    longitude DOUBLE PRECISION, -- LLM provided longitude
    distance DOUBLE PRECISION, -- Distance from the user's current location (if applicable)
    latency_ms INTEGER, -- Time taken for the API call
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for querying logs
CREATE INDEX idx_llm_interactions_user_id ON llm_interactions (user_id);

CREATE INDEX idx_llm_interactions_created_at ON llm_interactions (created_at);
-- Consider JSONB indexes if querying payload frequently:
-- CREATE INDEX idx_llm_interactions_resp_payload_gin ON llm_interactions USING GIN (response_payload);

CREATE TABLE llm_suggested_pois (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL, -- The user for whom this was generated
    search_profile_id UUID, -- The specific search profile used (if applicable)
    llm_interaction_id UUID NOT NULL REFERENCES llm_interactions(id) ON DELETE CASCADE, -- Links to the LLM request/response log
    city_id UUID REFERENCES cities(id) ON DELETE SET NULL, -- The city context for this POI
    city_name TEXT,
    latitude DOUBLE PRECISION, -- LLM provided latitude
    longitude DOUBLE PRECISION, -- LLM provided longitude
    distance DOUBLE PRECISION, -- Distance from the user's current location (if applicable)
    phone_number TEXT,
    opening_hours JSONB,
    rating DOUBLE PRECISION,
    price_level TEXT,
    location GEOMETRY(Point, 4326) NOT NULL, -- PostGIS geometry type for spatial queries
    name TEXT NOT NULL,
    description TEXT, -- LLM-generated description
    category TEXT, -- LLM-suggested category
    address TEXT, -- If LLM provides it
    website TEXT, -- If LLM provides it
    opening_hours_suggestion TEXT, -- If LLM provides it
    description_poi TEXT, -- LLM-generated description of the POI
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    --FOREIGN KEY (search_profile_id) REFERENCES user_preference_profiles(id) ON DELETE SET NULL,
    FOREIGN KEY (llm_interaction_id) REFERENCES llm_interactions(id) ON DELETE CASCADE,
    FOREIGN KEY (city_id) REFERENCES cities(id) ON DELETE CASCADE,
    -- You can add other fields from types.POIDetail if the LLM commonly provides them

-- Foreign key constraints (if not defined inline above)
-- CONSTRAINT fk_llm_suggested_pois_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE, -- Assuming you have a users table
-- CONSTRAINT fk_llm_suggested_pois_profile FOREIGN KEY (search_profile_id) REFERENCES user_search_profiles(id) ON DELETE SET NULL,

created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_llm_suggested_pois_location ON llm_suggested_pois USING GIST (location);

CREATE INDEX idx_llm_suggested_pois_interaction_id ON llm_suggested_pois (llm_interaction_id);
-- Crucial for distance sorting

-- Trigger to update 'updated_at' timestamp
CREATE TRIGGER trigger_set_llm_suggested_pois_updated_at
BEFORE UPDATE ON llm_suggested_pois
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

--

CREATE TABLE poi_details (
    id UUID PRIMARY KEY,
    city_id UUID REFERENCES cities (id),
    name TEXT NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    location GEOMETRY (POINT, 4326) NOT NULL,
    description TEXT,
    address TEXT,
    website TEXT,
    phone_number TEXT,
    opening_hours TEXT,
    price_range TEXT,
    category TEXT,
    tags TEXT [],
    images TEXT [],
    rating DOUBLE PRECISION,
    llm_interaction_id UUID REFERENCES llm_interactions (id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_updated TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (city_id) REFERENCES cities (id) ON DELETE CASCADE,
    FOREIGN KEY (llm_interaction_id) REFERENCES llm_interactions (id) ON DELETE SET NULL
);

-- Index for spatial queries
CREATE INDEX idx_poi_details_location ON poi_details USING GIST (location);

CREATE INDEX idx_poi_details_city_name ON poi_details (city_id, name);

CREATE TABLE hotel_details (
    id UUID PRIMARY KEY,
    city_id UUID REFERENCES cities (id),
    name TEXT NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    location GEOMETRY (POINT, 4326) NOT NULL,
    category TEXT,
    description TEXT,
    address TEXT,
    website TEXT,
    phone_number TEXT,
    opening_hours JSONB,
    price_range TEXT,
    tags TEXT [],
    images TEXT [],
    rating DOUBLE PRECISION,
    llm_interaction_id UUID REFERENCES llm_interactions (id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_updated TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (city_id) REFERENCES cities (id) ON DELETE CASCADE,
    FOREIGN KEY (llm_interaction_id) REFERENCES llm_interactions (id) ON DELETE SET NULL
);

CREATE INDEX idx_hotel_details_location ON hotel_details USING GIST (location);

CREATE INDEX idx_hotel_details_city_name ON hotel_details (city_id, name);

CREATE TABLE restaurant_details (
    id UUID PRIMARY KEY,
    city_id UUID REFERENCES cities (id),
    name TEXT NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    location GEOMETRY (POINT, 4326) NOT NULL,
    category TEXT,
    description TEXT,
    address TEXT,
    website TEXT,
    phone_number TEXT,
    opening_hours JSONB,
    price_level TEXT,
    cuisine_type TEXT,
    tags TEXT [],
    images TEXT [],
    rating DOUBLE PRECISION,
    llm_interaction_id UUID REFERENCES llm_interactions (id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_updated TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (city_id) REFERENCES cities (id) ON DELETE CASCADE,
    FOREIGN KEY (llm_interaction_id) REFERENCES llm_interactions (id) ON DELETE SET NULL
);

CREATE INDEX idx_restaurant_details_location ON restaurant_details USING GIST (location);

CREATE INDEX idx_restaurant_details_city_name ON restaurant_details (city_id, name);

CREATE TABLE chat_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id),
    current_itinerary JSONB,
    conversation_history JSONB,
    session_context JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(20) NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE INDEX idx_chat_sessions_user_id ON chat_sessions (user_id);