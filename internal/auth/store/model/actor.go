// Package model defines the actor and identity persistence types.
package model

import (
	"time"
)

const (
	ActorTypeHuman          = "human"
	ActorTypeServiceAccount = "service_account"

	ActorStatusActive      = "active"
	ActorStatusSuspended   = "suspended"
	ActorStatusDeactivated = "deactivated"
)

type Actor struct {
	ID          string    `gorm:"column:id;primaryKey"`
	Username    string    `gorm:"column:username;not null;uniqueIndex"`
	Email       string    `gorm:"column:email"`
	DisplayName string    `gorm:"column:display_name"`
	Type        string    `gorm:"column:type;not null"`
	Status      string    `gorm:"column:status;not null;default:active"`
	CreateTime  time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime  time.Time `gorm:"column:update_time;autoUpdateTime"`
}
