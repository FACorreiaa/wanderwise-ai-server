-- Revert the foreign key constraint changes for user_saved_itineraries
ALTER TABLE user_saved_itineraries DROP CONSTRAINT IF EXISTS user_saved_itineraries_source_llm_interaction_id_fkey;
ALTER TABLE user_saved_itineraries ALTER COLUMN source_llm_interaction_id SET NOT NULL;
ALTER TABLE user_saved_itineraries ADD CONSTRAINT user_saved_itineraries_source_llm_interaction_id_fkey 
    FOREIGN KEY (source_llm_interaction_id) REFERENCES llm_interactions(id) ON DELETE CASCADE;