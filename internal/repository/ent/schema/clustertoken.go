package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ClusterToken stores only a one-way token hash and non-secret audit metadata.
type ClusterToken struct{ ent.Schema }

func (ClusterToken) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (ClusterToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("token_id").MaxLen(128).NotEmpty().Match(stableIdentifierPattern).Immutable().Unique(),
		field.String("token_hash").MaxLen(64).MinLen(64).Match(sha256HexPattern).Immutable().Unique().Sensitive(),
		field.String("installation_id").MaxLen(128).NotEmpty().Match(stableIdentifierPattern).Immutable(),
		field.Enum("role").Values("api", "worker", "web"),
		field.Time("expires_at").Immutable(),
		field.Time("consumed_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
		field.String("audit_actor").MaxLen(128).NotEmpty(),
	}
}

func (ClusterToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("installation_id", "role"),
		index.Fields("expires_at"),
		index.Fields("consumed_at"),
	}
}

func (ClusterToken) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "cluster_tokens"}}
}
