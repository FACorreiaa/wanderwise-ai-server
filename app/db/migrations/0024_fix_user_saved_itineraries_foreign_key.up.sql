-- Fix the foreign key constraint in user_saved_itineraries to allow NULL source_llm_interaction_id
-- This allows itineraries to be saved without a specific LLM interaction context

-- First, drop the existing foreign key constraint
ALTER TABLE user_saved_itineraries DROP CONSTRAINT IF EXISTS user_saved_itineraries_source_llm_interaction_id_fkey;

-- Make source_llm_interaction_id nullable
ALTER TABLE user_saved_itineraries ALTER COLUMN source_llm_interaction_id DROP NOT NULL;

-- Add the foreign key constraint back, but allow NULL values
ALTER TABLE user_saved_itineraries ADD CONSTRAINT user_saved_itineraries_source_llm_interaction_id_fkey 
    FOREIGN KEY (source_llm_interaction_id) REFERENCES llm_interactions(id) ON DELETE SET NULL;