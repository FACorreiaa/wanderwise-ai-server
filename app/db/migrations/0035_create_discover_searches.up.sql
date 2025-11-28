-- Create table to track discover searches for trending analysis
CREATE TABLE IF NOT EXISTS discover_searches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    query TEXT NOT NULL,
    city_name TEXT NOT NULL,
    search_type TEXT DEFAULT 'discover', -- 'discover', 'poi', 'semantic'
    result_count INT DEFAULT 0,
    source TEXT DEFAULT 'database', -- 'database', 'llm'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for trending queries (most searched today)
-- Composite index for the trending query - covers the GROUP BY and ORDER BY
CREATE INDEX IF NOT EXISTS idx_discover_searches_trending ON discover_searches(created_at DESC, query, city_name);

-- Additional indexes for other access patterns
CREATE INDEX IF NOT EXISTS idx_discover_searches_query_city ON discover_searches(query, city_name);
CREATE INDEX IF NOT EXISTS idx_discover_searches_user_created ON discover_searches(user_id, created_at DESC);
