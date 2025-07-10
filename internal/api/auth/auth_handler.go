package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/markbates/goth/gothic"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// ProviderKey is a custom type for context keys to avoid collisions
type ProviderKey string

const providerKey ProviderKey = "provider"

var _ Handler = (*HandlerImpl)(nil)

type Handler interface {
	Login(w http.ResponseWriter, r *http.Request)
	Logout(w http.ResponseWriter, r *http.Request)
	RefreshToken(w http.ResponseWriter, r *http.Request)
	Register(w http.ResponseWriter, r *http.Request)
	ValidateSession(w http.ResponseWriter, r *http.Request)
	ChangePassword(w http.ResponseWriter, r *http.Request)

	// provider
	LoginWithGoogle(w http.ResponseWriter, r *http.Request)
	GoogleCallback(w http.ResponseWriter, r *http.Request)
}
type HandlerImpl struct {
	authService AuthService
	logger      *slog.Logger
}

func NewAuthHandlerImpl(authService AuthService, logger *slog.Logger) *HandlerImpl {
	return &HandlerImpl{
		logger:      logger,
		authService: authService,
	}
}

// Login godoc
// @Summary      User Login
// @Description  Authenticates a user and returns JWT access and refresh tokens.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        credentials body types.LoginRequest true "Login Credentials"
// @Success      200 {object} types.LoginResponse "Successful Login"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Authentication Failed"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Router       /auth/login [post]
func (h *HandlerImpl) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := h.logger.With(slog.String("HandlerImpl", "Login"))

	var req types.LoginRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.WarnContext(ctx, "Failed to decode request", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request format")
		return
	}
	if req.Email == "" || req.Password == "" {
		api.ErrorResponse(w, r, http.StatusBadRequest, "Email and password are required")
		return
	}

	accessToken, refreshToken, err := h.authService.Login(ctx, req.Email, req.Password)
	if err != nil {
		l.WarnContext(ctx, "Service login failed", slog.Any("error", err), slog.String("email", req.Email))
		if errors.Is(err, types.ErrUnauthenticated) {
			api.ErrorResponse(w, r, http.StatusUnauthorized, "Invalid email or password")
		} else {
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Login failed")
		}
		return
	}

	// Set refresh token in HttpOnly cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refreshToken",
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // Set to true if using HTTPS
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((7 * 24 * time.Hour).Seconds()), // Match your refresh token TTL
	})

	// Respond with access token only
	resp := types.LoginResponse{
		AccessToken: accessToken,
		Message:     "Login successful",
	}
	l.InfoContext(ctx, "Login successful", slog.String("email", req.Email))
	api.WriteJSONResponse(w, r, http.StatusOK, resp)
}

// Logout godoc
// @Summary      User Logout
// @Description  Invalidates the user's current session/refresh token. Typically uses Refresh Token from HttpOnly cookie. Body might be empty or contain refresh token if not using cookies.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        token body types.LogoutRequest false "Logout Request (only needed if sending refresh_token in body)"
// @Success      200 {object} types.Response "Logout Successful"
// @Failure      400 {object} types.Response "Bad Request (e.g., malformed body if used)"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /auth/logout [post]
func (h *HandlerImpl) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := h.logger.With(slog.String("HandlerImpl", "Logout"))

	// Extract refresh token from cookie
	refreshCookie, err := r.Cookie("refreshToken")
	if err != nil {
		if err == http.ErrNoCookie {
			l.WarnContext(ctx, "No refresh token cookie present for logout")
			// Still proceed to clear cookie and succeed
		} else {
			l.ErrorContext(ctx, "Error reading refresh token cookie", slog.Any("error", err))
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Internal server error")
			return
		}
	}
	refreshToken := ""
	if refreshCookie != nil {
		refreshToken = refreshCookie.Value
	}

	// Invalidate refresh token if present
	if refreshToken != "" {
		err = h.authService.Logout(ctx, refreshToken)
		if err != nil {
			l.ErrorContext(ctx, "Service logout failed", slog.Any("error", err))
			// Proceed anyway, as cookie will be cleared
		}
	}

	// Clear the refresh token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refreshToken",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   -1, // Delete cookie
	})

	l.InfoContext(ctx, "Logout processed")
	api.WriteJSONResponse(w, r, http.StatusOK, types.Response{Success: true, Message: "Logged out successfully"})
}

// RefreshToken godoc
// @Summary      Refresh Access Token
// @Description  Provides a new access token using a valid refresh token (typically sent via HttpOnly cookie). Body might be empty or contain refresh token if not using cookies.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        token body types.RefreshTokenRequest false "Refresh Token Request (only needed if sending refresh_token in body)"
// @Success      200 {object} types.TokenResponse "New Access Token (Refresh Token set in cookie)"
// @Failure      400 {object} types.Response "Bad Request (e.g., missing token)"
// @Failure      401 {object} types.Response "Invalid or Expired Refresh Token"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Router       /auth/refresh [post]
func (h *HandlerImpl) RefreshToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := h.logger.With(slog.String("HandlerImpl", "RefreshToken"))

	// Extract refresh token from cookie
	refreshCookie, err := r.Cookie("refreshToken")
	if err != nil {
		if err == http.ErrNoCookie {
			h.respondWithError(w, http.StatusBadRequest, "Refresh token cookie missing")
			return
		}
		l.Error("Error reading cookie", "error", err)
		h.respondWithError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	refreshToken := refreshCookie.Value

	if refreshToken == "" {
		h.respondWithError(w, http.StatusBadRequest, "Refresh token is required")
		return
	}

	// Call service
	accessToken, newRefreshToken, err := h.authService.RefreshSession(ctx, refreshToken)
	if err != nil {
		l.Error("Token refresh failed", "error", err)
		h.respondWithError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	// Set new refresh token in cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refreshToken",
		Value:    newRefreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
	})

	// Respond with access token only
	h.respondWithJSON(w, http.StatusOK, types.TokenResponse{
		AccessToken: accessToken,
	})
}

// Register godoc
// @Summary      Register New User
// @Description  Creates a new user account.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        user body types.RegisterRequest true "User Registration Details"
// @Success      201 {object} types.Response "User Registered Successfully"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      409 {object} types.Response "Email or Username already exists"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Router       /auth/register [post]
func (h *HandlerImpl) Register(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("RegisterHandlerImpl").Start(r.Context(), "RegisterHandlerImpl", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/register"),
	))
	defer span.End()

	l := h.logger.With(slog.String("HandlerImpl", "Register"))

	// Record start time for duration metric
	//startTime := time.Now()

	var req types.RegisterRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.WarnContext(ctx, "Failed to decode request", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid request format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request format")
		return
	}

	if req.Email == "" || req.Password == "" || req.Username == "" {
		span.SetStatus(codes.Error, "Missing required fields")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Username, email, and password are required")
		return
	}

	// Call the service layer with the traced context
	err := h.authService.Register(ctx, req.Username, req.Email, req.Password, req.Role)
	if err != nil {
		l.ErrorContext(ctx, "Service registration failed", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Registration failed")
		if errors.Is(err, types.ErrConflict) {
			api.ErrorResponse(w, r, http.StatusConflict, "Email or username already exists")
		} else {
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Registration failed")
		}
		return
	}

	// Record metrics
	// duration := time.Since(startTime).Seconds()
	// registerRequestsTotal.Add(ctx, 1)
	// registerDurationSeconds.Record(ctx, duration)

	l.InfoContext(ctx, "User registered successfully", slog.String("email", req.Email))
	span.SetStatus(codes.Ok, "User registered successfully")
	api.WriteJSONResponse(w, r, http.StatusCreated, types.Response{Success: true, Message: "User registered successfully"})
}

// ValidateSession godoc
// @Summary      Validate JWT Access Token
// @Description  Validates the JWT access token from Authorization header and returns user information.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Success      200 {object} types.ValidateSessionResponse "Token validation result with user info if valid"
// @Failure      401 {object} types.Response "Invalid or expired token"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /auth/validate-session [post]
func (h *HandlerImpl) ValidateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := h.logger.With(slog.String("handler", "ValidateSession"))

	// Get UserID from context (set by Authenticate middleware)
	userID, ok := GetUserIDFromContext(ctx)
	if !ok || userID == "" {
		l.WarnContext(ctx, "User ID not found in context - middleware issue")
		h.respondWithJSON(w, http.StatusOK, types.ValidateSessionResponse{
			Valid: false,
		})
		return
	}

	// Fetch user details to return complete info
	user, err := h.authService.GetUserByID(ctx, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get user details", slog.String("userID", userID), slog.Any("error", err))
		h.respondWithJSON(w, http.StatusOK, types.ValidateSessionResponse{
			Valid: false,
		})
		return
	}

	// Token is valid (confirmed by middleware), return user info
	l.DebugContext(ctx, "JWT token validation successful", slog.String("userID", userID))
	h.respondWithJSON(w, http.StatusOK, types.ValidateSessionResponse{
		Valid:    true,
		UserID:   user.ID,
		Username: user.Username,
		Email:    user.Email,
	})
}

// ChangePassword godoc
// @Summary      Change User Password
// @Description  Allows an authenticated user to change their own password.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        passwords body types.ChangePasswordRequest true "Old and New Passwords"
// @Success      200 {object} types.Response "Password Updated Successfully"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Unauthorized (Invalid old password or bad token)"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /auth/password [put]
func (h *HandlerImpl) ChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := h.logger.With(slog.String("HandlerImpl", "ChangePassword"))

	// Get UserID from context (set by Authenticate middleware)
	userID, ok := GetUserIDFromContext(ctx) // Use actual helper
	if !ok || userID == "" {
		l.ErrorContext(ctx, "User ID not found in context for ChangePassword")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	l = l.With(slog.String("userID", userID))

	var req types.ChangePasswordRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.WarnContext(ctx, "Failed to decode request", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request format")
		return
	}
	// Add validation for passwords
	if req.OldPassword == "" || req.NewPassword == "" {
		api.ErrorResponse(w, r, http.StatusBadRequest, "Old and new passwords are required")
		return
	}
	if req.OldPassword == req.NewPassword {
		api.ErrorResponse(w, r, http.StatusBadRequest, "New password must be different from old password")
		return
	}

	l.DebugContext(ctx, "Attempting password change")
	err := h.authService.UpdatePassword(ctx, userID, req.OldPassword, req.NewPassword)
	if err != nil {
		l.ErrorContext(ctx, "Service password update failed", slog.Any("error", err))
		if errors.Is(err, types.ErrUnauthenticated) { // Check if service returned this
			api.ErrorResponse(w, r, http.StatusUnauthorized, "Incorrect old password")
		} else {
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to update password")
		}
		return
	}

	l.InfoContext(ctx, "Password updated successfully")
	api.WriteJSONResponse(w, r, http.StatusOK, types.Response{Success: true, Message: "Password updated successfully"})
}

// ChangeEmail godoc
// @Summary      Change User Email
// @Description  Allows an authenticated user to change their email address after verifying their password.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        email_change body types.ChangeEmailRequest true "Password verification and new email"
// @Success      200 {object} types.Response "Email Updated Successfully"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Unauthorized (Invalid password or bad token)"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /auth/email [put]
func (h *HandlerImpl) ChangeEmail(w http.ResponseWriter, r *http.Request) {
	var req types.ChangeEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Validate request
	if req.Password == "" || req.NewEmail == "" {
		h.respondWithError(w, http.StatusBadRequest, "All fields are required")
		return
	}

	// Get user ID from context (assuming middleware has set it)
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		h.respondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Verify password
	err := h.authService.VerifyPassword(r.Context(), userID, req.Password)
	if err != nil {
		h.logger.Error("Password verification failed", "error", err)
		h.respondWithError(w, http.StatusUnauthorized, "Invalid password")
		return
	}

	// Call service to update email
	// Note: In a real implementation, you would have a dedicated method for this
	// For now, we'll assume there's an UpdateEmail method on the user repository
	// that would be called by the service
	h.respondWithError(w, http.StatusNotImplemented, "Email change not implemented")
}

// Helper functions for response handling

func (h *HandlerImpl) respondWithError(w http.ResponseWriter, code int, message string) {
	h.respondWithJSON(w, code, types.Response{
		Success: false,
		Error:   message,
	})
}

func (h *HandlerImpl) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("Failed to marshal JSON response", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, err = w.Write(response)
	if err != nil {
		h.logger.Error("Failed to write response", "error", err)
	}
}

// AuthenticateMiddleware godoc
// @Summary      Authentication Middleware
// @Description  Middleware that authenticates requests using JWT tokens and adds user information to the request context.
// @Tags         Auth
// @Security     BearerAuth
func (h *HandlerImpl) AuthenticateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Implement your authentication logic here
		next.ServeHTTP(w, r)
	})
}

// RefreshSession godoc
// @Summary      Refresh User Session
// @Description  Refreshes a user's session using a refresh token provided in the request body.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        token body types.RefreshTokenRequest true "Refresh Token"
// @Success      200 {object} types.TokenResponse "New Access Token"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Invalid or Expired Refresh Token"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Router       /auth/refresh-session [post]
func (h *HandlerImpl) RefreshSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := h.logger.With(slog.String("HandlerImpl", "RefreshSession"))

	var req types.RefreshTokenRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil || req.RefreshToken == "" {
		l.WarnContext(ctx, "Missing refresh token for refresh", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Refresh token required")
		return
	}
	refreshToken := req.RefreshToken

	l.DebugContext(ctx, "Attempting token refresh")
	newAccessToken, newRefreshToken, err := h.authService.RefreshSession(ctx, refreshToken)
	if err != nil {
		l.WarnContext(ctx, "Service token refresh failed", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	// Set new refresh token in HttpOnly cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refreshToken",
		Value:    newRefreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // Use true for HTTPS in production
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((7 * 24 * time.Hour).Seconds()), // Match your refresh token TTL
	})

	resp := types.TokenResponse{
		AccessToken: newAccessToken,
	}
	l.InfoContext(ctx, "Token refresh successful")
	api.WriteJSONResponse(w, r, http.StatusOK, resp)
}

// LoginWithGoogle initiates the Google authentication flow
func (h *HandlerImpl) LoginWithGoogle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := h.logger.With(slog.String("HandlerImpl", "LoginWithGoogle"))

	// Set the provider in the context (Goth uses this to determine which provider to use)
	ctx = context.WithValue(ctx, providerKey, "google")
	r = r.WithContext(ctx)

	// Begin the authentication process and redirect the user to Google's login page
	gothic.BeginAuthHandler(w, r)
	l.DebugContext(r.Context(), "Google auth")
}

// GoogleCallback godoc
// @Summary      Google Authentication Callback
// @Description  Handles the callback from Google after user authentication, logging in existing users or registering new ones.
// @Tags         Auth
// @Produce      json
// @Success      200 {object} types.LoginResponse "Authentication successful"
// @Failure      400 {object} types.Response "Invalid request"
// @Failure      401 {object} types.Response "Authentication failed"
// @Failure      409 {object} types.Response "Email already exists"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Router       /auth/google/callback [get]
func (h *HandlerImpl) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	// Start tracing
	ctx, span := otel.Tracer("GoogleCallbackHandlerImpl").Start(r.Context(), "GoogleCallbackHandlerImpl", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/auth/google/callback"),
	))
	defer span.End()

	l := h.logger.With(slog.String("HandlerImpl", "GoogleCallback"))

	// Set the provider in the context
	ctx = context.WithValue(ctx, providerKey, "google")
	r = r.WithContext(ctx)

	// Complete the authentication process and retrieve user information
	gothUser, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		l.ErrorContext(ctx, "Failed to complete Google authentication", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Authentication failed")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication failed")
		return
	}

	// Get or create the user based on Google provider information
	user, err := h.authService.GetOrCreateUserFromProvider(ctx, "google", gothUser)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get or create user", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "User processing failed")
		if errors.Is(err, types.ErrConflict) {
			api.ErrorResponse(w, r, http.StatusConflict, "Email already exists. Please log in with your existing account.")
		} else {
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to process authentication")
		}
		return
	}

	// Generate JWT tokens
	accessToken, refreshToken, err := h.authService.GenerateTokens(ctx, user, nil)
	if err != nil {
		l.ErrorContext(ctx, "Failed to generate tokens", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Token generation failed")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to generate tokens")
		return
	}

	// Set refresh token in an HttpOnly cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refreshToken",
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // Set to true if using HTTPS
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((7 * 24 * time.Hour).Seconds()), // 7 days
	})

	// Prepare    // Respond with the access token and success message
	resp := types.LoginResponse{
		AccessToken: accessToken,
		Message:     "Authentication successful",
	}
	l.InfoContext(ctx, "Authentication successful", slog.String("email", gothUser.Email))
	span.SetStatus(codes.Ok, "Authentication successful")
	api.WriteJSONResponse(w, r, http.StatusOK, resp)
}
