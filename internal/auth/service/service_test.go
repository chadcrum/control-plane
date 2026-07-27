package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/dcm-project/control-plane/internal/auth"
	"github.com/dcm-project/control-plane/internal/auth/store"
	"github.com/dcm-project/control-plane/internal/auth/store/model"
	"gorm.io/gorm"
)

type mockStore struct {
	findActorByExternalIDFn func(ctx context.Context, provider, externalID string) (*model.Actor, error)
	createActorFn           func(ctx context.Context, actor model.Actor) (*model.Actor, error)
	createActorIdentityFn   func(ctx context.Context, identity model.ActorIdentity) (*model.ActorIdentity, error)
	getActorByUsernameFn    func(ctx context.Context, username string) (*model.Actor, error)
	updateActorStatusFn     func(ctx context.Context, actorID string, status string) error
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

func (m *mockStore) UpdateActorStatus(ctx context.Context, actorID string, status string) error {
	if m.updateActorStatusFn != nil {
		return m.updateActorStatusFn(ctx, actorID, status)
	}
	return nil
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

func TestResolveActor_UsernameCollisionReturnsConflict(t *testing.T) {
	callCount := 0
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			callCount++
			if callCount == 1 {
				return nil, store.ErrIdentityNotFound
			}
			return nil, store.ErrIdentityNotFound
		},
		createActorFn: func(_ context.Context, _ model.Actor) (*model.Actor, error) {
			return nil, fmt.Errorf("create actor: UNIQUE constraint failed: actors.username")
		},
	}, "", slog.Default())

	ctx := auth.WithPreferredUsername(context.Background(), "alice")
	_, err := s.ResolveActor(ctx, "ext-different-user")
	if !errors.Is(err, auth.ErrUsernameConflict) {
		t.Fatalf("err = %v, want ErrUsernameConflict", err)
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
	if got := err.Error(); got == "" {
		t.Fatal("expected non-empty error message")
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

func TestResolveActor_UnrecognizedStatusTreatedAsActive(t *testing.T) {
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "actor-1", Type: model.ActorTypeHuman, Status: "unknown_garbage"}, nil
		},
	}, "", slog.Default())

	info, err := s.ResolveActor(context.Background(), "ext-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ActorID != "actor-1" {
		t.Fatalf("ActorID = %q, want %q", info.ActorID, "actor-1")
	}
}

func TestResolveActor_IdentityRaceUsesExistingActor(t *testing.T) {
	callCount := 0
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			callCount++
			if callCount == 1 {
				return nil, store.ErrIdentityNotFound
			}
			return &model.Actor{ID: "winner-actor", Type: model.ActorTypeHuman, Status: model.ActorStatusActive}, nil
		},
		createActorFn: func(_ context.Context, actor model.Actor) (*model.Actor, error) {
			return &actor, nil
		},
		createActorIdentityFn: func(_ context.Context, _ model.ActorIdentity) (*model.ActorIdentity, error) {
			return nil, fmt.Errorf("UNIQUE constraint failed: actor_identities.idx_provider_external")
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

func TestResolveActor_RaceRetryFailure(t *testing.T) {
	callCount := 0
	retryErr := errors.New("connection lost")
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			callCount++
			if callCount == 1 {
				return nil, store.ErrIdentityNotFound
			}
			return nil, retryErr
		},
		createActorFn: func(_ context.Context, _ model.Actor) (*model.Actor, error) {
			return nil, fmt.Errorf("UNIQUE constraint failed: actors.username")
		},
	}, "", slog.Default())

	_, err := s.ResolveActor(context.Background(), "ext-race")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, retryErr) {
		t.Fatalf("err should wrap retryErr, got: %v", err)
	}
}

func TestResolveActor_GormErrDuplicatedKeyTriggersRace(t *testing.T) {
	callCount := 0
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			callCount++
			if callCount == 1 {
				return nil, store.ErrIdentityNotFound
			}
			return &model.Actor{ID: "winner", Type: model.ActorTypeHuman, Status: model.ActorStatusActive}, nil
		},
		createActorFn: func(_ context.Context, _ model.Actor) (*model.Actor, error) {
			return nil, gorm.ErrDuplicatedKey
		},
	}, "", slog.Default())

	info, err := s.ResolveActor(context.Background(), "ext-race")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ActorID != "winner" {
		t.Fatalf("ActorID = %q, want %q", info.ActorID, "winner")
	}
}

func TestResolveActor_CreateIdentityNonUniqueError(t *testing.T) {
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return nil, store.ErrIdentityNotFound
		},
		createActorFn: func(_ context.Context, actor model.Actor) (*model.Actor, error) {
			return &actor, nil
		},
		createActorIdentityFn: func(_ context.Context, _ model.ActorIdentity) (*model.ActorIdentity, error) {
			return nil, errors.New("connection refused")
		},
	}, "", slog.Default())

	_, err := s.ResolveActor(context.Background(), "ext-fail")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := "provision actor: create identity: connection refused"
	if got := err.Error(); got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestSeed_SkipsWhenAdminSubjectEmpty(t *testing.T) {
	s := NewService(&mockStore{}, "", slog.Default())

	err := s.Seed(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSeed_CreatesAdminActorAndIdentity(t *testing.T) {
	var createdActor, createdIdentity bool
	s := NewService(&mockStore{
		getActorByUsernameFn: func(_ context.Context, _ string) (*model.Actor, error) {
			return nil, store.ErrActorNotFound
		},
		createActorFn: func(_ context.Context, actor model.Actor) (*model.Actor, error) {
			createdActor = true
			if actor.Username != "admin" {
				t.Errorf("username = %q, want %q", actor.Username, "admin")
			}
			return &actor, nil
		},
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return nil, store.ErrIdentityNotFound
		},
		createActorIdentityFn: func(_ context.Context, identity model.ActorIdentity) (*model.ActorIdentity, error) {
			createdIdentity = true
			if identity.ExternalID != "admin-ext-id" {
				t.Errorf("external id = %q, want %q", identity.ExternalID, "admin-ext-id")
			}
			if identity.AuthProvider != model.AuthProviderKeycloak {
				t.Errorf("auth provider = %q, want %q", identity.AuthProvider, model.AuthProviderKeycloak)
			}
			return &identity, nil
		},
	}, "admin-ext-id", slog.Default())

	err := s.Seed(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !createdActor {
		t.Fatal("expected CreateActor to be called")
	}
	if !createdIdentity {
		t.Fatal("expected CreateActorIdentity to be called")
	}
}

func TestSeed_IdempotentWhenBothExist(t *testing.T) {
	s := NewService(&mockStore{
		getActorByUsernameFn: func(_ context.Context, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "existing-admin"}, nil
		},
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "existing-admin"}, nil
		},
	}, "admin-ext-id", slog.Default())

	err := s.Seed(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSeed_ReusesExistingActorCreatesIdentity(t *testing.T) {
	var createdIdentity bool
	s := NewService(&mockStore{
		getActorByUsernameFn: func(_ context.Context, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "existing-admin"}, nil
		},
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return nil, store.ErrIdentityNotFound
		},
		createActorIdentityFn: func(_ context.Context, identity model.ActorIdentity) (*model.ActorIdentity, error) {
			createdIdentity = true
			if identity.ActorID != "existing-admin" {
				t.Errorf("actor id = %q, want %q", identity.ActorID, "existing-admin")
			}
			return &identity, nil
		},
	}, "admin-ext-id", slog.Default())

	err := s.Seed(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !createdIdentity {
		t.Fatal("expected CreateActorIdentity to be called")
	}
}

func TestSeed_ReactivatesSuspendedAdminActor(t *testing.T) {
	var reactivated bool
	s := NewService(&mockStore{
		getActorByUsernameFn: func(_ context.Context, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "admin-1", Status: model.ActorStatusSuspended}, nil
		},
		updateActorStatusFn: func(_ context.Context, actorID string, status string) error {
			if actorID != "admin-1" {
				t.Errorf("actorID = %q, want %q", actorID, "admin-1")
			}
			if status != model.ActorStatusActive {
				t.Errorf("status = %q, want %q", status, model.ActorStatusActive)
			}
			reactivated = true
			return nil
		},
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "admin-1"}, nil
		},
	}, "admin-ext-id", slog.Default())

	err := s.Seed(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reactivated {
		t.Fatal("expected UpdateActorStatus to be called")
	}
}

func TestSeed_RejectsIdentityBoundToDifferentActor(t *testing.T) {
	s := NewService(&mockStore{
		getActorByUsernameFn: func(_ context.Context, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "admin-1", Status: model.ActorStatusActive}, nil
		},
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "other-actor"}, nil
		},
	}, "admin-ext-id", slog.Default())

	err := s.Seed(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "already bound to actor") {
		t.Fatalf("error = %q, want it to mention actor mismatch", got)
	}
}

func TestSeed_PropagatesGetActorError(t *testing.T) {
	dbErr := errors.New("db connection lost")
	s := NewService(&mockStore{
		getActorByUsernameFn: func(_ context.Context, _ string) (*model.Actor, error) {
			return nil, dbErr
		},
	}, "admin-ext-id", slog.Default())

	err := s.Seed(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, dbErr) {
		t.Fatalf("err should wrap dbErr, got: %v", err)
	}
}

func TestSeed_PropagatesCreateActorError(t *testing.T) {
	createErr := errors.New("insert failed")
	s := NewService(&mockStore{
		getActorByUsernameFn: func(_ context.Context, _ string) (*model.Actor, error) {
			return nil, store.ErrActorNotFound
		},
		createActorFn: func(_ context.Context, _ model.Actor) (*model.Actor, error) {
			return nil, createErr
		},
	}, "admin-ext-id", slog.Default())

	err := s.Seed(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, createErr) {
		t.Fatalf("err should wrap createErr, got: %v", err)
	}
}

func TestSeed_PropagatesFindIdentityError(t *testing.T) {
	findErr := errors.New("identity lookup failed")
	s := NewService(&mockStore{
		getActorByUsernameFn: func(_ context.Context, _ string) (*model.Actor, error) {
			return &model.Actor{ID: "admin-1"}, nil
		},
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return nil, findErr
		},
	}, "admin-ext-id", slog.Default())

	err := s.Seed(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, findErr) {
		t.Fatalf("err should wrap findErr, got: %v", err)
	}
}

func TestSeed_PropagatesCreateIdentityError(t *testing.T) {
	createErr := errors.New("identity insert failed")
	s := NewService(&mockStore{
		getActorByUsernameFn: func(_ context.Context, _ string) (*model.Actor, error) {
			return nil, store.ErrActorNotFound
		},
		createActorFn: func(_ context.Context, actor model.Actor) (*model.Actor, error) {
			return &actor, nil
		},
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return nil, store.ErrIdentityNotFound
		},
		createActorIdentityFn: func(_ context.Context, _ model.ActorIdentity) (*model.ActorIdentity, error) {
			return nil, createErr
		},
	}, "admin-ext-id", slog.Default())

	err := s.Seed(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, createErr) {
		t.Fatalf("err should wrap createErr, got: %v", err)
	}
}

func TestResolveActor_JITProvisionServiceAccount(t *testing.T) {
	s := NewService(&mockStore{
		findActorByExternalIDFn: func(_ context.Context, _, _ string) (*model.Actor, error) {
			return nil, store.ErrIdentityNotFound
		},
		createActorFn: func(_ context.Context, actor model.Actor) (*model.Actor, error) {
			if actor.Type != model.ActorTypeServiceAccount {
				t.Errorf("actor type = %q, want %q", actor.Type, model.ActorTypeServiceAccount)
			}
			if actor.Username != "service-account-dcm-proxy" {
				t.Errorf("actor username = %q, want %q", actor.Username, "service-account-dcm-proxy")
			}
			return &actor, nil
		},
		createActorIdentityFn: func(_ context.Context, identity model.ActorIdentity) (*model.ActorIdentity, error) {
			return &identity, nil
		},
	}, "", slog.Default())

	ctx := auth.WithPreferredUsername(context.Background(), "service-account-dcm-proxy")
	info, err := s.ResolveActor(ctx, "ext-svc-uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ActorType != model.ActorTypeServiceAccount {
		t.Fatalf("ActorType = %q, want %q", info.ActorType, model.ActorTypeServiceAccount)
	}
}

func TestInferActorType(t *testing.T) {
	tests := []struct {
		username string
		want     string
	}{
		{"service-account-dcm-proxy", model.ActorTypeServiceAccount},
		{"service-account-", model.ActorTypeServiceAccount},
		{"service-account-x", model.ActorTypeServiceAccount},
		{"alice", model.ActorTypeHuman},
		{"", model.ActorTypeHuman},
		{"SERVICE-ACCOUNT-foo", model.ActorTypeHuman},
		{"svc-account-foo", model.ActorTypeHuman},
		{"service-account", model.ActorTypeHuman},
	}
	for _, tt := range tests {
		t.Run(tt.username, func(t *testing.T) {
			if got := inferActorType(tt.username); got != tt.want {
				t.Errorf("inferActorType(%q) = %q, want %q", tt.username, got, tt.want)
			}
		})
	}
}
