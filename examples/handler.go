package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

// UserHandler handles HTTP requests for user resources.
type UserHandler struct {
	repo *UserRepository
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(repo *UserRepository) *UserHandler {
	return &UserHandler{repo: repo}
}

// List handles GET /users
// Supports optional query parameters:
//
//	status  — filter by status (active, inactive)
//	min_age — filter by minimum age
//	max_age — filter by maximum age
//	search  — filter by name (partial match)
//	limit   — number of results per page (default 20)
//	offset  — number of results to skip (default 0)
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	params := FromRequest(r)

	req := UserRequest{
		Status: params.String("status", ""),
		MinAge: params.Int("min_age", 0),
		MaxAge: params.Int("max_age", 0),
		Search: params.String("search", ""),
		Limit:  params.Int("limit", 20),
		Offset: params.Int("offset", 0),
	}

	if !params.Valid() {
		writeValidationError(w, params.Errors())
		return
	}

	users, err := h.repo.List(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	writeList(w, http.StatusOK, users, len(users))
}

// Get handles GET /users/{id}
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := pathParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	user, err := h.repo.GetByID(id)
	if err != nil {
		var notFound NotFoundError
		if errors.As(err, &notFound) {
			writeError(w, http.StatusNotFound, notFound.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	writeData(w, http.StatusOK, user)
}

// Create handles POST /users
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var s CreateUserSerializer
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.Validate()
	if !s.Valid() {
		writeValidationError(w, s.Errors)
		return
	}

	user, err := h.repo.Create(s.ToModel())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	writeData(w, http.StatusCreated, user)
}

// Delete handles DELETE /users/{id}
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := pathParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := h.repo.SoftDelete(id); err != nil {
		var notFound NotFoundError
		if errors.As(err, &notFound) {
			writeError(w, http.StatusNotFound, notFound.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Helpers

// pathParamInt extracts an integer path parameter from the request.
func pathParamInt(r *http.Request, key string) (int, error) {
	v := r.PathValue(key)
	if v == "" {
		return 0, errors.New("missing path parameter: " + key)
	}
	return strconv.Atoi(v)
}
