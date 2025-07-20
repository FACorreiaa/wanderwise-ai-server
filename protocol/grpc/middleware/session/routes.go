package session

// rolePermissions Define permissions for each role
var rolePermissions = map[string][]string{
	"USER":     {"view_profile", "view_workouts, calculator_service"},
	"PT":       {"view_profile", "view_workouts", "manage_clients, calculator_service"},
	"ADMIN":    {"view_profile", "view_workouts", "manage_clients", "admin_dashboard", "view_all_users, calculator_service"},
	"VISITORS": {},
}

// MethodPermissions Define required permissions for each gRPC method
var MethodPermissions = map[string]string{
	"/auth.Auth/GetUserInfo":                 "view_profile",
	"/auth.Auth/ManageClients":               "manage_clients",
	"/auth.Auth/ViewWorkouts":                "view_workouts",
	"/auth.Auth/AdminDashboard":              "admin_dashboard",
	"calculator.Calculator/GetUsersMacros":   "calculator_service",
	"calculator.Calculator/GetUserMacros":    "calculator_service",
	"calculator.Calculator/CreateUserMacros": "calculator_service",
}

func GetUserPermissions(role string) []string {
	// Check if the role exists in the rolePermissions map
	if rolePerms, exists := rolePermissions[role]; exists {
		return rolePerms
	}
	// Return an empty list if the role doesn't exist or is invalid
	return []string{}
}
