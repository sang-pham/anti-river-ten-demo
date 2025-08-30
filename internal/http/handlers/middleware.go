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