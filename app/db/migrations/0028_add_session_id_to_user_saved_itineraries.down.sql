-- Rollback session_id addition to user_saved_itineraries table

BEGIN;

-- Drop the indexes first
DROP INDEX IF EXISTS idx_user_saved_itineraries_session_id;
DROP INDEX IF EXISTS idx_user_saved_itineraries_user_session;

-- Drop the session_id column
ALTER TABLE user_saved_itineraries 
DROP COLUMN IF EXISTS session_id;

COMMIT;