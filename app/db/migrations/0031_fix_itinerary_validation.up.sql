-- +migrate Up

-- Update the validation trigger to handle both lists and user_saved_itineraries
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