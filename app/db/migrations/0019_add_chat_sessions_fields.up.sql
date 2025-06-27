-- Add missing fields to chat_sessions table
ALTER TABLE chat_sessions 
ADD COLUMN profile_id UUID,
ADD COLUMN city_name VARCHAR(255);

-- Add foreign key constraint for profile_id if user_profiles table exists
-- (assuming it references user_profiles table based on the ProfileID field usage)
-- ALTER TABLE chat_sessions 
-- ADD CONSTRAINT fk_chat_sessions_profile_id 
-- FOREIGN KEY (profile_id) REFERENCES user_profiles (id) ON DELETE SET NULL;

-- Add indexes for better query performance
CREATE INDEX idx_chat_sessions_profile_id ON chat_sessions (profile_id);
CREATE INDEX idx_chat_sessions_city_name ON chat_sessions (city_name);