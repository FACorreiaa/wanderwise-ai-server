# Kafka Integration Opportunities in the Loci API

This API currently serves REST + SSE endpoints for auth, user profiles, interests/tags, chat/LLM interactions, POIs, itineraries, recents, discover, and statistics. Everything is synchronous on Postgres today. Below are concrete places where Kafka can add value in this context.

## High-Value Event Streams
- **Discover/search telemetry** (`discover.TrackSearch`, POI search endpoints): publish `search.executed` with user/session, query, city, source (chat/manual), and result count to power trending cities, top searches, and real-time analytics without hammering `chat_sessions`.
- **LLM chat lifecycle** (`StartChatMessageStream`, `ContinueChatSessionStream`, `SaveItenerary`): emit `chat.session.started`, `chat.response.generated`, `chat.itinerary.saved`, including latency/cost metadata. Consumers can do quality scoring, cost monitoring, safety checks, and finetuning data capture asynchronously.
- **Recommendations & personalization signals** (`AddPoiToFavourites`, bookmarks, recents): send `poi.favorited`, `poi.unfavorited`, `itinerary.saved`, `itinerary.updated` to feed a streaming feature store (e.g., Redis/Feast) and improve next-turn suggestions without blocking the request.
- **POI enrichment/embeddings** (`GenerateEmbeddingForPOI`, `GenerateEmbeddingsForAllPOIs`): push `poi.embedding.requested` and let workers consume/compute embeddings off the main request path; publish `poi.embedding.completed` to invalidate caches and backfill search indexes.
- **City/POI ingestion and dedup** (city/POI repositories): use topics like `poi.ingested` and `poi.dedup.requested` to process third-party feeds, run geocoding/normalization, and write into Postgres in batches with retry/backpressure.
- **Statistics dashboards** (`statistics` + `recents`): stream raw interaction events (searches, chat completions, favorites) into Kafka and have a dedicated consumer aggregate into materialized tables instead of doing heavy COUNT/GROUP BY on hot paths.

## Example Topic Plan (producers → consumers)
- `search.executed` (chat + POI services → analytics aggregator, trending calculator)
- `chat.session.started` / `chat.response.generated` (LLM handler → quality/cost monitor, safety filter, finetuning sink)
- `chat.itinerary.saved` (LLM handler → itinerary recommender, notification/email worker)
- `poi.favorited` / `poi.unfavorited` (POI handler → personalization feature store, marketing triggers)
- `poi.embedding.requested` / `poi.embedding.completed` (POI service → embedding workers → cache/index updater)
- `poi.ingested` / `poi.dedup.requested` (ingestion jobs → dedup/enrichment workers)

## Integration Pattern
- **Outbox + Kafka**: add an outbox table alongside existing transactions (Postgres) in the handlers/services above; a relay publishes to Kafka to avoid dual-write issues.
- **Background workers**: embedding generation, ingestion, and aggregation consumers run outside the API process; keep the API path fast and idempotent.
- **Contract-first events**: define Avro/Protobuf schemas for the topics above with stable keys (`user_id`, `session_id`, `city_name`), include trace IDs for cross-service observability.

## Quick Wins to Pilot
- Start with `search.executed` and `chat.response.generated` to feed trending stats and LLM cost/latency dashboards.
- Move `GenerateEmbeddingsForAllPOIs` to emit `poi.embedding.requested` batches and let workers process them asynchronously.
- Emit `poi.favorited` / `chat.itinerary.saved` to bootstrap personalization signals for recents/discover pages.
