-- Fix the foreign key constraint to allow NULL llm_interaction_id
-- This allows POIs to be saved without a specific LLM interaction context

-- First, drop the existing foreign key constraint
ALTER TABLE llm_poi DROP CONSTRAINT IF EXISTS fk_llm_poi_interaction;

-- Make llm_interaction_id nullable
ALTER TABLE llm_poi ALTER COLUMN llm_interaction_id DROP NOT NULL;

-- Add the foreign key constraint back, but allow NULL values
ALTER TABLE llm_poi ADD CONSTRAINT fk_llm_poi_interaction 
    FOREIGN KEY (llm_interaction_id) REFERENCES llm_interactions(id) ON DELETE SET NULL;