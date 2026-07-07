package auth

import (
	"context"
	"testing"
)

func TestActorInfoContext(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() context.Context
		wantInfo  ActorInfo
		wantFound bool
	}{
		{
			name: "round-trips through context",
			setup: func() context.Context {
				return WithActorInfo(context.Background(), ActorInfo{
					ActorID:   "user-42",
					ActorType: "user",
				})
			},
			wantInfo:  ActorInfo{ActorID: "user-42", ActorType: "user"},
			wantFound: true,
		},
		{
			name:      "empty context returns false",
			setup:     context.Background,
			wantInfo:  ActorInfo{},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			got, ok := ActorInfoFromContext(ctx)
			if ok != tt.wantFound {
				t.Fatalf("ActorInfoFromContext() ok = %v, want %v", ok, tt.wantFound)
			}
			if got != tt.wantInfo {
				t.Fatalf("ActorInfoFromContext() = %+v, want %+v", got, tt.wantInfo)
			}
		})
	}
}

func TestPreferredUsernameContext(t *testing.T) {
	t.Run("round-trips through context", func(t *testing.T) {
		ctx := WithPreferredUsername(context.Background(), "alice")
		got := PreferredUsernameFromContext(ctx)
		if got != "alice" {
			t.Fatalf("PreferredUsernameFromContext() = %q, want %q", got, "alice")
		}
	})

	t.Run("empty context returns empty string", func(t *testing.T) {
		got := PreferredUsernameFromContext(context.Background())
		if got != "" {
			t.Fatalf("PreferredUsernameFromContext() = %q, want %q", got, "")
		}
	})
}
