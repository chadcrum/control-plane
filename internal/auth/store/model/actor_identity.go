package model

import (
	"time"
)

const (
	AuthProviderKeycloak = "keycloak"
)

type ActorIdentity struct {
	ID           string    `gorm:"column:id;primaryKey"`
	ActorID      string    `gorm:"column:actor_id;not null;index;constraint:OnDelete:CASCADE"`
	AuthProvider string    `gorm:"column:auth_provider;not null;uniqueIndex:idx_provider_external"`
	ExternalID   string    `gorm:"column:external_id;not null;uniqueIndex:idx_provider_external"`
	CreateTime   time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime   time.Time `gorm:"column:update_time;autoUpdateTime"`
}
