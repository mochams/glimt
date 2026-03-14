package main

import "fmt"

// NotFoundError represents a resource not found error.
type NotFoundError struct {
	Resource string
	ID       int
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("%s with id %d not found", e.Resource, e.ID)
}

// ErrNotFound returns a NotFoundError for a specific resource.
func ErrNotFound(resource string, id int) NotFoundError {
	return NotFoundError{Resource: resource, ID: id}
}
