// Package service implements actor resolution and JIT provisioning.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dcm-project/control-plane/internal/auth"
	"github.com/dcm-project/control-plane/internal/auth/store"
	"github.com/dcm-project/control-plane/internal/auth/store/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	store        store.Store
	adminSubject string
	logger       *slog.Logger
}

func NewService(s store.Store, adminSubject string, logger *slog.Logger) *Service {
	return &Service{
		store:        s,
		adminSubject: adminSubject,
		logger:       logger.With("component", "auth"),
	}
}

func (s *Service) ResolveActor(ctx context.Context, externalID string) (*auth.ActorInfo, error) {
	actor, err := s.store.FindActorByExternalID(ctx, model.AuthProviderKeycloak, externalID)
	if err != nil {
		if errors.Is(err, store.ErrIdentityNotFound) {
			return s.provisionActor(ctx, externalID)
		}
		return nil, fmt.Errorf("resolve actor: %w", err)
	}

	switch actor.Status {
	case model.ActorStatusSuspended:
		return nil, auth.ErrActorSuspended
	case model.ActorStatusDeactivated:
		return nil, auth.ErrActorDeactivated
	}

	return &auth.ActorInfo{
		ActorID:   actor.ID,
		ActorType: actor.Type,
	}, nil
}

var errProvisionRace = errors.New("provision race")

func (s *Service) provisionActor(ctx context.Context, externalID string) (*auth.ActorInfo, error) {
	username := auth.PreferredUsernameFromContext(ctx)
	if username == "" {
		username = externalID
	}

	var actorID string
	err := s.store.RunInTransaction(ctx, func(txStore store.Store) error {
		actor, err := txStore.CreateActor(ctx, model.Actor{
			ID:       uuid.New().String(),
			Username: username,
			Type:     model.ActorTypeHuman,
			Status:   model.ActorStatusActive,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return errProvisionRace
			}
			return fmt.Errorf("create actor: %w", err)
		}
		actorID = actor.ID

		_, err = txStore.CreateActorIdentity(ctx, model.ActorIdentity{
			ID:           uuid.New().String(),
			ActorID:      actor.ID,
			AuthProvider: model.AuthProviderKeycloak,
			ExternalID:   externalID,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return errProvisionRace
			}
			return fmt.Errorf("create identity: %w", err)
		}
		return nil
	})
	if errors.Is(err, errProvisionRace) {
		actor, findErr := s.store.FindActorByExternalID(ctx, model.AuthProviderKeycloak, externalID)
		if findErr != nil {
			if errors.Is(findErr, store.ErrIdentityNotFound) {
				s.logger.Warn("Username collision during JIT provisioning", "externalID", externalID, "username", username)
				return nil, auth.ErrUsernameConflict
			}
			return nil, fmt.Errorf("lookup after provision race: %w", findErr)
		}
		return &auth.ActorInfo{ActorID: actor.ID, ActorType: actor.Type}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("provision actor: %w", err)
	}

	s.logger.Info("Provisioned new actor on first login", "actorId", actorID)
	return &auth.ActorInfo{
		ActorID:   actorID,
		ActorType: model.ActorTypeHuman,
	}, nil
}

// Seed creates the admin actor (if DCM_ADMIN_SUBJECT is set) with a
// Keycloak identity binding. Both operations run in a single transaction
// so a partial failure (actor created, identity not) cannot occur.
func (s *Service) Seed(ctx context.Context) error {
	if s.adminSubject == "" {
		s.logger.Info("DCM_ADMIN_SUBJECT not set, skipping admin actor creation")
		return nil
	}

	err := s.store.RunInTransaction(ctx, func(txStore store.Store) error {
		actor, err := s.seedActor(ctx, txStore)
		if err != nil {
			return err
		}
		return s.seedIdentity(ctx, txStore, actor.ID)
	})
	if err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}
	return nil
}

func (s *Service) seedActor(ctx context.Context, txStore store.Store) (*model.Actor, error) {
	existing, err := txStore.GetActorByUsername(ctx, "admin")
	if err == nil {
		if existing.Status != model.ActorStatusActive {
			s.logger.Warn("Admin actor is not active, reactivating", "actorId", existing.ID, "previousStatus", existing.Status)
			existing.Status = model.ActorStatusActive
			if updateErr := txStore.UpdateActorStatus(ctx, existing.ID, model.ActorStatusActive); updateErr != nil {
				return nil, fmt.Errorf("reactivate admin actor: %w", updateErr)
			}
		}
		s.logger.Info("Admin actor already exists, reusing", "actorId", existing.ID)
		return existing, nil
	}
	if !errors.Is(err, store.ErrActorNotFound) {
		return nil, fmt.Errorf("check admin actor: %w", err)
	}

	actor, err := txStore.CreateActor(ctx, model.Actor{
		ID:       uuid.New().String(),
		Username: "admin",
		Type:     model.ActorTypeHuman,
		Status:   model.ActorStatusActive,
	})
	if err != nil {
		return nil, fmt.Errorf("create admin actor: %w", err)
	}
	return actor, nil
}

func (s *Service) seedIdentity(ctx context.Context, txStore store.Store, actorID string) error {
	existing, err := txStore.FindActorByExternalID(ctx, model.AuthProviderKeycloak, s.adminSubject)
	if err == nil {
		if existing.ID != actorID {
			return fmt.Errorf("admin subject %q is already bound to actor %q, expected %q", s.adminSubject, existing.ID, actorID)
		}
		s.logger.Info("Admin identity already exists, skipping")
		return nil
	}
	if !errors.Is(err, store.ErrIdentityNotFound) {
		return fmt.Errorf("check admin identity: %w", err)
	}

	_, err = txStore.CreateActorIdentity(ctx, model.ActorIdentity{
		ID:           uuid.New().String(),
		ActorID:      actorID,
		AuthProvider: model.AuthProviderKeycloak,
		ExternalID:   s.adminSubject,
	})
	if err != nil {
		return fmt.Errorf("create admin identity: %w", err)
	}
	s.logger.Info("Created admin actor with keycloak identity", "actorId", actorID)
	return nil
}

func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	// Fallback for drivers that don't map to gorm.ErrDuplicatedKey:
	// PG: "duplicate key value violates unique constraint"
	// SQLite: "UNIQUE constraint failed"
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key")
}
