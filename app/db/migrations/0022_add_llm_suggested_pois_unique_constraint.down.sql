-- Remove unique constraint from llm_suggested_pois
ALTER TABLE llm_suggested_pois 
DROP CONSTRAINT IF EXISTS unique_llm_suggested_poi_name_location;