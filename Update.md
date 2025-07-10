Some of my Update use dynamic querying to update multiple fields:

like 

func (r *PostgresUserRepo) UpdateProfile(ctx context.Context, userID uuid.UUID, params types.UpdateProfileParams) error {
ctx, span := otel.Tracer("UserRepo").Start(ctx, "UpdateProfile", trace.WithAttributes(
semconv.DBSystemPostgreSQL,
attribute.String("db.operation", "UPDATE"),
attribute.String("db.sql.table", "users"),
attribute.String("db.user.id", userID.String()),
))
defer span.End()

	l := r.logger.With(slog.String("method", "UpdateProfile"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Updating user profile", slog.Any("params", params)) // Log incoming params

	// Use squirrel or build query dynamically
	var setClauses []string
	var args []interface{}
	argID := 1 // Argument counter for placeholders ($1, $2, ...)

	// Check each field in params. If not nil, add to SET clause and args slice.
	if params.Username != nil {
		setClauses = append(setClauses, fmt.Sprintf("username = $%d", argID))
		args = append(args, *params.Username)
		argID++
		span.SetAttributes(attribute.Bool("update.username", true)) // Add trace attribute
	}
	if params.Email != nil {
		setClauses = append(setClauses, fmt.Sprintf("email = $%d", argID))
		args = append(args, *params.Email)
		argID++
		span.SetAttributes(attribute.Bool("update.email", true))
	}
	if params.DisplayName != nil {
		setClauses = append(setClauses, fmt.Sprintf("display_name = $%d", argID))
		args = append(args, *params.DisplayName)
		argID++
		span.SetAttributes(attribute.Bool("update.display_name", true))
	}
	if params.ProfileImageURL != nil {
		setClauses = append(setClauses, fmt.Sprintf("profile_image_url = $%d", argID))
		args = append(args, *params.ProfileImageURL)
		argID++
		span.SetAttributes(attribute.Bool("update.profile_image_url", true))
	}
	if params.Firstname != nil {
		setClauses = append(setClauses, fmt.Sprintf("firstname = $%d", argID))
		args = append(args, *params.Firstname)
		argID++
		span.SetAttributes(attribute.Bool("update.firstname", true))
	}
	if params.Lastname != nil {
		setClauses = append(setClauses, fmt.Sprintf("lastname = $%d", argID))
		args = append(args, *params.Lastname)
		argID++
		span.SetAttributes(attribute.Bool("update.lastname", true))
	}
	if params.Age != nil {
		setClauses = append(setClauses, fmt.Sprintf("age = $%d", argID))
		args = append(args, *params.Age)
		argID++
		span.SetAttributes(attribute.Bool("update.age", true))
	}
	if params.City != nil {
		setClauses = append(setClauses, fmt.Sprintf("city = $%d", argID))
		args = append(args, *params.City)
		argID++
		span.SetAttributes(attribute.Bool("update.city", true))
	}
	if params.Country != nil {
		setClauses = append(setClauses, fmt.Sprintf("country = $%d", argID))
		args = append(args, *params.Country)
		argID++
		span.SetAttributes(attribute.Bool("update.country", true))
	}
	if params.AboutYou != nil {
		setClauses = append(setClauses, fmt.Sprintf("about_you = $%d", argID))
		args = append(args, *params.AboutYou)
		argID++
		span.SetAttributes(attribute.Bool("update.about_you", true))
	}
	if params.Location != nil {
		setClauses = append(setClauses, fmt.Sprintf("location = $%d", argID))
		args = append(args, *params.Location)
		argID++
		span.SetAttributes(attribute.Bool("update.location", true))
	}
	if params.Interests != nil {
		setClauses = append(setClauses, fmt.Sprintf("interests = $%d", argID))
		args = append(args, *params.Interests)
		argID++
		span.SetAttributes(attribute.Bool("update.interests", true))
	}
	if params.PhoneNumber != nil {
		setClauses = append(setClauses, fmt.Sprintf("phone = $%d", argID))
		args = append(args, *params.PhoneNumber)
		argID++
		span.SetAttributes(attribute.Bool("update.phone", true))
	}
	if params.Badges != nil {
		setClauses = append(setClauses, fmt.Sprintf("badges = $%d", argID))
		args = append(args, *params.Badges)
		argID++
		span.SetAttributes(attribute.Bool("update.badges", true))
	}

	// If no fields were provided to update, return early (or error?)
	if len(setClauses) == 0 {
		l.WarnContext(ctx, "UpdateProfile called with no fields to update")
		span.SetStatus(codes.Ok, "No update fields provided") // Not an error, just no-op
		return nil                                            // Or return specific error types.ErrBadRequest("no update fields provided")
	}

	// Add updated_at clause (always update this if other fields change)
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argID))
	args = append(args, time.Now())
	argID++

	// Final WHERE clause argument
	args = append(args, userID)

	// Construct the final query
	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d AND is_active = TRUE",
		strings.Join(setClauses, ", "), // e.g., "username = $1, age = $2, updated_at = $3"
		argID,                          // The placeholder for userID
	)

	l.DebugContext(ctx, "Executing dynamic update query", slog.String("query", query), slog.Int("arg_count", len(args)))

	// Execute the dynamic query
	tag, err := r.pgpool.Exec(ctx, query, args...)
	if err != nil {
		// Add specific error checking (e.g., unique constraint violations on email/username if updated)
		l.ErrorContext(ctx, "Failed to execute update profile query", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error updating profile: %w", err)
	}

	// Check if the user existed and was updated
	if tag.RowsAffected() == 0 {
		l.WarnContext(ctx, "User not found or no update occurred", slog.Int64("rows_affected", tag.RowsAffected()))
		span.SetStatus(codes.Error, "User not found or no change")
		// Check if user exists to differentiate "not found" vs "no effective change"
		var exists bool
		// Use a separate query or modify the UPDATE to return something on match
		checkErr := r.pgpool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND is_active = TRUE)", userID).Scan(&exists)
		if checkErr == nil && !exists {
			return fmt.Errorf("user not found for update: %w", types.ErrNotFound)
		}
		// If user exists, maybe the provided values were the same as existing ones.
		// Or maybe user was inactive. Treat as not found for simplicity for now.
		return fmt.Errorf("user not found or update failed: %w", types.ErrNotFound)
	}

	l.InfoContext(ctx, "User profile updated successfully")
	span.SetStatus(codes.Ok, "Profile updated")
	return nil
}

Having fmt.Sprintf("badges = $%d", argID) could not be the most secure way of updating fields because it could be faulty to SQL Injections.
whats the alternative? Should I use something like Squirrel? Write your analysis here:

### Analysis of the Update Method

The concern raised about using `fmt.Sprintf` for constructing SQL queries is valid and demonstrates good security awareness. While the current implementation is not directly vulnerable to SQL injection, it uses a pattern that is considered risky and can be improved for maintainability and security.

#### SQL Injection Risk

1.  **Current State**: The code constructs the `SET` clauses by concatenating hardcoded column names with argument placeholders (e.g., `fmt.Sprintf("username = $%d", argID)`). The actual user-provided *values* are passed as separate arguments to `r.pgpool.Exec`, which uses parameterized queries. This correctly prevents SQL injection *through the values*. Since the column names (`username`, `email`, etc.) are static strings and not derived from user input, **this specific code is not currently vulnerable to SQL injection.**

2.  **The Risky Pattern**: The danger lies in the pattern itself. If a future developer modifies this code to include dynamic column names based on user input, it would immediately create a severe SQL injection vulnerability. Relying on developers to never make this mistake is a fragile security posture.

#### Maintainability

The current approach is verbose and prone to errors. Manually managing the argument counter (`argID`) and concatenating strings is cumbersome. Adding a new updatable field requires adding another `if` block and carefully managing the argument position, which increases boilerplate and the chance of bugs.

### Recommended Alternative: SQL Query Builder (Squirrel)

As suggested, using a library like **Squirrel** is the ideal solution. Squirrel is a fluent SQL generator that allows you to build complex queries programmatically while ensuring the security of parameterization.

#### Example Refactor with Squirrel

Here’s a conceptual example of how the `UpdateProfile` function could be refactored using Squirrel:

```go
import (
    "github.com/Masterminds/squirrel"
)

func (r *PostgresUserRepo) UpdateProfile(ctx context.Context, userID uuid.UUID, params types.UpdateProfileParams) error {
    // ... (tracing and logging setup remains the same) ...

    // Use squirrel's UpdateBuilder
    updateBuilder := squirrel.Update("users").
        PlaceholderFormat(squirrel.Dollar).
        Where(squirrel.Eq{"id": userID, "is_active": true}).
        Set("updated_at", time.Now())

    // Conditionally add SET clauses
    if params.Username != nil {
        updateBuilder = updateBuilder.Set("username", *params.Username)
    }
    if params.Email != nil {
        updateBuilder = updateBuilder.Set("email", *params.Email)
    }
    if params.DisplayName != nil {
        updateBuilder = updateBuilder.Set("display_name", *params.DisplayName)
    }
    // ... add other fields in the same manner ...

    // Generate the SQL query and arguments
    query, args, err := updateBuilder.ToSql()
    if err != nil {
        return fmt.Errorf("failed to build update query: %w", err)
    }

    // ... (log the generated query) ...

    // Execute the query
    tag, err := r.pgpool.Exec(ctx, query, args...)
    if err != nil {
        // ... (error handling) ...
        return fmt.Errorf("database error updating profile: %w", err)
    }

    if tag.RowsAffected() == 0 {
        // ... (handle user not found or no update occurred) ...
        return types.ErrNotFound
    }

    // ... (log success) ...
    return nil
}
```

### Benefits of Using Squirrel

1.  **Improved Security**: Squirrel handles query construction and parameterization correctly, completely eliminating the risk of SQL injection. It enforces a clean separation between the query structure and the data values.
2.  **Enhanced Readability & Maintainability**: The code becomes more declarative and easier to understand. The logic for building the query is fluent and less cluttered than manual string manipulation.
3.  **Reduced Boilerplate**: It significantly reduces repetitive code. Adding new fields is as simple as adding one more `.Set()` call, without needing to manage argument counters manually.

### Conclusion

While the existing code is not immediately vulnerable, the pattern it uses is an anti-pattern for building dynamic queries. **Migrating to a SQL builder library like Squirrel is the recommended approach.** It will make the code safer, more readable, and easier to maintain, aligning with modern Go development best practices.
