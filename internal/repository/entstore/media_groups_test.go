package entstore

import (
	"entgo.io/ent/dialect"
	"fmt"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
	"reflect"
	"testing"
	"time"
)

func TestMediaGroupsIncludeAllPagesAndScopeOwnerProject(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-groups-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err = client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := client.Project.Create().SetUserID(44).SetName("A").SetNameKey("a").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	q, err := client.Project.Create().SetUserID(44).SetName("B").SetNameKey("b").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 125; i++ {
		group := "常用"
		if i == 124 {
			group = "末页"
		}
		_, err = client.MediaAsset.Create().SetUserID(44).SetProjectID(p.ID).SetName(fmt.Sprint(i)).SetNameKey(fmt.Sprint(i)).SetMediaType("image").SetSourceType("local_upload").SetStatus("ready").SetStorageDriver("local").SetObjectKey(fmt.Sprint(i)).SetMimeType("image/png").SetFileSizeBytes(1).SetGroupName(group).Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}
	add := func(user int64, project uuid.UUID, group string, deleted bool) {
		b := client.MediaAsset.Create().SetUserID(user).SetProjectID(project).SetName(group).SetNameKey(group).SetMediaType("image").SetSourceType("local_upload").SetStatus("ready").SetStorageDriver("local").SetObjectKey(group).SetMimeType("image/png").SetFileSizeBytes(1).SetGroupName(group)
		if deleted {
			b.SetDeletedAt(time.Now())
		}
		if _, err := b.Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	add(99, p.ID, "其他用户", false)
	add(44, q.ID, "其他项目", false)
	add(44, p.ID, "已删除", true)
	got, err := NewMediaStore(client).ListAssetGroups(ctx, 44, &p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"常用", "末页"}) {
		t.Fatalf("groups=%v", got)
	}
}
