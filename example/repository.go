package main

import (
	"database/sql"
	"errors"
	"fmt"

	gl "github.com/mochams/glimt"
)

// UserRepository handles all user database operations.
type UserRepository struct {
	db       *sql.DB
	registry *gl.Registry
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(db *sql.DB, registry *gl.Registry) *UserRepository {
	return &UserRepository{db: db, registry: registry}
}

// Create inserts a new user into the database.
func (r *UserRepository) Create(u User) (User, error) {
	query, args := r.registry.MustGet("insertUser").Build()

	result, err := r.db.Exec(query, append(args, u.Name, u.Email, u.Age, u.Status)...)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("get last insert id: %w", err)
	}

	return r.GetByID(int(id))
}

// GetByID retrieves a single user by ID.
// Returns ErrUserNotFound if the user does not exist or has been soft deleted.
func (r *UserRepository) GetByID(id int) (User, error) {
	query, args := r.registry.MustGet("listUsers").
		Where(gl.Eq("id", id)).
		Where(gl.Null("deleted_at")).
		Limit(1).
		Build()

	row := r.db.QueryRow(query, args...)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound("user", id)
	}

	return u, err
}

// List retrieves users based on the given request filters.
func (r *UserRepository) List(req UserRequest) ([]User, error) {
	q := r.registry.MustGet("listUsers").
		Where(gl.Null("deleted_at"))

	if req.Status != "" {
		q.Where(gl.Eq("status", req.Status))
	}

	if req.MinAge > 0 {
		q.Where(gl.Gte("age", req.MinAge))
	}

	if req.MaxAge > 0 {
		q.Where(gl.Lte("age", req.MaxAge))
	}

	if req.Search != "" {
		q.Where(gl.Like("name", "%"+req.Search+"%"))
	}

	if req.Limit > 0 {
		q.Limit(req.Limit)
	}

	if req.Offset > 0 {
		q.Offset(req.Offset)
	}

	query, args := q.Build()

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// return empty slice instead of nil for clean JSON response — [] not null
	if users == nil {
		users = []User{}
	}

	return users, nil
}

// SoftDelete marks a user as deleted without removing them from the database.
func (r *UserRepository) SoftDelete(id int) error {
	query, args := r.registry.MustGet("softDeleteUser").Build()

	result, err := r.db.Exec(query, append(args, id)...)
	if err != nil {
		return fmt.Errorf("soft delete user: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}

	if affected == 0 {
		return ErrNotFound("user", id)
	}

	return nil
}

// Helpers

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(s scanner) (User, error) {
	var u User
	err := s.Scan(
		&u.ID,
		&u.Name,
		&u.Email,
		&u.Age,
		&u.Status,
		&u.CreatedAt,
		&u.DeletedAt,
	)
	if err != nil {
		return User{}, err
	}
	return u, nil
}
