-- Fix foreign key constraint in user_favorite_llm_pois table
-- The constraint currently references llm_suggested_pois but should reference llm_poi

BEGIN;

-- Drop the existing foreign key constraint
ALTER TABLE user_favorite_llm_pois 
DROP CONSTRAINT IF EXISTS user_favorite_llm_pois_llm_poi_id_fkey;

-- Add the correct foreign key constraint to reference llm_poi table
ALTER TABLE user_favorite_llm_pois 
ADD CONSTRAINT user_favorite_llm_pois_llm_poi_id_fkey 
FOREIGN KEY (llm_poi_id) REFERENCES llm_poi (id) ON DELETE CASCADE;

COMMIT;