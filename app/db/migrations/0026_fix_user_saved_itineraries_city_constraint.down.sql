-- Revert the foreign key constraint changes for user_saved_itineraries
ALTER TABLE user_saved_itineraries DROP CONSTRAINT IF EXISTS user_saved_itineraries_primary_city_id_fkey;
ALTER TABLE user_saved_itineraries ALTER COLUMN primary_city_id SET NOT NULL;
ALTER TABLE user_saved_itineraries ADD CONSTRAINT user_saved_itineraries_primary_city_id_fkey 
    FOREIGN KEY (primary_city_id) REFERENCES cities(id) ON DELETE CASCADE;