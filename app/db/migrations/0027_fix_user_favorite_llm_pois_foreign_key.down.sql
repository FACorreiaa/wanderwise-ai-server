-- Rollback fix for user_favorite_llm_pois foreign key constraint

BEGIN;

-- -- Drop the corrected foreign key constraint
-- ALTER TABLE user_favorite_llm_pois
-- DROP CONSTRAINT IF EXISTS user_favorite_llm_pois_llm_poi_id_fkey;
--
-- -- Restore the original (incorrect) foreign key constraint
-- ALTER TABLE user_favorite_llm_pois
-- ADD CONSTRAINT user_favorite_llm_pois_llm_poi_id_fkey
-- FOREIGN KEY (llm_poi_id) REFERENCES llm_suggested_pois (id) ON DELETE CASCADE;

COMMIT;