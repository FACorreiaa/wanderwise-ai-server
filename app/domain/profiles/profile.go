package profiles

// Profile represents a user profile in the system.
// It contains basic information about a user.
type Profile struct {
	// ID is the unique identifier for the profile.
	ID string `json:"id"`
	// Name is the name of the user.
	Name string `json:"name"`
}
