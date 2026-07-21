// Package store defines the actor store interface and GORM implementation.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/dcm-project/control-plane/internal/auth/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrActorNotFound    = errors.New("actor not found")
	ErrIdentityNotFound = errors.New("identity not found")
)

type Store interface {
	FindActorByExternalID(ctx context.Context, provider, externalID string) (*model.Actor, error)
	CreateActor(ctx context.Context, actor model.Actor) (*model.Actor, error)
	CreateActorIdentity(ctx context.Context, identity model.ActorIdentity) (*model.ActorIdentity, error)
	GetActorByUsername(ctx context.Context, username string) (*model.Actor, error)
	UpdateActorStatus(ctx context.Context, actorID string, status string) error
	RunInTransaction(ctx context.Context, fn func(Store) error) error
}

type DataStore struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) Store {
	return &DataStore{db: db}
}

func (s *DataStore) FindActorByExternalID(ctx context.Context, provider, externalID string) (*model.Actor, error) {
	var identity model.ActorIdentity
	err := s.db.WithContext(ctx).
		Where("auth_provider = ? AND external_id = ?", provider, externalID).
		First(&identity).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIdentityNotFound
		}
		return nil, fmt.Errorf("find identity: %w", err)
	}

	var actor model.Actor
	err = s.db.WithContext(ctx).Where("id = ?", identity.ActorID).First(&actor).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrActorNotFound
		}
		return nil, fmt.Errorf("find actor: %w", err)
	}

	return &actor, nil
}

func (s *DataStore) CreateActor(ctx context.Context, actor model.Actor) (*model.Actor, error) {
	if err := s.db.WithContext(ctx).Clauses(clause.Returning{}).Create(&actor).Error; err != nil {
		return nil, fmt.Errorf("create actor: %w", err)
	}
	return &actor, nil
}

func (s *DataStore) CreateActorIdentity(ctx context.Context, identity model.ActorIdentity) (*model.ActorIdentity, error) {
	if err := s.db.WithContext(ctx).Clauses(clause.Returning{}).Create(&identity).Error; err != nil {
		return nil, fmt.Errorf("create actor identity: %w", err)
	}
	return &identity, nil
}

func (s *DataStore) RunInTransaction(ctx context.Context, fn func(Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&DataStore{db: tx})
	})
}

func (s *DataStore) UpdateActorStatus(ctx context.Context, actorID string, status string) error {
	result := s.db.WithContext(ctx).Model(&model.Actor{}).Where("id = ?", actorID).Update("status", status)
	if result.Error != nil {
		return fmt.Errorf("update actor status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrActorNotFound
	}
	return nil
}

func (s *DataStore) GetActorByUsername(ctx context.Context, username string) (*model.Actor, error) {
	var actor model.Actor
	err := s.db.WithContext(ctx).Where("username = ?", username).First(&actor).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrActorNotFound
		}
		return nil, fmt.Errorf("get actor by username: %w", err)
	}
	return &actor, nil
}
