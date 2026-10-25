package migrate

import (
	"entgo.io/ent/dialect/sql/schema"
	"entgo.io/ent/schema/field"
)

// History visibility has no foreign key: hiding must never change task or asset lifetime.
func init() {
	columns := []*schema.Column{
		{Name: "task_kind", Type: field.TypeString, Size: 16},
		{Name: "user_id", Type: field.TypeInt64},
		{Name: "task_id", Type: field.TypeUUID},
		{Name: "hidden_at", Type: field.TypeTime},
	}
	Tables = append(Tables, &schema.Table{Name: "task_history_hidden", Columns: columns, PrimaryKey: columns[:3]})
}
