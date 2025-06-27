-- Fix session_id column type in llm_interactions table to match UUID type
ALTER TABLE llm_interactions 
ALTER COLUMN session_id TYPE UUID USING session_id::UUID;