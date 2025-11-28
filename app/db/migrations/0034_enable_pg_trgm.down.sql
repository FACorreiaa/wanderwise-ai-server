-- Drop the GIN index on city names
DROP INDEX IF EXISTS idx_cities_name_trgm;

-- Drop pg_trgm extension (only if not used elsewhere)
-- Note: This is commented out by default to avoid breaking other functionality
-- DROP EXTENSION IF EXISTS pg_trgm;
