package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
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
