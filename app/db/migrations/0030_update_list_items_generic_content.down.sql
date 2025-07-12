-- +migrate Down

-- Drop the validation trigger and function
DROP TRIGGER IF EXISTS trigger_validate_list_item_content_type ON list_items;
DROP FUNCTION IF EXISTS validate_list_item_content_type();

-- Drop the compatibility view
DROP VIEW IF EXISTS list_items_poi_compat;

-- Drop new indexes
DROP INDEX IF EXISTS idx_list_items_item_id;
DROP INDEX IF EXISTS idx_list_items_content_type;
DROP INDEX IF EXISTS idx_list_items_source_llm_interaction_id;
DROP INDEX IF EXISTS idx_list_items_content_type_item_id;

-- Restore the original primary key
ALTER TABLE list_items DROP CONSTRAINT list_items_pkey;

-- Remove the new columns
ALTER TABLE list_items 
DROP COLUMN IF EXISTS item_id,
DROP COLUMN IF EXISTS content_type,
DROP COLUMN IF EXISTS source_llm_interaction_id,
DROP COLUMN IF EXISTS item_ai_description;

-- Restore original primary key
ALTER TABLE list_items ADD CONSTRAINT list_items_pkey PRIMARY KEY (list_id, poi_id);

-- Restore original item count trigger function
CREATE OR REPLACE FUNCTION update_list_item_count() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        -- Update list item count when an item is deleted
        UPDATE lists
        SET item_count = (
            SELECT COUNT(*)
            FROM list_items
            WHERE list_id = OLD.list_id
        )
        WHERE id = OLD.list_id;
        RETURN OLD;
    ELSE
        -- Update list item count when an item is inserted
        UPDATE lists
        SET item_count = (
            SELECT COUNT(*)
            FROM list_items
            WHERE list_id = NEW.list_id
        )
        WHERE id = NEW.list_id;
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;