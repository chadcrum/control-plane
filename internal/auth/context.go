package auth

import (
	"context"
)

type contextKey int

const (
	actorInfoKey contextKey = iota
	preferredUsernameKey
)

type ActorInfo struct {
	ActorID   string
	ActorType string
}

func WithActorInfo(ctx context.Context, info ActorInfo) context.Context {
	return context.WithValue(ctx, actorInfoKey, info)
}

func ActorInfoFromContext(ctx context.Context) (ActorInfo, bool) {
	info, ok := ctx.Value(actorInfoKey).(ActorInfo)
	return info, ok
}

func WithPreferredUsername(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, preferredUsernameKey, username)
}

func PreferredUsernameFromContext(ctx context.Context) string {
	s, _ := ctx.Value(preferredUsernameKey).(string)
	return s
}
