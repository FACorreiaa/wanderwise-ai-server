-- Drop indexes
DROP INDEX IF EXISTS idx_discover_searches_user_created;
DROP INDEX IF EXISTS idx_discover_searches_query_city;
DROP INDEX IF EXISTS idx_discover_searches_trending;

-- Drop table
DROP TABLE IF EXISTS discover_searches;
