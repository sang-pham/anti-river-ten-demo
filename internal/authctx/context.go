package authctx

import (
	"context"

	"go-demo/internal/db"
)

type ctxKey int

const userKey ctxKey = iota

// WithUser stores the authenticated user in the context.
func WithUser(ctx context.Context, u *db.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// UserFrom retrieves the authenticated user from the context.
func UserFrom(ctx context.Context) (*db.User, bool) {
	v := ctx.Value(userKey)
	if v == nil {
		return nil, false
	}
	u, ok := v.(*db.User)
	return u, ok
}