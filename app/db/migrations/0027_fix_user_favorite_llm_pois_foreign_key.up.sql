-- Fix foreign key constraint in user_favorite_llm_pois table
-- The constraint currently references llm_suggested_pois but should reference llm_poi

BEGIN;



COMMIT;