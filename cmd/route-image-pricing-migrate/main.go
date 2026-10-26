// route-image-pricing-migrate only plans/applies route sale config; it never runs schema migration.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("route-image-pricing-migrate", flag.ContinueOnError)
	adminAPI := flags.Bool("admin-api", false, "export full route PUT requests as JSON without sending or writing")
	apply := flags.Bool("apply", false, "apply route config only after backing up database")
	backedUp := flags.Bool("backup-confirmed", false, "confirm an operator database backup exists")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *adminAPI && *apply {
		return fmt.Errorf("--admin-api cannot be combined with --apply")
	}
	if *apply && !*backedUp {
		return fmt.Errorf("apply requires --backup-confirmed after operator backup")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client, err := db.OpenContext(ctx, url)
	if err != nil {
		return err
	}
	defer client.Close()
	plan, err := entstore.NewModelAdminStore(client).MigrateRouteImagePricing(ctx, *apply)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if *adminAPI {
		store := entstore.NewModelAdminStore(client)
		requests := make([]map[string]any, 0, len(plan))
		for _, item := range plan {
			if item.AlreadyApplied {
				continue
			}
			route, err := store.GetRouteModel(ctx, item.RouteModelID)
			if err != nil {
				return err
			}
			requests = append(requests, map[string]any{"method": "PUT", "path": fmt.Sprintf("/api/ops/admin/v1/route-models/%d", route.ID), "body": map[string]any{
				"code": route.Code, "name": route.Name, "description": route.Description, "visibility": route.Visibility, "media_type": route.MediaType, "enabled": route.Enabled, "sort_order": route.SortOrder, "group_ids": route.GroupIDs, "icon_key": route.IconKey, "icon_svg": route.IconSVG, "image_pricing": item.ImagePricing,
			}})
		}
		return encoder.Encode(requests)
	}
	return encoder.Encode(plan)
}
