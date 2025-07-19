-- +migrate Down

-- Revert poi_id column to NOT NULL (this may fail if there are NULL values)
ALTER TABLE list_items ALTER COLUMN poi_id SET NOT NULL;