-- +migrate Up
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE EXTENSION IF NOT EXISTS postgis_topology;

CREATE EXTENSION IF NOT EXISTS fuzzystrmatch;

CREATE EXTENSION IF NOT EXISTS postgis_tiger_geocoder;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE EXTENSION IF NOT EXISTS citext;

CREATE EXTENSION IF NOT EXISTS vector;

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE OR REPLACE FUNCTION set_updated_at()
    RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'subscription_plan_type') THEN
        CREATE TYPE subscription_plan_type AS ENUM (
            'free',
            'premium_monthly',
            'premium_annual'
        );
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'subscription_status') THEN
        CREATE TYPE subscription_status AS ENUM (
            'active',       -- Currently paid or free plan active
            'trialing',     -- In a trial period
            'past_due',     -- Payment failed
            'canceled',     -- Canceled by user, might still be active until end_date
            'expired'       -- Subscription period ended and not renewed
        );
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'poi_source') THEN
        CREATE TYPE poi_source AS ENUM (
            'loci_ai', -- Added by our system/AI
            'openstreetmap', -- Imported from OSM
            'user_submitted',-- Submitted by a user (maybe requires verification)
            'partner'        -- From a paying partner/featured listing
        );
    END IF;
END
$$;
