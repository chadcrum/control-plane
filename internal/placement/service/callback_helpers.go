package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dcm-project/control-plane/internal/placement/store"
	"github.com/dcm-project/control-plane/internal/placement/types"
)

// releaseProvisioningClaim rolls a create-progression claim back to PENDING.
// Placement uses PROVISIONING only as a short-lived CAS lock so duplicate
// RUNNING callbacks cannot double-dispatch SPRM
func releaseProvisioningClaim(ctx context.Context, resources store.Resource, resourceID string) error {
	applied, err := resources.UpdateStatusFrom(ctx, resourceID,
		[]string{types.ResourceStatusProvisioning},
		types.ResourceStatusPending,
	)
	if err != nil {
		return fmt.Errorf("release provisioning claim for %s: %w", resourceID, err)
	}
	if !applied {
		return fmt.Errorf("release provisioning claim for %s: status was not PROVISIONING", resourceID)
	}
	return nil
}

// releaseDeletionDispatch rolls a failed SPRM delete dispatch back to PENDING_DELETION.
func releaseDeletionDispatch(ctx context.Context, resources store.Resource, resourceID string) error {
	applied, err := resources.UpdateStatusFrom(ctx, resourceID,
		[]string{types.ResourceStatusDeleting},
		types.ResourceStatusPendingDeletion,
	)
	if err != nil {
		return fmt.Errorf("release deletion dispatch for %s: %w", resourceID, err)
	}
	if !applied {
		return fmt.Errorf("release deletion dispatch for %s: status was not DELETING", resourceID)
	}
	return nil
}

func logClaimRollbackFailure(log *slog.Logger, kind string, resourceID string, err error) {
	log.Warn("Failed to roll back placement claim after progression error",
		"claim_kind", kind,
		"resource_id", resourceID,
		"error", err,
	)
}
