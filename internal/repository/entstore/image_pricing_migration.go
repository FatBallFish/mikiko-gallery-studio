package entstore

import (
	"context"
	"fmt"

	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodel"
	modeladminservice "github.com/fatballfish/pic-gallery/internal/service/modeladmin"
)

// MigrateRouteImagePricing plans or applies only route image_pricing. Callers must
// back up the database before apply. No schema, account, task or legacy-price writes.
func (s *ModelAdminStore) MigrateRouteImagePricing(ctx context.Context, apply bool) ([]modeladminservice.ImagePricingMigration, error) {
	if !apply {
		snapshot, err := s.ModelRoutingConfig(ctx)
		if err != nil {
			return nil, err
		}
		return modeladminservice.PlanImagePricingMigration(snapshot)
	}
	return withModelAdminTx(ctx, s, func(store *ModelAdminStore) ([]modeladminservice.ImagePricingMigration, error) {
		// Serialize route configuration writers, then build the plan from current state.
		routes, err := store.client.RouteModel.Query().Where(routemodel.DeletedAtIsNil(), lockRouteModelRow()).All(ctx)
		if err != nil {
			return nil, err
		}
		_ = routes
		snapshot, err := store.ModelRoutingConfig(ctx)
		if err != nil {
			return nil, err
		}
		plan, err := modeladminservice.PlanImagePricingMigration(snapshot)
		if err != nil {
			return nil, err
		}
		for _, item := range plan {
			if item.AlreadyApplied {
				continue
			}
			if _, err := store.client.RouteModel.UpdateOneID(int(item.RouteModelID)).SetImagePricing(item.ImagePricing).Save(ctx); err != nil {
				return nil, fmt.Errorf("migrate route %d pricing: %w", item.RouteModelID, err)
			}
		}
		return plan, nil
	})
}
