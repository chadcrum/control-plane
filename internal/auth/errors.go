package auth

import "errors"

var (
	ErrActorSuspended   = errors.New("account suspended")
	ErrActorDeactivated = errors.New("account deactivated")
	ErrUsernameConflict = errors.New("username already in use")
)
