package auth

type SessionManagerKey struct{}

type UserSession struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type User struct {
	ID       string
	Username string
	Email    string
	Password string
}
