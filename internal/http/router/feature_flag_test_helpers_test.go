package router

import (
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
)

func enabledFeatureAdmin(t *testing.T, keys ...string) *adminconfigservice.Service {
	t.Helper()
	service := adminconfigservice.NewService(config.Config{})
	tab, err := service.GetTab(t.Context(), "site")
	if err != nil {
		t.Fatal(err)
	}
	items := make([]domainadminconfig.Item, 0, len(keys))
	for _, key := range keys {
		items = append(items, domainadminconfig.Item{ConfigCategory: "features", ConfigKey: key, ConfigValue: map[string]any{"value": true}, Scope: "global"})
	}
	if _, err := service.UpdateTab(t.Context(), domainadminconfig.UpdateTabRequest{TabKey: "site", Version: tab.Version, Items: items}); err != nil {
		t.Fatal(err)
	}
	return service
}
