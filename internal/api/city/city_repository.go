package city

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var _ Repository = (*RepositoryImpl)(nil)

type Repository interface {
	SaveCity(ctx context.Context, city types.CityDetail) (uuid.UUID, error)
	FindCityByNameAndCountry(ctx context.Context, city, country string) (*types.CityDetail, error)
	FindCityByFuzzyName(ctx context.Context, cityName string) (*types.CityDetail, error)
	GetCityIDByName(ctx context.Context, cityName string) (uuid.UUID, error)
	GetAllCities(ctx context.Context) ([]types.CityDetail, error)

	// Vector similarity search methods
	FindSimilarCities(ctx context.Context, queryEmbedding []float32, limit int) ([]types.CityDetail, error)
	UpdateCityEmbedding(ctx context.Context, cityID uuid.UUID, embedding []float32) error
	GetCitiesWithoutEmbeddings(ctx context.Context, limit int) ([]types.CityDetail, error)

	GetCity(ctx context.Context, lat, lon float64) (uuid.UUID, string, error)
}

type RepositoryImpl struct {
	logger *slog.Logger
	pgpool *pgxpool.Pool
}

func NewCityRepository(pgxpool *pgxpool.Pool, logger *slog.Logger) *RepositoryImpl {
	return &RepositoryImpl{
		logger: logger,
		pgpool: pgxpool,
	}
}

func (r *RepositoryImpl) SaveCity(ctx context.Context, city types.CityDetail) (uuid.UUID, error) {
	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Construct center_location geometry
	// var centerLocationArg interface{}
	// if city.CenterLatitude != 0 && city.CenterLongitude != 0 {
	// 	if city.CenterLatitude < -90 || city.CenterLatitude > 90 || city.CenterLongitude < -180 || city.CenterLongitude > 180 {
	// 		r.logger.WarnContext(ctx, "Invalid coordinates for city center, saving as NULL",
	// 			slog.String("city", city.Name),
	// 			slog.Float64("lat", city.CenterLatitude),
	// 			slog.Float64("lon", city.CenterLongitude))
	// 		centerLocationArg = nil
	// 	} else {
	// 		centerLocationArg = fmt.Sprintf("ST_SetSRID(ST_MakePoint(%f, %f), 4326)", city.CenterLongitude, city.CenterLatitude)
	// 	}
	// } else {
	// 	centerLocationArg = nil // Save as NULL if no coordinates provided
	// }

	// BoundingBox: For now, we'll insert NULL for bounding_box as AI doesn't provide it easily.
	// You would populate this later if you have a geocoding service or other data source.
	// query := `
	//     INSERT INTO cities (
	//         name, country, state_province, ai_summary, center_location, bounding_box
	//     ) VALUES ($1, $2, $3, $4, ST_GeomFromText($5, 4326), ST_GeomFromText($6, 4326)) RETURNING id
	// `
	// Note: ST_GeomFromText is for WKT strings. For ST_MakePoint, you don't need ST_GeomFromText wrapper if it's directly in SQL.
	// However, pgx typically requires you to pass geometry types in a specific way or as WKT.
	// A common way with pgx for dynamic geometry is to build the SQL string part.
	// Let's adjust to pass coordinates and build the point in SQL.

	query := `
        INSERT INTO cities (
            name, country, state_province, ai_summary, center_location 
            -- bounding_box will use its DEFAULT or be NULL if not specified
        ) VALUES (
            $1, $2, $3, $4, 
            -- Check for 0.0 is a bit naive if 0,0 is a valid location.
            -- It's better if types.CityDetail.CenterLongitude/Latitude are pointers (*float64)
            -- Then you can check for nil. For now, assuming 0.0 implies "not set".
            CASE 
                WHEN ($5::DOUBLE PRECISION IS NOT NULL AND $6::DOUBLE PRECISION IS NOT NULL) 
                     AND ($5::DOUBLE PRECISION != 0.0 OR $6::DOUBLE PRECISION != 0.0) -- Example: only make point if not (0,0) AND both are provided
                     AND ($5::DOUBLE PRECISION >= -180 AND $5::DOUBLE PRECISION <= 180) -- Longitude check
                     AND ($6::DOUBLE PRECISION >= -90 AND $6::DOUBLE PRECISION <= 90)   -- Latitude check
                THEN ST_SetSRID(ST_MakePoint($5::DOUBLE PRECISION, $6::DOUBLE PRECISION), 4326) 
                ELSE NULL 
            END
        ) RETURNING id
    `
	var id uuid.UUID

	// For ST_MakePoint, longitude is first, then latitude
	err = tx.QueryRow(ctx, query,
		city.Name,
		city.Country,
		NewNullString(city.StateProvince),
		city.AiSummary,
		NewNullFloat64(city.CenterLongitude),
		NewNullFloat64(city.CenterLatitude),
	).Scan(&id)

	if err != nil {
		// No need to check for pgx.ErrNoRows on INSERT RETURNING, an error means failure.
		return uuid.Nil, fmt.Errorf("failed to insert city: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return id, nil
}

// Helper function to convert empty strings to sql.NullString for database insertion
func NewNullString(s string) sql.NullString {
	if len(s) == 0 {
		return sql.NullString{} // Valid = false, String is empty
	}
	return sql.NullString{
		String: s,
		Valid:  true,
	}
}

// Helper function to convert 0.0 float to sql.NullFloat64 for database insertion
func NewNullFloat64(f float64) sql.NullFloat64 {
	if f == 0.0 { // Or whatever your condition for "not set" is, e.g. NaN
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{
		Float64: f,
		Valid:   true,
	}
}

// You'll also need to update FindCityByNameAndCountry to retrieve these new fields.
func (r *RepositoryImpl) FindCityByNameAndCountry(ctx context.Context, cityName, countryName string) (*types.CityDetail, error) {
	query := `
        SELECT 
            id, name, country, 
            COALESCE(state_province, '') as state_province, -- Handle NULL state_province
            ai_summary,
            ST_Y(center_location) as center_latitude,    -- Extract Y coordinate (latitude)
            ST_X(center_location) as center_longitude   -- Extract X coordinate (longitude)
            -- Add bounding_box retrieval if you store it: ST_AsText(bounding_box) as bounding_box_wkt
        FROM cities
        WHERE LOWER(name) = LOWER($1)
        AND ($2 = '' OR country = $2)
    `

	var cityDetail types.CityDetail
	var lat, lon sql.NullFloat64 // To handle potentially NULL location

	err := r.pgpool.QueryRow(ctx, query, cityName, countryName).Scan(
		&cityDetail.ID,
		&cityDetail.Name,
		&cityDetail.Country,
		&cityDetail.StateProvince,
		&cityDetail.AiSummary,
		&lat,
		&lon,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find city '%s', '%s': %w", cityName, countryName, err)
	}

	if lat.Valid {
		cityDetail.CenterLatitude = lat.Float64
	}
	if lon.Valid {
		cityDetail.CenterLongitude = lon.Float64
	}

	return &cityDetail, nil
}

// LevenshteinDistance calculates the Levenshtein distance between two strings.
func LevenshteinDistance(a, b string) int {
	// Create a 2D slice to store the distances
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
	}

	// Initialize the first row and column
	for i := 0; i <= len(a); i++ {
		d[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		d[0][j] = j
	}

	// Fill the rest of the matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			d[i][j] = min(
				d[i-1][j]+1,      // Deletion
				d[i][j-1]+1,      // Insertion
				d[i-1][j-1]+cost, // Substitution
			)
		}
	}

	// Return the final distance
	return d[len(a)][len(b)]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
	} else {
		if b < c {
			return b
		}
	}
	return c
}

// FindCityByFuzzyName finds the city with the most similar name using Levenshtein distance.
func (r *RepositoryImpl) FindCityByFuzzyName(ctx context.Context, cityName string) (*types.CityDetail, error) {
	// Get all cities from the database
	cities, err := r.GetAllCities(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all cities: %w", err)
	}

	// Find the city with the minimum Levenshtein distance
	var bestMatch *types.CityDetail
	minDistance := -1

	for i, city := range cities {
		distance := LevenshteinDistance(strings.ToLower(cityName), strings.ToLower(city.Name))
		if minDistance == -1 || distance < minDistance {
			minDistance = distance
			bestMatch = &cities[i]
		}
	}

	// You can set a threshold for the maximum allowed distance
	// For example, if the distance is too large, you might not want to return a match
	// if minDistance > 3 {
	// 	return nil, nil
	// }

	return bestMatch, nil
}

// GetCityIDByName retrieves a city ID by its name
func (r *RepositoryImpl) GetCityIDByName(ctx context.Context, cityName string) (uuid.UUID, error) {
	ctx, span := otel.Tracer("CityRepository").Start(ctx, "GetCityIDByName", trace.WithAttributes(
		attribute.String("city.name", cityName),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetCityIDByName"))

	query := `
        SELECT id
        FROM cities
        WHERE LOWER(name) = LOWER($1)
        LIMIT 1
    `

	var cityID uuid.UUID
	err := r.pgpool.QueryRow(ctx, query, cityName).Scan(&cityID)
	if err != nil {
		if err == pgx.ErrNoRows {
			l.WarnContext(ctx, "City not found", slog.String("city_name", cityName))
			span.SetStatus(codes.Error, "City not found")
			return uuid.Nil, fmt.Errorf("city not found: %s", cityName)
		}
		l.ErrorContext(ctx, "Failed to get city ID by name",
			slog.Any("error", err),
			slog.String("city_name", cityName))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return uuid.Nil, fmt.Errorf("failed to get city ID by name '%s': %w", cityName, err)
	}

	l.InfoContext(ctx, "City ID retrieved successfully",
		slog.String("city_name", cityName),
		slog.String("city_id", cityID.String()))
	span.SetAttributes(
		attribute.String("city.name", cityName),
		attribute.String("city.id", cityID.String()),
	)
	span.SetStatus(codes.Ok, "City ID retrieved")

	return cityID, nil
}

// FindSimilarCities finds cities similar to the provided query embedding using cosine similarity
func (r *RepositoryImpl) FindSimilarCities(ctx context.Context, queryEmbedding []float32, limit int) ([]types.CityDetail, error) {
	ctx, span := otel.Tracer("CityRepository").Start(ctx, "FindSimilarCities", trace.WithAttributes(
		attribute.Int("embedding.dimension", len(queryEmbedding)),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "FindSimilarCities"))

	// Convert []float32 to pgvector format string
	embeddingStr := fmt.Sprintf("[%v]", strings.Join(func() []string {
		strs := make([]string, len(queryEmbedding))
		for i, v := range queryEmbedding {
			strs[i] = fmt.Sprintf("%f", v)
		}
		return strs
	}(), ","))

	query := `
        SELECT 
            id, 
            name, 
            country,
            COALESCE(state_province, '') as state_province,
            ai_summary,
            ST_Y(center_location) as center_latitude,
            ST_X(center_location) as center_longitude,
            1 - (embedding <=> $1::vector) AS similarity_score
        FROM cities
        WHERE embedding IS NOT NULL
        ORDER BY embedding <=> $1::vector
        LIMIT $2
    `

	l.DebugContext(ctx, "Executing city similarity search query",
		slog.String("query", query),
		slog.Int("embedding_dim", len(queryEmbedding)),
		slog.Int("limit", limit))

	rows, err := r.pgpool.Query(ctx, query, embeddingStr, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query similar cities", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to search similar cities: %w", err)
	}
	defer rows.Close()

	var cities []types.CityDetail
	for rows.Next() {
		var city types.CityDetail
		var similarityScore float64
		var lat, lon sql.NullFloat64

		err := rows.Scan(
			&city.ID,
			&city.Name,
			&city.Country,
			&city.StateProvince,
			&city.AiSummary,
			&lat,
			&lon,
			&similarityScore,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan similar city row", slog.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan similar city row: %w", err)
		}

		if lat.Valid {
			city.CenterLatitude = lat.Float64
		}
		if lon.Valid {
			city.CenterLongitude = lon.Float64
		}

		cities = append(cities, city)
	}

	if err = rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating similar city rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating similar city rows: %w", err)
	}

	l.InfoContext(ctx, "Similar cities found", slog.Int("count", len(cities)))
	span.SetAttributes(attribute.Int("results.count", len(cities)))
	span.SetStatus(codes.Ok, "Similar cities found")

	return cities, nil
}

// UpdateCityEmbedding updates the embedding vector for a specific city
func (r *RepositoryImpl) UpdateCityEmbedding(ctx context.Context, cityID uuid.UUID, embedding []float32) error {
	ctx, span := otel.Tracer("CityRepository").Start(ctx, "UpdateCityEmbedding", trace.WithAttributes(
		attribute.String("city.id", cityID.String()),
		attribute.Int("embedding.dimension", len(embedding)),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "UpdateCityEmbedding"))

	// Convert []float32 to pgvector format string
	embeddingStr := fmt.Sprintf("[%v]", strings.Join(func() []string {
		strs := make([]string, len(embedding))
		for i, v := range embedding {
			strs[i] = fmt.Sprintf("%f", v)
		}
		return strs
	}(), ","))

	query := `
        UPDATE cities 
        SET embedding = $1::vector, embedding_generated_at = NOW()
        WHERE id = $2
    `

	result, err := r.pgpool.Exec(ctx, query, embeddingStr, cityID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to update city embedding",
			slog.Any("error", err),
			slog.String("city_id", cityID.String()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database update failed")
		return fmt.Errorf("failed to update city embedding: %w", err)
	}

	if result.RowsAffected() == 0 {
		err := fmt.Errorf("no city found with ID %s", cityID.String())
		l.WarnContext(ctx, "No city found for embedding update", slog.String("city_id", cityID.String()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "City not found")
		return err
	}

	l.InfoContext(ctx, "City embedding updated successfully",
		slog.String("city_id", cityID.String()),
		slog.Int("embedding_dimension", len(embedding)))
	span.SetAttributes(
		attribute.String("city.id", cityID.String()),
		attribute.Int("embedding.dimension", len(embedding)),
	)
	span.SetStatus(codes.Ok, "City embedding updated")

	return nil
}

// GetCitiesWithoutEmbeddings retrieves cities that don't have embeddings generated yet
func (r *RepositoryImpl) GetCitiesWithoutEmbeddings(ctx context.Context, limit int) ([]types.CityDetail, error) {
	ctx, span := otel.Tracer("CityRepository").Start(ctx, "GetCitiesWithoutEmbeddings", trace.WithAttributes(
		attribute.Int("limit", limit),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetCitiesWithoutEmbeddings"))

	query := `
        SELECT 
            id, 
            name, 
            country,
            COALESCE(state_province, '') as state_province,
            ai_summary,
            ST_Y(center_location) as center_latitude,
            ST_X(center_location) as center_longitude
        FROM cities
        WHERE embedding IS NULL
        ORDER BY created_at ASC
        LIMIT $1
    `

	rows, err := r.pgpool.Query(ctx, query, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query cities without embeddings", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query cities without embeddings: %w", err)
	}
	defer rows.Close()

	var cities []types.CityDetail
	for rows.Next() {
		var city types.CityDetail
		var lat, lon sql.NullFloat64

		err := rows.Scan(
			&city.ID,
			&city.Name,
			&city.Country,
			&city.StateProvince,
			&city.AiSummary,
			&lat,
			&lon,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan city without embedding row", slog.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan city without embedding row: %w", err)
		}

		if lat.Valid {
			city.CenterLatitude = lat.Float64
		}
		if lon.Valid {
			city.CenterLongitude = lon.Float64
		}

		cities = append(cities, city)
	}

	if err = rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating city without embedding rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating city without embedding rows: %w", err)
	}

	l.InfoContext(ctx, "Cities without embeddings found", slog.Int("count", len(cities)))
	span.SetAttributes(attribute.Int("results.count", len(cities)))
	span.SetStatus(codes.Ok, "Cities without embeddings retrieved")

	return cities, nil
}

// GetAllCities retrieves all cities from the database with their coordinates
func (r *RepositoryImpl) GetAllCities(ctx context.Context) ([]types.CityDetail, error) {
	ctx, span := otel.Tracer("CityRepository").Start(ctx, "GetAllCities")
	defer span.End()

	l := r.logger.With(slog.String("method", "GetAllCities"))

	query := `
        SELECT 
            id, 
            name, 
            country,
            COALESCE(state_province, '') as state_province,
            ai_summary,
            ST_Y(center_location) as center_latitude,
            ST_X(center_location) as center_longitude
        FROM cities
        WHERE center_location IS NOT NULL
        ORDER BY name ASC
    `

	rows, err := r.pgpool.Query(ctx, query)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query all cities", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query all cities: %w", err)
	}
	defer rows.Close()

	var cities []types.CityDetail
	for rows.Next() {
		var city types.CityDetail
		var lat, lon sql.NullFloat64

		err := rows.Scan(
			&city.ID,
			&city.Name,
			&city.Country,
			&city.StateProvince,
			&city.AiSummary,
			&lat,
			&lon,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan city row", slog.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan city row: %w", err)
		}

		if lat.Valid {
			city.CenterLatitude = lat.Float64
		}
		if lon.Valid {
			city.CenterLongitude = lon.Float64
		}

		cities = append(cities, city)
	}

	if err = rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating city rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating city rows: %w", err)
	}

	l.InfoContext(ctx, "All cities retrieved", slog.Int("count", len(cities)))
	span.SetAttributes(attribute.Int("results.count", len(cities)))
	span.SetStatus(codes.Ok, "All cities retrieved")

	return cities, nil
}

// determineCityID finds the city ID and name closest to the given latitude and longitude
func (l *RepositoryImpl) GetCity(ctx context.Context, lat, lon float64) (uuid.UUID, string, error) {
	// Start OpenTelemetry tracing
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "determineCityID", trace.WithAttributes(
		attribute.Float64("lat", lat),
		attribute.Float64("lon", lon),
	))
	defer span.End()

	// Log the request
	l.logger.DebugContext(ctx, "Determining city ID for coordinates",
		slog.Float64("lat", lat),
		slog.Float64("lon", lon))

	// Create a POINT geometry from longitude and latitude
	point := fmt.Sprintf("POINT(%f %f)", lon, lat)

	// SQL query to find the closest city based on center_location
	query := `
        SELECT id, name
        FROM cities
        ORDER BY ST_Distance(center_location, ST_GeomFromText($1, 4326)) ASC
        LIMIT 1
    `

	var cityID uuid.UUID
	var cityName string
	err := l.pgpool.QueryRow(ctx, query, point).Scan(&cityID, &cityName)
	if err != nil {
		if err == pgx.ErrNoRows {
			l.logger.WarnContext(ctx, "No city found for the given coordinates")
			span.SetStatus(codes.Error, "No city found")
			return uuid.Nil, "", fmt.Errorf("no city found for coordinates (%f, %f)", lat, lon)
		}
		l.logger.ErrorContext(ctx, "Failed to determine city ID", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return uuid.Nil, "", fmt.Errorf("failed to determine city ID: %w", err)
	}

	// Log success and set tracing attributes
	l.logger.InfoContext(ctx, "City determined",
		slog.String("city_id", cityID.String()),
		slog.String("city_name", cityName))
	span.SetAttributes(
		attribute.String("city.id", cityID.String()),
		attribute.String("city.name", cityName),
	)
	span.SetStatus(codes.Ok, "City determined")

	return cityID, cityName, nil
}
