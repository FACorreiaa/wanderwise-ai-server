-- Add unique constraint for ON CONFLICT clause in llm_suggested_pois
-- This prevents duplicate POIs with same name and location
ALTER TABLE llm_suggested_pois 
ADD CONSTRAINT unique_llm_suggested_poi_name_location 
UNIQUE (name, latitude, longitude);