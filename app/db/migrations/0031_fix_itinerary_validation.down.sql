-- +migrate Down

-- Revert the validation trigger to only check lists table
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
        IF NOT EXISTS (SELECT 1 FROM lists WHERE id = NEW.item_id AND is_itinerary = true) THEN
            RAISE EXCEPTION 'Referenced itinerary with id % does not exist', NEW.item_id;
        END IF;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;