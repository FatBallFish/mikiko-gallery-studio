package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ClusterChallenge persists the public enrollment transcript and an encrypted
// server ephemeral key so replay protection survives API restarts.
type ClusterChallenge struct{ ent.Schema }

func (ClusterChallenge) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (ClusterChallenge) Fields() []ent.Field {
	return []ent.Field{
		field.String("challenge_id").MaxLen(128).NotEmpty().Match(stableIdentifierPattern).Immutable().Unique(),
		field.String("installation_id").MaxLen(128).NotEmpty().Match(stableIdentifierPattern).Immutable(),
		field.String("token_id").MaxLen(128).NotEmpty().Match(stableIdentifierPattern).Immutable(),
		field.Enum("role").Values("api", "worker", "web").Immutable(),
		field.String("node_id").MaxLen(128).NotEmpty().Match(stableIdentifierPattern).Immutable(),
		field.String("node_public_key").MaxLen(43).MinLen(43).Match(base64URL32Pattern).Immutable(),
		field.String("server_public_key").MaxLen(43).MinLen(43).Match(base64URL32Pattern).Immutable(),
		field.String("server_nonce").MaxLen(43).MinLen(43).Match(base64URL32Pattern).Immutable(),
		field.String("app_version").MaxLen(128).NotEmpty().Immutable(),
		field.Int("runtime_schema_version").Positive().Immutable(),
		field.Int64("config_revision").Positive().Immutable(),
		field.String("sealed_server_private_key").MaxLen(512).NotEmpty().Immutable().Sensitive(),
		field.Time("expires_at").Immutable(),
		field.Time("consumed_at").Optional().Nillable(),
	}
}

func (ClusterChallenge) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("installation_id", "token_id"),
		index.Fields("expires_at"),
		index.Fields("consumed_at"),
	}
}

func (ClusterChallenge) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "cluster_challenges"}}
}
