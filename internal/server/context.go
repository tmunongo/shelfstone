package server

import (
	"context"

	"github.com/tmunongo/shelfstone/internal/models"
)

type contextKey string

const userContextKey contextKey = "user"

func withUser(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

func userFromContext(ctx context.Context) *models.User {
	u, _ := ctx.Value(userContextKey).(*models.User)
	return u
}
