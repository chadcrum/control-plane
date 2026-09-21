package types

// Resource lifecycle statuses stored on placement resources.
const (
	ResourceStatusPending         = "PENDING"
	ResourceStatusProvisioning    = "PROVISIONING"
	ResourceStatusRunning         = "RUNNING"
	ResourceStatusDeleting        = "DELETING"
	ResourceStatusDeleted         = "DELETED"
	ResourceStatusFailed          = "FAILED"
	ResourceStatusPendingDeletion = "PENDING_DELETION"
)
