package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ClusterNode stores only runtime identity and operational health metadata.
type ClusterNode struct{ ent.Schema }

func (ClusterNode) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (ClusterNode) Fields() []ent.Field {
	return []ent.Field{
		field.String("node_id").MaxLen(128).NotEmpty().Match(stableIdentifierPattern).Immutable().Unique(),
		field.String("installation_id").MaxLen(128).NotEmpty().Match(stableIdentifierPattern).Immutable(),
		field.Enum("role").Values("single", "control", "api", "worker", "web"),
		field.String("app_version").MaxLen(128).NotEmpty(),
		field.Int("runtime_schema_version").Positive(),
		field.Int64("config_revision").NonNegative(),
		field.Enum("health").Values("joining", "healthy", "degraded", "unready", "offline").Default("joining"),
		field.String("last_error").MaxLen(1024).Default(""),
		field.Time("last_heartbeat_at").Optional().Nillable(),
	}
}

func (ClusterNode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("installation_id", "role"),
		index.Fields("health"),
		index.Fields("last_heartbeat_at"),
	}
}

func (ClusterNode) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "cluster_nodes"}}
}
