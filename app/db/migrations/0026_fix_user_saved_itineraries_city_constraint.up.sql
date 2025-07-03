-- Fix the foreign key constraint in user_saved_itineraries to allow NULL primary_city_id
-- This allows itineraries to be saved without a specific city context

-- First, drop the existing foreign key constraint
ALTER TABLE user_saved_itineraries DROP CONSTRAINT IF EXISTS user_saved_itineraries_primary_city_id_fkey;

-- Make primary_city_id nullable
ALTER TABLE user_saved_itineraries ALTER COLUMN primary_city_id DROP NOT NULL;

-- Add the foreign key constraint back, but allow NULL values
ALTER TABLE user_saved_itineraries ADD CONSTRAINT user_saved_itineraries_primary_city_id_fkey 
    FOREIGN KEY (primary_city_id) REFERENCES cities(id) ON DELETE SET NULL;