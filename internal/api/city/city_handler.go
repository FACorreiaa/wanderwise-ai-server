package city

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

type Handler struct {
	logger  *slog.Logger
	service Service
}

func NewCityHandler(service Service, logger *slog.Logger) *Handler {
	return &Handler{
		logger:  logger,
		service: service,
	}
}

// GetAllCities handles GET /cities
func (h *Handler) GetAllCities(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("CityHandler").Start(r.Context(), "GetAllCities")
	defer span.End()

	l := h.logger.With(slog.String("method", "GetAllCities"))

	if r.Method != http.MethodGet {
		l.WarnContext(ctx, "Method not allowed", slog.String("method", r.Method))
		span.SetStatus(codes.Error, "Method not allowed")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	l.InfoContext(ctx, "Retrieving all cities")

	cities, err := h.service.GetAllCities(ctx)
	if err != nil {
		l.ErrorContext(ctx, "Failed to retrieve cities", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Service operation failed")
		http.Error(w, "Failed to retrieve cities", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if err := json.NewEncoder(w).Encode(cities); err != nil {
		l.ErrorContext(ctx, "Failed to encode cities response", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "JSON encoding failed")
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	l.InfoContext(ctx, "Successfully returned cities", slog.Int("count", len(cities)))
	span.SetStatus(codes.Ok, "Cities returned successfully")
}
