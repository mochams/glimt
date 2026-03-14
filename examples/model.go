package main

import "time"

// User represents a user in the system.
type User struct {
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	Age       int        `json:"age"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// IsActive returns true if the user is active and not soft deleted.
func (u User) IsActive() bool {
	return u.Status == "active" && u.DeletedAt == nil
}

// UserRequest represents the query parameters for filtering users.
type UserRequest struct {
	Status string
	MinAge int
	MaxAge int
	Search string
	Limit  int
	Offset int
}
