-- Fix the foreign key constraint in user_favorite_llm_pois to point to llm_poi table
-- The current constraint points to llm_suggested_pois but we're using llm_poi

-- First, drop the existing foreign key constraint
ALTER TABLE user_favorite_llm_pois DROP CONSTRAINT IF EXISTS user_favorite_llm_pois_llm_poi_id_fkey;

-- Add the correct foreign key constraint pointing to llm_poi table
ALTER TABLE user_favorite_llm_pois ADD CONSTRAINT user_favorite_llm_pois_llm_poi_id_fkey 
    FOREIGN KEY (llm_poi_id) REFERENCES llm_poi(id) ON DELETE CASCADE;