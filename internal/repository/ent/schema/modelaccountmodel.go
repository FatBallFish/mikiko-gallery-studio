package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelAccountModel struct{ ent.Schema }

func (ModelAccountModel) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (ModelAccountModel) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.String("model_code").MaxLen(128).NotEmpty(),
		field.String("display_name").MaxLen(128).Default(""),
		field.JSON("task_types", []string{}).Optional(),
		field.JSON("base_resolution", []string{}).Optional(),
		field.JSON("quality", []string{"auto"}).Optional(),
		field.Int("max_reference_image_count").Default(0),
		field.Int("max_image_count").Default(1).Range(1, 10),
		field.JSON("size_modes", []string{"ratio"}).Optional(),
		field.JSON("supported_ratios", []string{"1:1"}).Optional(),
		field.JSON("supported_pixel_sizes", []string{"1024x1024"}).Optional(),
		field.Bool("supports_custom_ratio").Default(false),
		field.JSON("output_format", []string{"png"}).Optional(),
		field.JSON("supported_backgrounds", []string{}).Optional(),
		field.Int("output_compression").Default(100),
		field.Bool("supports_output_compression").Default(false),
		field.Bool("supports_custom_size").Default(false),
		field.Int("min_width").Default(256),
		field.Int("max_width").Default(3840),
		field.Int("min_height").Default(256),
		field.Int("max_height").Default(3840),
		field.JSON("moderation", []string{"auto"}).Optional(),
		field.String("cost_per_image").SchemaType(map[string]string{dialect.Postgres: "numeric(18,5)"}).Default("0.00000"),
		field.String("currency").MaxLen(16).Default("USD"),
		field.Bool("enabled").Default(true),
		field.JSON("extra", map[string]any{}).Optional(),
	}
}

func (ModelAccountModel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id"),
		index.Fields("account_id", "model_code").Unique(),
		index.Fields("enabled"),
	}
}

func (ModelAccountModel) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "model_account_models"}}
}
