package handlers

import (
	"net/http"
	"strings"

	"go-demo/internal/auth"
	"go-demo/internal/authctx"
)

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// RequireAuth returns a middleware that verifies the Bearer token,
// loads the user, and injects it into request context.
func RequireAuth(s *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := bearerToken(r)
			if tok == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
				return
			}
			sub, err := s.ParseToken(tok)
			if err != nil || sub == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
				return
			}
			u, err := s.GetUserByID(r.Context(), sub)
			if err != nil || u == nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "user not found")
				return
			}
			ctx := authctx.WithUser(r.Context(), u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdminRole returns a middleware that requires the user to have ADMIN role.
// This middleware should be used after RequireAuth middleware.
func RequireAdminRole() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := authctx.UserFrom(r.Context())
			if !ok || u == nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}
			if u.Role != "ADMIN" {
				writeError(w, http.StatusForbidden, "forbidden", "admin role required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRoles allows any of the provided roles. Use after RequireAuth.
func RequireRoles(roles ...http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := authctx.UserFrom(r.Context())
		if !ok || u == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		// Check if user role is in allowed list (case-insensitive)
		role := u.Role
		allowed := false
		for _, want := range []string{"ADMIN", "TEAM_LEADER"} {
			if strings.EqualFold(role, want) {
				allowed = true
				break
			}
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "forbidden", "insufficient role")
			return
		}
		// Pass through
		wrapped := roles
		if len(wrapped) == 0 {
			// No-op if no inner handlers were provided
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}).ServeHTTP(w, r)
			return
		}
		// If roles slice contains a single handler, treat as normal next
		if len(wrapped) == 1 {
			wrapped[0].ServeHTTP(w, r)
			return
		}
		// Chain any provided handlers in order
		var h http.Handler = wrapped[len(wrapped)-1]
		for i := len(wrapped) - 2; i >= 0; i-- {
			cur := wrapped[i]
			prev := h
			h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				cur.ServeHTTP(w, r)
				prev.ServeHTTP(w, r)
			})
		}
		h.ServeHTTP(w, r)
	})
}

// RequireAnyRole returns a middleware that allows any of the provided roles.
// Must be used after RequireAuth, similar to RequireAdminRole.
// Example: handlers.RequireAuth(authSvc)(handlers.RequireAnyRole("ADMIN","TEAM_LEADER")(h))
func RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := authctx.UserFrom(r.Context())
			if !ok || u == nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}
			role := strings.TrimSpace(u.Role)
			for _, want := range roles {
				if strings.EqualFold(role, strings.TrimSpace(want)) {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeError(w, http.StatusForbidden, "forbidden", "insufficient role")
		})
	}
}
