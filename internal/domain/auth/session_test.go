package auth

import "errors"

// MockSessionManager is a mock implementation of the auth.SessionManager interface for testing.
type MockSessionManager struct{}

func (m *MockSessionManager) CreateSession(userID int) (*UserSession, error) {
	// Implement the mock behavior for creating a session.
	// For example, you can return a predefined session or an error based on the input userID.
	return &UserSession{
		ID:       string(userID),
		Username: "duck",
		Email:    "duck@duck.com",
	}, nil
}

func (m *MockSessionManager) GetSession(sessionID int) (*UserSession, error) {
	// Implement the mock behavior for getting a session by sessionID.
	// For testing purposes, you can return a predefined session or an error based on the input sessionID.
	if sessionID == 69 {
		return &UserSession{
			ID:       string(sessionID),
			Username: "duck",
			Email:    "duck@duck.com",
		}, nil
	}
	return nil, errors.New("session not found")
}
