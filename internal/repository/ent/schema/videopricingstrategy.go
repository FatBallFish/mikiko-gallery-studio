package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type VideoPricingStrategy struct{ ent.Schema }

func (VideoPricingStrategy) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (VideoPricingStrategy) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").MaxLen(64).NotEmpty(),
		field.String("name").MaxLen(128).NotEmpty(),
		field.String("gross_point_value_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.31250"),
		field.String("minimum_net_point_income_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.25260"),
		field.String("max_bonus_ratio").SchemaType(map[string]string{dialect.Postgres: "numeric(10,5)"}).Default("0.20000"),
		field.String("payment_fee_rate").SchemaType(map[string]string{dialect.Postgres: "numeric(10,5)"}).Default("0.03000"),
		field.String("target_margin_rate").SchemaType(map[string]string{dialect.Postgres: "numeric(10,5)"}).Default("0.25000"),
		field.String("provider_cost_buffer_rate").SchemaType(map[string]string{dialect.Postgres: "numeric(10,5)"}).Default("0.10000"),
		field.String("platform_fixed_cost_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.15000"),
		field.String("platform_output_second_cost_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.02000"),
		field.String("platform_reference_cost_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.03000"),
		field.String("platform_audio_fixed_cost_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("platform_audio_second_cost_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("exact_reserve_markup").SchemaType(map[string]string{dialect.Postgres: "numeric(10,5)"}).Default("1.00000"),
		field.String("metered_reserve_markup").SchemaType(map[string]string{dialect.Postgres: "numeric(10,5)"}).Default("1.15000"),
		field.Int("strategy_version").Default(1).Positive(),
		field.Bool("enabled").Default(false),
	}
}

func (VideoPricingStrategy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code", "strategy_version").Unique(),
		index.Fields("enabled"),
	}
}
