package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ObjectReconcileCheckpoint struct{ ent.Schema }

func (ObjectReconcileCheckpoint) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (ObjectReconcileCheckpoint) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("storage_identity").MaxLen(255).NotEmpty(),
		field.String("namespace").MaxLen(80).NotEmpty(),
		field.String("prefix").MaxLen(64).NotEmpty(),
		field.Text("cursor").Default(""),
		field.Int64("generation").Default(0),
	}
}

func (ObjectReconcileCheckpoint) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("storage_identity", "prefix").Unique(),
		index.Fields("generation", "updated_at"),
	}
}

func (ObjectReconcileCheckpoint) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "object_reconcile_checkpoints"}}
}
