-- +migrate Up

-- Add unique constraint on (user_id, city_id) for itineraries table
-- This allows ON CONFLICT (user_id, city_id) to work in the code
ALTER TABLE itineraries ADD CONSTRAINT itineraries_user_city_unique UNIQUE (user_id, city_id);