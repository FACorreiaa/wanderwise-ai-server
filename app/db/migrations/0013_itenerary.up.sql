-- +migrate Up

-- Table to store user itineraries
CREATE TABLE itineraries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4 (),
    user_id UUID REFERENCES users (id) ON DELETE CASCADE,
    city_id UUID REFERENCES cities (id) ON DELETE CASCADE,
    source_llm_interaction_id UUID REFERENCES llm_interactions (id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Table to link POIs to itineraries with AI-generated descriptions
CREATE TABLE itinerary_pois (
    itinerary_id UUID REFERENCES itineraries (id) ON DELETE CASCADE,
    poi_id UUID REFERENCES points_of_interest (id) ON DELETE CASCADE,
    order_index INTEGER NOT NULL, -- To maintain the sequence of POIs in the itinerary
    ai_description TEXT, -- AI-generated description specific to this POI in this itinerary
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (itinerary_id, poi_id)
);

CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_timestamp
BEFORE UPDATE ON itinerary_pois
FOR EACH ROW
EXECUTE FUNCTION update_timestamp();