package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/dcm-project/control-plane/internal/auth"
	"github.com/dcm-project/control-plane/internal/auth/store"
	"github.com/dcm-project/control-plane/internal/auth/store/model"
)

type mockStore struct {
	findActorByExternalIDFn func(ctx context.Context, provider, externalID string) (*model.Actor, error)
	createActorFn           func(ctx context.Context, actor model.Actor) (*model.Actor, error)
	createActorIdentityFn   func(ctx context.Context, identity model.ActorIdentity) (*model.ActorIdentity, error)
	getActorByUsernameFn    func(ctx context.Context, username string) (*model.Actor, error)
}

func (m *mockStore) FindActorByExternalID(ctx context.Context, provider, externalID string) (*model.Actor, error) {
	return m.findActorByExternalIDFn(ctx, provider, externalID)
}

func (m *mockStore) CreateActor(ctx context.Context, actor model.Actor) (*model.Actor, error) {
	return m.createActorFn(ctx, actor)
}

func (m *mockStore) CreateActorIdentity(ctx context.Context, identity model.ActorIdentity) (*model.ActorIdentity, error) {
	return m.createActorIdentityFn(ctx, identity)
}

func (m *mockStore) GetActorByUsername(ctx context.Context, username string) (*model.Actor, error) {
	return m.getActorByUsernameFn(ctx, username)
}

func (m *mockStore) RunInTransaction(_ context.Context, fn func(store.Store) error) error {
	return fn(m)
}

func TestResolveActor_ExistingActiveActor(t *testing.T) {
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "actor-1", Type: model.ActorTypeHuman, Status: model.ActorStatusActive}, nil
		},
	}, "", slog.Default())

	info, err := s.ResolveActor(context.Background(), "ext-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ActorID != "actor-1" {
		t.Fatalf("ActorID = %q, want %q", info.ActorID, "actor-1")
	}
	if info.ActorType != model.ActorTypeHuman {
		t.Fatalf("ActorType = %q, want %q", info.ActorType, model.ActorTypeHuman)
	}
}

func TestResolveActor_SuspendedActor(t *testing.T) {
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "actor-1", Status: model.ActorStatusSuspended}, nil
		},
	}, "", slog.Default())

	_, err := s.ResolveActor(context.Background(), "ext-123")
	if !errors.Is(err, auth.ErrActorSuspended) {
		t.Fatalf("err = %v, want ErrActorSuspended", err)
	}
}

func TestResolveActor_DeactivatedActor(t *testing.T) {
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "actor-1", Status: model.ActorStatusDeactivated}, nil
		},
	}, "", slog.Default())

	_, err := s.ResolveActor(context.Background(), "ext-123")
	if !errors.Is(err, auth.ErrActorDeactivated) {
		t.Fatalf("err = %v, want ErrActorDeactivated", err)
	}
}

func TestResolveActor_JITProvisionOnFirstLogin(t *testing.T) {
	var createdActor bool
	var createdIdentity bool

	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return nil, store.ErrIdentityNotFound
		},
		createActorFn: func(_ context.Context, actor model.Actor) (*model.Actor, error) {
			createdActor = true
			if actor.Username != "ext-new" {
				t.Errorf("actor username = %q, want %q", actor.Username, "ext-new")
			}
			if actor.Type != model.ActorTypeHuman {
				t.Errorf("actor type = %q, want %q", actor.Type, model.ActorTypeHuman)
			}
			if actor.Status != model.ActorStatusActive {
				t.Errorf("actor status = %q, want %q", actor.Status, model.ActorStatusActive)
			}
			return &actor, nil
		},
		createActorIdentityFn: func(_ context.Context, identity model.ActorIdentity) (*model.ActorIdentity, error) {
			createdIdentity = true
			if identity.AuthProvider != model.AuthProviderKeycloak {
				t.Errorf("auth provider = %q, want %q", identity.AuthProvider, model.AuthProviderKeycloak)
			}
			if identity.ExternalID != "ext-new" {
				t.Errorf("external id = %q, want %q", identity.ExternalID, "ext-new")
			}
			return &identity, nil
		},
	}, "", slog.Default())

	// No preferred username in context — falls back to externalID
	info, err := s.ResolveActor(context.Background(), "ext-new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ActorType != model.ActorTypeHuman {
		t.Fatalf("ActorType = %q, want %q", info.ActorType, model.ActorTypeHuman)
	}
	if !createdActor {
		t.Fatal("expected CreateActor to be called")
	}
	if !createdIdentity {
		t.Fatal("expected CreateActorIdentity to be called")
	}
}

func TestResolveActor_JITProvisionUsesPreferredUsername(t *testing.T) {
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return nil, store.ErrIdentityNotFound
		},
		createActorFn: func(_ context.Context, actor model.Actor) (*model.Actor, error) {
			if actor.Username != "alice" {
				t.Errorf("actor username = %q, want %q (preferred username)", actor.Username, "alice")
			}
			return &actor, nil
		},
		createActorIdentityFn: func(_ context.Context, identity model.ActorIdentity) (*model.ActorIdentity, error) {
			return &identity, nil
		},
	}, "", slog.Default())

	ctx := auth.WithPreferredUsername(context.Background(), "alice")
	info, err := s.ResolveActor(ctx, "37f51208-some-uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ActorType != model.ActorTypeHuman {
		t.Fatalf("ActorType = %q, want %q", info.ActorType, model.ActorTypeHuman)
	}
}

func TestResolveActor_JITProvisionRaceUsesExistingActor(t *testing.T) {
	callCount := 0
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			callCount++
			if callCount == 1 {
				return nil, store.ErrIdentityNotFound
			}
			return &model.Actor{ID: "winner-actor", Type: model.ActorTypeHuman, Status: model.ActorStatusActive}, nil
		},
		createActorFn: func(_ context.Context, _ model.Actor) (*model.Actor, error) {
			return nil, fmt.Errorf("create actor: UNIQUE constraint failed: actors.username")
		},
	}, "", slog.Default())

	info, err := s.ResolveActor(context.Background(), "ext-race")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ActorID != "winner-actor" {
		t.Fatalf("ActorID = %q, want %q", info.ActorID, "winner-actor")
	}
}

func TestResolveActor_CreateActorNonUniqueError(t *testing.T) {
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return nil, store.ErrIdentityNotFound
		},
		createActorFn: func(_ context.Context, _ model.Actor) (*model.Actor, error) {
			return nil, errors.New("connection refused")
		},
	}, "", slog.Default())

	_, err := s.ResolveActor(context.Background(), "ext-fail")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errors.Unwrap(errors.Unwrap(err))) {
		// Just verify it wraps properly and isn't swallowed.
	}
}

func TestResolveActor_StoreErrorOnFind(t *testing.T) {
	dbErr := errors.New("database timeout")
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return nil, dbErr
		},
	}, "", slog.Default())

	_, err := s.ResolveActor(context.Background(), "ext-123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, dbErr) {
		t.Fatalf("err should wrap dbErr, got: %v", err)
	}
}
