-- +migrate Up

-- For POIs from the main 'points_of_interest' table
CREATE TABLE user_favorite_pois (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4 (), -- Optional, (user_id, poi_id) can be PK
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    poi_id UUID NOT NULL REFERENCES points_of_interest (id) ON DELETE CASCADE,
    notes TEXT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (poi_id) REFERENCES points_of_interest (id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT unique_user_poi UNIQUE (user_id, poi_id)
);

-- Indexes for user_favorite_pois
CREATE INDEX idx_user_favorite_pois_user_id ON user_favorite_pois (user_id);

CREATE INDEX idx_user_favorite_pois_poi_id ON user_favorite_pois (poi_id);

CREATE TABLE user_favorite_llm_pois (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4 (), -- Optional
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    llm_poi_id UUID NOT NULL REFERENCES llm_suggested_pois (id) ON DELETE CASCADE,
    notes TEXT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_user_favorite_llm_poi UNIQUE (user_id, llm_poi_id)
);

-- Indexes for user_favorite_llm_pois
--CREATE INDEX idx_user_favorite_llm_pois_user_id ON user_favorite_llm_pois (user_id);

--CREATE INDEX idx_user_favorite_llm_pois_llm_poi_id ON user_favorite_llm_pois (llm_poi_id);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;