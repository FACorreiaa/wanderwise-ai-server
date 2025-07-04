# Chat Streaming Workflow Documentation - Working Main Branch

This document provides a comprehensive analysis of the complete workflow for the working streaming endpoint: `http://localhost:8000/api/v1/llm/prompt-response/chat/sessions/stream/{sessionID}`

## Overview

The endpoint handles continuing chat sessions with real-time streaming responses, semantic POI enhancement, and multi-worker concurrent processing for generating travel recommendations.

## 1. HTTP Request Routing

### Router Configuration
**File:** `/internal/router/router.go`

```go
// Line 201: Route definition
r.Post("/prompt-response/chat/sessions/{sessionID}/continue", HandlerImpl.ContinueChatSessionStream)
```

**Route Protection:**
- Located under `/api/v1/llm` mount point (Line 103)
- Protected by JWT authentication middleware (`cfg.AuthenticateMiddleware`)
- Applied to all routes under the authenticated group (Lines 81-83)

**URL Pattern:** `POST /api/v1/llm/prompt-response/chat/sessions/{sessionID}/continue`

## 2. Handler Layer

### Handler Method
**File:** `/internal/api/chat_prompt/chat_handler.go`
**Method:** `ContinueChatSessionStream` (Lines 571-658)

#### Handler Workflow:

1. **Parameter Extraction**
   ```go
   sessionIDStr := chi.URLParam(r, "sessionID")
   sessionID, err := uuid.Parse(sessionIDStr)
   ```

2. **Request Body Parsing**
   ```go
   var req struct {
       Message      string                `json:"message"`
       CityName     string                `json:"city_name,omitempty"`
       ContextType  types.ChatContextType `json:"context_type,omitempty"`
       UserLocation *types.UserLocation   `json:"user_location,omitempty"`
   }
   ```

3. **SSE Headers Setup**
   ```go
   w.Header().Set("Content-Type", "text/event-stream")
   w.Header().Set("Cache-Control", "no-cache")
   w.Header().Set("Connection", "keep-alive")
   w.Header().Set("Access-Control-Allow-Origin", "*")
   ```

4. **Event Channel Creation**
   ```go
   eventCh := make(chan types.StreamEvent, 100)
   ```

5. **Service Invocation (Goroutine)**
   ```go
   go func() {
       err := h.llmInteractionService.ContinueSessionStreamed(
           ctx, sessionID, req.Message, req.UserLocation, eventCh
       )
   }()
   ```

6. **Real-time Event Streaming**
   ```go
   for {
       select {
       case event, ok := <-eventCh:
           eventData, _ := json.Marshal(event)
           fmt.Fprintf(w, "data: %s\n\n", eventData)
           flusher.Flush() // Immediate client delivery
       case <-r.Context().Done():
           return // Client disconnect
       }
   }
   ```

## 3. Service Layer

### Main Service Method
**File:** `/internal/api/chat_prompt/chat_service.go`
**Method:** `ContinueSessionStreamed` (Line 1358)

#### Service Workflow Phases:

### Phase 1: Session Validation & Context Loading

1. **Session Retrieval**
   ```go
   session, err := l.llmInteractionRepo.GetSession(ctx, sessionID)
   ```
   - **Repository Call:** `chat_repository.go:GetSession()`
   - **SQL Query:** 
     ```sql
     SELECT id, user_id, profile_id, city_name, current_itinerary, 
            conversation_history, session_context, created_at, updated_at, 
            expires_at, status
     FROM chat_sessions WHERE id = $1
     ```

2. **Session Status Validation**
   ```go
   if session.Status != "active" {
       return fmt.Errorf("session is not active")
   }
   ```

3. **City Resolution & Fallback**
   ```go
   city, err := l.cityRepo.FindCityByNameAndCountry(ctx, cityName, "")
   ```
   - **Repository Call:** `city_repository.go:FindCityByNameAndCountry()`
   - **Fuzzy Matching:** If exact match fails, tries partial matching
   - **Fallback Creation:** Creates basic city entry if none exists

4. **Progress Event Emission**
   ```go
   l.sendEvent(ctx, eventCh, types.StreamEvent{
       Type: types.EventTypeProgress,
       Message: "Session validated and context loaded"
   }, 3)
   ```

### Phase 2: Message Processing & Intent Classification

1. **User Message Addition**
   ```go
   session.ConversationHistory = append(session.ConversationHistory, types.ChatMessage{
       Role:      "user",
       Content:   message,
       Timestamp: time.Now(),
   })
   ```

2. **Intent Classification**
   ```go
   intentResult := l.intentClassifier.Classify(ctx, message)
   ```
   - **Purpose:** Determines user intent (add_poi, remove_poi, ask_question, etc.)
   - **Implementation:** Pattern matching and ML-based classification

3. **Semantic POI Enhancement**
   ```go
   semanticPOIs, err := l.generateSemanticPOIRecommendations(ctx, message, city.ID)
   ```
   - **Embedding Generation:** Converts message to vector embedding
   - **Similarity Search:** Finds relevant POIs using vector similarity
   - **Threshold:** Default 0.6 similarity threshold for relevance

### Phase 3: Intent-Based Processing

Based on classified intent, different handlers are invoked:

#### Add POI Intent
```go
case types.IntentAddPOI:
    return l.handleSemanticAddPOIStreamed(ctx, session, message, semanticPOIs, eventCh)
```

#### Remove POI Intent
```go
case types.IntentRemovePOI:
    return l.handleSemanticRemovePOI(ctx, session, message, eventCh)
```

#### Question Intent
```go
case types.IntentAskQuestion:
    // Generic response handler
```

#### Replace POI Pattern
```go
if strings.Contains(strings.ToLower(message), "replace") {
    // Pattern-based POI replacement logic
}
```

## 4. Worker Orchestration

### Multi-Worker Concurrent Processing
**File:** `/internal/api/chat_prompt/chat_workers.go`

The system launches multiple concurrent workers for different data generation tasks:

#### Worker Launch Pattern
```go
var wg sync.WaitGroup
asyncCtx := context.WithValue(ctx, "trace_id", ctx.Value("trace_id"))

// City Data Worker
wg.Add(1)
go func() {
    defer wg.Done()
    l.streamingCityDataWorker(asyncCtx, city.Name, sendEventWrapper)
}()

// General POI Worker  
wg.Add(1)
go func() {
    defer wg.Done()
    l.streamingGeneralPOIWorker(asyncCtx, city.Name, sendEventWrapper)
}()

// Personalized POI Worker
wg.Add(1)
go func() {
    defer wg.Done()
    l.streamingPersonalizedPOIWorkerWithSemantics(asyncCtx, city.Name, searchProfile, semanticPOIs, sendEventWrapper)
}()
```

### Individual Worker Details

#### 1. City Data Worker (`streamingCityDataWorker`)

**Purpose:** Generates city information and metadata
**AI Prompt:** `getCityDataPrompt(cityName)`
**Streaming Process:**
```go
iter, err := l.aiClient.GenerateContentStreamWithCache(ctx, prompt, config, cacheKey)
for resp, err := range iter {
    // Process streaming chunks
    sendEvent(types.StreamEvent{
        Type: types.EventTypeChunk,
        Data: map[string]interface{}{
            "part": "city_data",
            "chunk": chunk,
            "cache_used": cacheKey != "",
        },
    })
}
```

**Database Operations:**
- **Interaction Saving:** `l.saveCityInteraction(ctx, interaction)`
- **City Data Parsing:** `parseCityDataFromResponse(fullResponse.String())`
- **City Persistence:** `l.HandleCityData(asyncCtx, cityData)`

#### 2. General POI Worker (`streamingGeneralPOIWorker`)

**Purpose:** Generates general points of interest for the city
**AI Prompt:** `getGeneralPOIPrompt(cityName)`
**Streaming Implementation:**
```go
iter, err := l.aiClient.GenerateContentStream(ctx, prompt, config)
if err != nil {
    // Fallback to non-streaming generation
    response, err := l.aiClient.GenerateContent(ctx, prompt, config)
}
```

**Database Operations:**
- **Interaction Logging:** `l.llmInteractionRepo.SaveInteraction(ctx, interaction)`
- **POI Extraction:** `parsePOIsFromResponse(fullResponse.String())`
- **POI Persistence:** Bulk saving of extracted POIs

#### 3. Personalized POI Worker (`streamingPersonalizedPOIWorkerWithSemantics`)

**Purpose:** Generates personalized POIs enhanced with semantic context
**Input Enhancement:**
```go
prompt := getPersonalizedItineraryPrompt(cityName, basePreferences)
if len(semanticPOIs) > 0 {
    prompt += "\n\nContext POIs for reference:\n" + formatSemanticPOIsForPrompt(semanticPOIs)
}
```

**User Context Integration:**
- **Preferences:** User interests, dietary needs, budget level
- **Location:** Current user location for distance calculations  
- **Semantic Context:** Relevant POIs from vector similarity search

## 5. Repository Layer Operations

### Chat Repository
**File:** `/internal/api/chat_prompt/chat_repository.go`

#### Session Management
```sql
-- GetSession
SELECT id, user_id, profile_id, city_name, current_itinerary, 
       conversation_history, session_context, created_at, updated_at, 
       expires_at, status
FROM chat_sessions WHERE id = $1

-- AddMessageToSession  
UPDATE chat_sessions 
SET conversation_history = $2, updated_at = $3
WHERE id = $1
```

#### Interaction Persistence
```go
func (r *Repository) SaveInteraction(ctx context.Context, interaction types.LlmInteraction) (uuid.UUID, error) {
    tx, err := r.db.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    // Insert main interaction
    var interactionID uuid.UUID
    err = tx.QueryRowContext(ctx, `
        INSERT INTO llm_interactions (...) 
        VALUES (...) 
        RETURNING id
    `, ...).Scan(&interactionID)
    
    // Insert itinerary if present
    if interaction.ItineraryData != nil {
        // Complex itinerary and POI persistence logic
    }
    
    return tx.Commit()
}
```

### City Repository  
**File:** `/internal/api/city/city_repository.go`

#### City Lookup with Fallback
```sql
-- Primary lookup
SELECT id, name, country, state_province, ai_summary, center_latitude, center_longitude
FROM cities 
WHERE LOWER(name) = LOWER($1) AND ($2 = '' OR country = $2)

-- Fuzzy fallback
SELECT id, name, country, state_province, ai_summary, center_latitude, center_longitude  
FROM cities
WHERE LOWER(name) LIKE LOWER($1) || '%'
LIMIT 1
```

#### City Creation
```sql
INSERT INTO cities (name, country, state_province, ai_summary, center_latitude, center_longitude)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id
```

### POI Repository Operations
**File:** `/internal/api/poi/poi_repository.go`

#### Semantic POI Search
```sql
SELECT poi.id, poi.name, poi.category, poi.description_poi, poi.latitude, poi.longitude,
       poi.embedding <-> $1 AS distance
FROM llm_suggested_pois poi
WHERE poi.city_id = $2 
  AND poi.embedding <-> $1 < $3
ORDER BY distance
LIMIT $4
```

#### Bulk POI Insertion
```go
func (r *Repository) SavePOIsBatch(ctx context.Context, pois []types.POIDetailedInfo) error {
    stmt, err := r.db.PrepareContext(ctx, `
        INSERT INTO llm_suggested_pois (name, category, description_poi, latitude, longitude, city_id, ...)
        VALUES ($1, $2, $3, $4, $5, $6, ...)
    `)
    
    for _, poi := range pois {
        _, err = stmt.ExecContext(ctx, poi.Name, poi.Category, ...)
    }
}
```

## 6. Database Schema

### Core Tables

#### `chat_sessions`
```sql
CREATE TABLE chat_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    profile_id UUID REFERENCES search_profiles(id),
    city_name VARCHAR(255),
    current_itinerary JSONB,
    conversation_history JSONB DEFAULT '[]'::jsonb,
    session_context JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    status VARCHAR(20) DEFAULT 'active'
);
```

#### `llm_interactions`
```sql
CREATE TABLE llm_interactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    session_id UUID REFERENCES chat_sessions(id),
    prompt TEXT NOT NULL,
    response_text TEXT,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    city_name VARCHAR(255),
    model_used VARCHAR(100),
    input_tokens INTEGER,
    output_tokens INTEGER,
    interaction_type VARCHAR(50)
);
```

#### `llm_suggested_pois`
```sql
CREATE TABLE llm_suggested_pois (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    category VARCHAR(100),
    description_poi TEXT,
    latitude DECIMAL(10, 8),
    longitude DECIMAL(11, 8),
    location GEOMETRY(POINT, 4326),
    city_id UUID REFERENCES cities(id),
    llm_interaction_id UUID REFERENCES llm_interactions(id),
    user_id UUID REFERENCES users(id),
    embedding vector(768), -- Vector embeddings for semantic search
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## 7. AI Integration & Streaming

### Generative AI Client Configuration
```go
client := genai.NewClient(apiKey)
model := client.GenerativeModel("gemini-2.0-flash")
config := &genai.GenerateContentConfig{
    Temperature: genai.Ptr[float32](0.5),
}
```

### Streaming Implementation
```go
iter, err := l.aiClient.GenerateContentStreamWithCache(ctx, prompt, config, cacheKey)
for resp, err := range iter {
    for _, cand := range resp.Candidates {
        for _, part := range cand.Content.Parts {
            chunk := string(part.Text)
            fullResponse.WriteString(chunk)
            
            // Immediate streaming to client
            sendEvent(types.StreamEvent{
                Type: types.EventTypeChunk,
                Data: map[string]interface{}{
                    "part":       partType,
                    "chunk":      chunk,
                    "cache_used": cacheKey != "",
                },
            })
        }
    }
}
```

### Caching Strategy
- **Cache Key Generation:** Based on prompt content hash
- **Cache Duration:** Configurable TTL for prompt responses
- **Cache Hit Detection:** `cache_used` flag in streaming events

## 8. Semantic Enhancement

### Vector Embedding Generation
```go
func (s *ServiceImpl) generateSemanticPOIRecommendations(ctx context.Context, message string, cityID uuid.UUID) ([]types.POIDetailedInfo, error) {
    // Generate embedding for user message
    embedding, err := s.embeddingService.GenerateEmbedding(ctx, message)
    
    // Perform similarity search
    semanticPOIs, err := s.poiRepo.SearchPOIsBySimilarity(ctx, embedding, cityID, 0.6, 10)
    
    return semanticPOIs, nil
}
```

### Prompt Enhancement
```go
prompt := getPersonalizedItineraryPrompt(cityName, basePreferences)
if len(semanticPOIs) > 0 {
    prompt += "\n\nContext POIs for reference:\n"
    for _, poi := range semanticPOIs {
        prompt += fmt.Sprintf("- %s (%s): %s\n", poi.Name, poi.Category, poi.DescriptionPOI)
    }
}
```

## 9. Event Streaming Architecture

### Event Types
```go
const (
    EventTypeStart    = "start"
    EventTypeProgress = "progress"  
    EventTypeChunk    = "chunk"
    EventTypeData     = "data"
    EventTypeComplete = "complete"
    EventTypeError    = "error"
)
```

### Event Structure
```go
type StreamEvent struct {
    Type       string                 `json:"type"`
    Message    string                 `json:"message"`
    Data       map[string]interface{} `json:"data,omitempty"`
    Error      string                 `json:"error,omitempty"`
    Timestamp  time.Time              `json:"timestamp"`
    EventID    string                 `json:"event_id"`
    IsFinal    bool                   `json:"is_final,omitempty"`
    Navigation *NavigationData        `json:"navigation,omitempty"`
}
```

### Streaming Flow
1. **Start Event:** Session validation completed
2. **Progress Events:** Worker initialization, context loading
3. **Chunk Events:** Real-time AI generation output
4. **Data Events:** Structured results (city data, POIs)
5. **Complete Event:** Final navigation and session update

## 10. Error Handling & Observability

### OpenTelemetry Tracing
```go
ctx, span := otel.Tracer("chat_service").Start(ctx, "ContinueSessionStreamed")
defer span.End()

span.SetAttributes(
    attribute.String("session.id", sessionID.String()),
    attribute.String("user.message", message),
    attribute.String("city.name", cityName),
)
```

### Error Recovery Patterns
```go
// Graceful worker failure handling
defer func() {
    if r := recover(); r != nil {
        l.logger.ErrorContext(ctx, "Worker panic recovered", slog.Any("panic", r))
        sendEvent(types.StreamEvent{
            Type:  types.EventTypeError,
            Error: "Worker encountered an error",
        })
    }
}()
```

### Database Transaction Management
```go
tx, err := r.db.BeginTx(ctx, nil)
defer func() {
    if err != nil {
        tx.Rollback()
    }
}()

// Perform operations...

if err = tx.Commit(); err != nil {
    return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
}
```

## 11. Performance Optimizations

### Concurrent Worker Processing
- **Parallel Execution:** City data, general POIs, and personalized POIs generated concurrently
- **Resource Isolation:** Each worker operates in separate goroutine with isolated context
- **Synchronization:** `sync.WaitGroup` ensures all workers complete before session finalization

### Caching Strategy
- **Prompt-Response Caching:** Reduces redundant AI API calls
- **Session Context Caching:** Maintains conversation state efficiently
- **POI Embedding Caching:** Pre-computed vectors for fast similarity search

### Database Optimizations
- **Bulk Operations:** Batch POI insertions for better performance
- **Connection Pooling:** Efficient database connection management
- **Prepared Statements:** Reduced query parsing overhead

This comprehensive workflow ensures a robust, scalable streaming chat system with real-time AI generation, semantic enhancement, and reliable data persistence.