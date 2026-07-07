package auth

import "errors"

var (
	ErrActorSuspended   = errors.New("account suspended")
	ErrActorDeactivated = errors.New("account deactivated")
	ErrUnknownSubject   = errors.New("unknown subject")
)
