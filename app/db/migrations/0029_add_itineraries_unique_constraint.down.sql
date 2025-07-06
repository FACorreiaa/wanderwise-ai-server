-- +migrate Down

-- Remove the unique constraint
ALTER TABLE itineraries DROP CONSTRAINT IF EXISTS itineraries_user_city_unique;