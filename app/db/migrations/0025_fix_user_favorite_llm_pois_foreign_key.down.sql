-- Revert the foreign key constraint back to llm_suggested_pois
ALTER TABLE user_favorite_llm_pois DROP CONSTRAINT IF EXISTS user_favorite_llm_pois_llm_poi_id_fkey;
ALTER TABLE user_favorite_llm_pois ADD CONSTRAINT user_favorite_llm_pois_llm_poi_id_fkey 
    FOREIGN KEY (llm_poi_id) REFERENCES llm_suggested_pois(id) ON DELETE CASCADE;