package deals

import (
	"context"
	"time"

	"uuid"
)

// ApproveOffer is how the team approves an offer for a niche, with the hook
// of its post: a use case of the project's own, on the generated ones.
func (d *Deals) ApproveOffer(ctx context.Context, id, niche uuid.UUID, hook string) (Offer, error) {
	approved := OfferStatusApproved
	now := time.Now()
	return d.PatchOffer(ctx, id, 0, OfferChanges{Status: &approved, NicheID: &niche, Hook: &hook, ApprovedAt: &now}, nil)
}
