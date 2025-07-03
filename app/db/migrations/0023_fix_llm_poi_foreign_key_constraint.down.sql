-- Revert the foreign key constraint changes
ALTER TABLE llm_poi DROP CONSTRAINT IF EXISTS fk_llm_poi_interaction;
ALTER TABLE llm_poi ALTER COLUMN llm_interaction_id SET NOT NULL;
ALTER TABLE llm_poi ADD CONSTRAINT fk_llm_poi_interaction 
    FOREIGN KEY (llm_interaction_id) REFERENCES llm_interactions(id) ON DELETE CASCADE;