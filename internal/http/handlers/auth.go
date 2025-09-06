package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go-demo/internal/auth"
	"go-demo/internal/authctx"
)

type Auth struct {
	S            *auth.Service
	Log          *slog.Logger
	MaxBodyBytes int64
}

func NewAuth(s *auth.Service, log *slog.Logger, maxBodyBytes int64) Auth {
	return Auth{S: s, Log: log, MaxBodyBytes: maxBodyBytes}
}

type RegisterReq struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CreateUserReq struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type UserResp struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	CreatedBy   string    `json:"created_by"`
	CreatedTime time.Time `json:"created_time"`
	UpdatedTime time.Time `json:"updated_time"`
	Role        string    `json:"role"`
}

// Register godoc
// @Summary Register user
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RegisterReq true "Register request"
// @Success 201 {object} UserResp
// @Failure 400 {object} ErrorEnvelope
// @Failure 409 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/auth/register [post]
func (h Auth) Register() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		dec := json.NewDecoder(io.LimitReader(r.Body, h.MaxBodyBytes))
		dec.DisallowUnknownFields()

		var req RegisterReq
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON payload")
			return
		}
		u, err := h.S.Register(r.Context(), req.Username, req.Email, req.Password, "self")
		if err != nil {
			switch err {
			case auth.ErrUserExists:
				writeError(w, http.StatusConflict, "user_exists", "username or email already exists")
				return
			default:
				writeError(w, http.StatusInternalServerError, "server_error", "could not register user")
				return
			}
		}

		resp := UserResp{
			ID:          u.ID,
			Username:    u.Username,
			Email:       u.Email,
			CreatedBy:   u.CreatedBy,
			CreatedTime: u.CreatedTime,
			UpdatedTime: u.UpdatedTime,
			Role:        u.Role,
		}
		writeJSON(w, http.StatusCreated, resp)
	})
}

type LoginReq struct {
	Identifier string `json:"identifier"` // username or email
	Password   string `json:"password"`
}

type LoginResp struct {
	Token            string    `json:"token"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	User             UserResp  `json:"user"`
}

// Login godoc
// @Summary Login
// @Description Login with username or email
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginReq true "Login request"
// @Success 200 {object} LoginResp
// @Failure 400 {object} ErrorEnvelope
// @Failure 401 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/auth/login [post]
func (h Auth) Login() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		dec := json.NewDecoder(io.LimitReader(r.Body, h.MaxBodyBytes))
		dec.DisallowUnknownFields()

		var req LoginReq
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON payload")
			return
		}

		u, tok, exp, rtok, rexp, err := h.S.Login(r.Context(), req.Identifier, req.Password)
		if err != nil {
			if err == auth.ErrInvalidCredentials {
				writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username/email or password")
				return
			}
			writeError(w, http.StatusInternalServerError, "server_error", "could not login")
			return
		}

		resp := LoginResp{
			Token:            tok,
			ExpiresAt:        exp,
			RefreshToken:     rtok,
			RefreshExpiresAt: rexp,
			User: UserResp{
				ID:          u.ID,
				Username:    u.Username,
				Email:       u.Email,
				CreatedBy:   u.CreatedBy,
				CreatedTime: u.CreatedTime,
				UpdatedTime: u.UpdatedTime,
				Role:        u.Role,
			},
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

// Me godoc
// @Summary Get current user
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} UserResp
// @Failure 401 {object} ErrorEnvelope
// @Router /v1/auth/me [get]
func (h Auth) Me() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		u, ok := authctx.UserFrom(r.Context())
		if !ok || u == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		resp := UserResp{
			ID:          u.ID,
			Username:    u.Username,
			Email:       u.Email,
			CreatedBy:   u.CreatedBy,
			CreatedTime: u.CreatedTime,
			UpdatedTime: u.UpdatedTime,
			Role:        u.Role,
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

type RefreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

type RefreshResp struct {
	Token            string    `json:"token"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	User             UserResp  `json:"user"`
}

// Refresh godoc
// @Summary Refresh access token
// @Description Exchange refresh token for a new access token (rotation)
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RefreshReq true "Refresh request"
// @Success 200 {object} RefreshResp
// @Failure 400 {object} ErrorEnvelope
// @Failure 401 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/auth/refresh [post]
func (h Auth) Refresh() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		dec := json.NewDecoder(io.LimitReader(r.Body, h.MaxBodyBytes))
		dec.DisallowUnknownFields()

		var req RefreshReq
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON payload")
			return
		}

		u, atok, aexp, rtok, rexp, err := h.S.Refresh(r.Context(), req.RefreshToken)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidCredentials) {
				writeError(w, http.StatusUnauthorized, "invalid_refresh", "invalid or expired refresh token")
				return
			}
			writeError(w, http.StatusInternalServerError, "server_error", "could not refresh token")
			return
		}

		resp := RefreshResp{
			Token:            atok,
			ExpiresAt:        aexp,
			RefreshToken:     rtok,
			RefreshExpiresAt: rexp,
			User: UserResp{
				ID:          u.ID,
				Username:    u.Username,
				Email:       u.Email,
				CreatedBy:   u.CreatedBy,
				CreatedTime: u.CreatedTime,
				UpdatedTime: u.UpdatedTime,
				Role:        u.Role,
			},
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

// CreateUser godoc
// @Summary Create user (Admin only)
// @Description Create a new user with specified role (ADMIN role required)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateUserReq true "Create user request"
// @Success 201 {object} UserResp
// @Failure 400 {object} ErrorEnvelope
// @Failure 401 {object} ErrorEnvelope
// @Failure 403 {object} ErrorEnvelope
// @Failure 409 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/admin/users [post]
func (h Auth) CreateUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		// Get the admin user from context
		adminUser, ok := authctx.UserFrom(r.Context())
		if !ok || adminUser == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}

		dec := json.NewDecoder(io.LimitReader(r.Body, h.MaxBodyBytes))
		dec.DisallowUnknownFields()

		var req CreateUserReq
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON payload")
			return
		}

		// Validate role - ADMIN cannot create other ADMIN users
		if req.Role == "ADMIN" {
			writeError(w, http.StatusBadRequest, "invalid_role", "cannot create ADMIN users")
			return
		}

		// Validate role is one of the allowed roles
		allowedRoles := []string{"USER", "ANALYZER", "MONITOR", "TEAM_LEADER"}
		validRole := false
		for _, role := range allowedRoles {
			if req.Role == role {
				validRole = true
				break
			}
		}
		if !validRole {
			writeError(w, http.StatusBadRequest, "invalid_role", "invalid role specified")
			return
		}

		u, err := h.S.CreateUser(r.Context(), req.Username, req.Email, req.Password, req.Role, adminUser.Username)
		if err != nil {
			switch err {
			case auth.ErrUserExists:
				writeError(w, http.StatusConflict, "user_exists", "username or email already exists")
				return
			default:
				h.Log.Error("create user failed", "err", err)
				writeError(w, http.StatusInternalServerError, "server_error", "could not create user")
				return
			}
		}

		resp := UserResp{
			ID:          u.ID,
			Username:    u.Username,
			Email:       u.Email,
			CreatedBy:   u.CreatedBy,
			CreatedTime: u.CreatedTime,
			UpdatedTime: u.UpdatedTime,
			Role:        u.Role,
		}
		writeJSON(w, http.StatusCreated, resp)
	})
}

type ListUsersResp struct {
	Users  []UserResp `json:"users"`
	Total  int64      `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

// ListUsers godoc
// @Summary List users (Admin only)
// @Description Get a paginated list of all users (ADMIN role required)
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param limit query int false "Number of users to return" default(20)
// @Param offset query int false "Number of users to skip" default(0)
// @Success 200 {object} ListUsersResp
// @Failure 400 {object} ErrorEnvelope
// @Failure 401 {object} ErrorEnvelope
// @Failure 403 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/admin/users [get]
func (h Auth) ListUsers() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get the admin user from context
		adminUser, ok := authctx.UserFrom(r.Context())
		if !ok || adminUser == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}

		// Parse query parameters
		limit := 20 // default
		offset := 0 // default

		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if parsedLimit, err := parsePositiveInt(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
				limit = parsedLimit
			}
		}

		if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
			if parsedOffset, err := parsePositiveInt(offsetStr); err == nil && parsedOffset >= 0 {
				offset = parsedOffset
			}
		}

		users, total, err := h.S.ListUsers(r.Context(), limit, offset)
		if err != nil {
			h.Log.Error("list users failed", "err", err)
			writeError(w, http.StatusInternalServerError, "server_error", "could not list users")
			return
		}

		userResps := make([]UserResp, len(users))
		for i, user := range users {
			userResps[i] = UserResp{
				ID:          user.ID,
				Username:    user.Username,
				Email:       user.Email,
				CreatedBy:   user.CreatedBy,
				CreatedTime: user.CreatedTime,
				UpdatedTime: user.UpdatedTime,
				Role:        user.Role,
			}
		}

		resp := ListUsersResp{
			Users:  userResps,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

type UpdateUserStatusReq struct {
	Active bool `json:"active"`
}

type UpdateUserRoleReq struct {
	Role string `json:"role"`
}

// UpdateUserStatus godoc
// @Summary Activate/Deactivate user (Admin only)
// @Description Activate or deactivate a user (ADMIN role required)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Param request body UpdateUserStatusReq true "Update status request"
// @Success 200 {object} UserResp
// @Failure 400 {object} ErrorEnvelope
// @Failure 401 {object} ErrorEnvelope
// @Failure 403 {object} ErrorEnvelope
// @Failure 404 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/admin/users/{id}/status [put]
func (h Auth) UpdateUserStatus() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		// Get the admin user from context
		adminUser, ok := authctx.UserFrom(r.Context())
		if !ok || adminUser == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}

		// Extract user ID from path
		userID := extractIDFromPath(r.URL.Path, "/v1/admin/users/", "/status")
		if userID == "" {
			writeError(w, http.StatusBadRequest, "invalid_path", "user ID is required")
			return
		}

		dec := json.NewDecoder(io.LimitReader(r.Body, h.MaxBodyBytes))
		dec.DisallowUnknownFields()

		var req UpdateUserStatusReq
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON payload")
			return
		}

		user, err := h.S.UpdateUserStatus(r.Context(), userID, req.Active, adminUser.Username)
		if err != nil {
			if err.Error() == "user not found" {
				writeError(w, http.StatusNotFound, "user_not_found", "user not found")
				return
			}
			if err.Error() == "cannot modify ADMIN user status" {
				writeError(w, http.StatusBadRequest, "invalid_operation", "cannot modify ADMIN user status")
				return
			}
			h.Log.Error("update user status failed", "err", err)
			writeError(w, http.StatusInternalServerError, "server_error", "could not update user status")
			return
		}

		resp := UserResp{
			ID:          user.ID,
			Username:    user.Username,
			Email:       user.Email,
			CreatedBy:   user.CreatedBy,
			CreatedTime: user.CreatedTime,
			UpdatedTime: user.UpdatedTime,
			Role:        user.Role,
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

// DeleteUser godoc
// @Summary Delete user (Admin only)
// @Description Soft delete a user (ADMIN role required)
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Success 204 "User deleted successfully"
// @Failure 400 {object} ErrorEnvelope
// @Failure 401 {object} ErrorEnvelope
// @Failure 403 {object} ErrorEnvelope
// @Failure 404 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/admin/users/{id} [delete]
func (h Auth) DeleteUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get the admin user from context
		adminUser, ok := authctx.UserFrom(r.Context())
		if !ok || adminUser == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}

		// Extract user ID from path
		userID := extractIDFromPath(r.URL.Path, "/v1/admin/users/", "")
		if userID == "" {
			writeError(w, http.StatusBadRequest, "invalid_path", "user ID is required")
			return
		}

		// Prevent admin from deleting themselves
		if userID == adminUser.ID {
			writeError(w, http.StatusBadRequest, "invalid_operation", "cannot delete your own account")
			return
		}

		err := h.S.DeleteUser(r.Context(), userID, adminUser.Username)
		if err != nil {
			if err.Error() == "user not found" {
				writeError(w, http.StatusNotFound, "user_not_found", "user not found")
				return
			}
			if err.Error() == "cannot delete ADMIN user" {
				writeError(w, http.StatusBadRequest, "invalid_operation", "cannot delete ADMIN user")
				return
			}
			h.Log.Error("delete user failed", "err", err)
			writeError(w, http.StatusInternalServerError, "server_error", "could not delete user")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// Helper functions
func parsePositiveInt(s string) (int, error) {
	var result int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid integer")
		}
		result = result*10 + int(r-'0')
	}
	return result, nil
}

func extractIDFromPath(path, prefix, suffix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	path = path[len(prefix):]
	if suffix != "" && strings.HasSuffix(path, suffix) {
		path = path[:len(path)-len(suffix)]
	}
	return path
}

// UpdateUserRole godoc
// @Summary Update user role (Admin only)
// @Description Update a user's role (ADMIN role required)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Param request body UpdateUserRoleReq true "Update role request"
// @Success 200 {object} UserResp
// @Failure 400 {object} ErrorEnvelope
// @Failure 401 {object} ErrorEnvelope
// @Failure 403 {object} ErrorEnvelope
// @Failure 404 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/admin/users/{id}/role [put]
func (h Auth) UpdateUserRole() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		// Get the admin user from context
		adminUser, ok := authctx.UserFrom(r.Context())
		if !ok || adminUser == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}

		// Extract user ID from path
		userID := extractIDFromPath(r.URL.Path, "/v1/admin/users/", "/role")
		if userID == "" {
			writeError(w, http.StatusBadRequest, "invalid_path", "user ID is required")
			return
		}

		dec := json.NewDecoder(io.LimitReader(r.Body, h.MaxBodyBytes))
		dec.DisallowUnknownFields()

		var req UpdateUserRoleReq
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON payload")
			return
		}

		// Validate role is one of the allowed roles
		allowedRoles := []string{"USER", "ANALYZER", "MONITOR", "TEAM_LEADER"}
		validRole := false
		for _, role := range allowedRoles {
			if req.Role == role {
				validRole = true
				break
			}
		}
		if !validRole {
			writeError(w, http.StatusBadRequest, "invalid_role", "invalid role specified")
			return
		}

		user, err := h.S.UpdateUserRole(r.Context(), userID, req.Role, adminUser.Username)
		if err != nil {
			if err.Error() == "user not found" {
				writeError(w, http.StatusNotFound, "user_not_found", "user not found")
				return
			}
			if err.Error() == "cannot modify ADMIN user role" {
				writeError(w, http.StatusBadRequest, "invalid_operation", "cannot modify ADMIN user role")
				return
			}
			if err.Error() == "cannot assign ADMIN role" {
				writeError(w, http.StatusBadRequest, "invalid_operation", "cannot assign ADMIN role")
				return
			}
			h.Log.Error("update user role failed", "err", err)
			writeError(w, http.StatusInternalServerError, "server_error", "could not update user role")
			return
		}

		resp := UserResp{
			ID:          user.ID,
			Username:    user.Username,
			Email:       user.Email,
			CreatedBy:   user.CreatedBy,
			CreatedTime: user.CreatedTime,
			UpdatedTime: user.UpdatedTime,
			Role:        user.Role,
		}
		writeJSON(w, http.StatusOK, resp)
	})
}
