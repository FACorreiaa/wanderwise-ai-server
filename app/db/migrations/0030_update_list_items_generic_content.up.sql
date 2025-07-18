-- +migrate Up

-- Update list_items table to support generic content types
-- First, add new columns for generic content support
ALTER TABLE list_items
ADD COLUMN item_id UUID,
ADD COLUMN content_type VARCHAR(20) CHECK (content_type IN ('poi', 'restaurant', 'hotel', 'itinerary')),
ADD COLUMN source_llm_interaction_id UUID REFERENCES llm_interactions(id) ON DELETE SET NULL,
ADD COLUMN item_ai_description TEXT;

-- Migrate existing data: copy poi_id to item_id and set content_type to 'poi'
UPDATE list_items 
SET item_id = poi_id, 
    content_type = 'poi'
WHERE poi_id IS NOT NULL;

-- Make the new columns NOT NULL after data migration
ALTER TABLE list_items 
ALTER COLUMN item_id SET NOT NULL,
ALTER COLUMN content_type SET NOT NULL;

-- Create indexes for better performance
CREATE INDEX idx_list_items_item_id ON list_items (item_id);
CREATE INDEX idx_list_items_content_type ON list_items (content_type);
CREATE INDEX idx_list_items_source_llm_interaction_id ON list_items (source_llm_interaction_id);

-- Create composite index for efficient content type queries
CREATE INDEX idx_list_items_content_type_item_id ON list_items (content_type, item_id);

-- Add foreign key constraints for different content types
-- Note: We'll need to handle this in application logic since PostgreSQL doesn't support 
-- conditional foreign keys. The constraints are documented here for reference:
-- 
-- When content_type = 'poi': item_id should reference points_of_interest(id)
-- When content_type = 'restaurant': item_id should reference restaurants(id) (if exists)
-- When content_type = 'hotel': item_id should reference hotels(id) (if exists)  
-- When content_type = 'itinerary': item_id should reference lists(id) where is_itinerary = true

-- Update the primary key to use the new generic structure
-- First drop the old primary key constraint
ALTER TABLE list_items DROP CONSTRAINT list_items_pkey;

-- Create new primary key using list_id, content_type, and item_id
ALTER TABLE list_items ADD CONSTRAINT list_items_pkey PRIMARY KEY (list_id, content_type, item_id);

-- Update the item count trigger function to work with the new structure
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

-- Add validation trigger to ensure content type integrity
CREATE OR REPLACE FUNCTION validate_list_item_content_type() RETURNS TRIGGER AS $$
BEGIN
    -- Validate that the item_id exists for the specified content_type
    IF NEW.content_type = 'poi' THEN
        IF NOT EXISTS (SELECT 1 FROM points_of_interest WHERE id = NEW.item_id) THEN
            RAISE EXCEPTION 'Referenced POI with id % does not exist', NEW.item_id;
        END IF;
    ELSIF NEW.content_type = 'restaurant' THEN
        -- Restaurants are stored as POIs with category 'restaurant' or similar
        IF NOT EXISTS (SELECT 1 FROM points_of_interest WHERE id = NEW.item_id) THEN
            RAISE EXCEPTION 'Referenced restaurant with id % does not exist', NEW.item_id;
        END IF;
    ELSIF NEW.content_type = 'hotel' THEN
        -- Hotels are stored as POIs with category 'hotel' or similar
        IF NOT EXISTS (SELECT 1 FROM points_of_interest WHERE id = NEW.item_id) THEN
            RAISE EXCEPTION 'Referenced hotel with id % does not exist', NEW.item_id;
        END IF;
    ELSIF NEW.content_type = 'itinerary' THEN
        -- Check if it's a user-created itinerary in lists table
        IF NOT EXISTS (SELECT 1 FROM lists WHERE id = NEW.item_id AND is_itinerary = true) THEN
            -- If not found in lists, check if it's a bookmarked itinerary in user_saved_itineraries
            IF NOT EXISTS (SELECT 1 FROM user_saved_itineraries WHERE id = NEW.item_id) THEN
                RAISE EXCEPTION 'Referenced itinerary with id % does not exist', NEW.item_id;
            END IF;
        END IF;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_validate_list_item_content_type
    BEFORE INSERT OR UPDATE ON list_items
    FOR EACH ROW EXECUTE FUNCTION validate_list_item_content_type();

-- Add a view for backward compatibility with the old poi_id column
CREATE VIEW list_items_poi_compat AS
SELECT 
    list_id,
    CASE WHEN content_type = 'poi' THEN item_id ELSE NULL END AS poi_id,
    item_id,
    content_type,
    position,
    notes,
    day_number,
    time_slot,
    duration,
    source_llm_interaction_id,
    item_ai_description,
    created_at,
    updated_at
FROM list_items;

COMMENT ON TABLE list_items IS 'Stores items in lists with support for multiple content types: POIs, restaurants, hotels, and nested itineraries';
COMMENT ON COLUMN list_items.item_id IS 'Generic reference to any content type (POI, restaurant, hotel, or itinerary)';
COMMENT ON COLUMN list_items.content_type IS 'Type of content: poi, restaurant, hotel, or itinerary';
COMMENT ON COLUMN list_items.source_llm_interaction_id IS 'Reference to the LLM interaction that generated this item (if AI-generated)';
COMMENT ON COLUMN list_items.item_ai_description IS 'AI-generated description or explanation for why this item was added to the list';