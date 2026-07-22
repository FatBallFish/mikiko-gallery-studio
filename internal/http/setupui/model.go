package setupui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type Deployment struct {
	Mode                 string `json:"mode"`
	Profile              string `json:"profile"`
	Topology             string `json:"topology"`
	Role                 string `json:"role"`
	PostgresManaged      bool   `json:"postgres_managed"`
	RedisManaged         bool   `json:"redis_managed"`
	ObjectStorageManaged bool   `json:"object_storage_managed"`
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Field struct {
	Key           string   `json:"key"`
	Group         string   `json:"group"`
	DescriptionZH string   `json:"description_zh"`
	DescriptionEN string   `json:"description_en"`
	Example       string   `json:"example,omitempty"`
	Value         string   `json:"value,omitempty"`
	Owner         string   `json:"owner"`
	Input         string   `json:"input"`
	Secret        bool     `json:"secret"`
	Required      bool     `json:"required"`
	Managed       bool     `json:"managed"`
	ReadOnly      bool     `json:"read_only"`
	Options       []Option `json:"options,omitempty"`
}

type Model struct {
	SchemaVersion int        `json:"schema_version"`
	Deployment    Deployment `json:"deployment"`
	Fields        []Field    `json:"fields"`
}

func NewModel(schema config.RuntimeSchema, bootstrap config.BootstrapConfig) (Model, error) {
	if err := schema.Validate(); err != nil {
		return Model{}, fmt.Errorf("validate setup UI runtime schema: %w", err)
	}
	context := bootstrap.Deployment
	context.SetupCompleted = false
	if value := strings.TrimSpace(bootstrap.Values["STORAGE_DRIVER"]); value != "" {
		context.StorageDriver = value
	}
	requiredFields, err := config.RequiredRuntimeFields(schema, context)
	if err != nil {
		return Model{}, fmt.Errorf("resolve setup UI required fields: %w", err)
	}
	required := make(map[string]bool, len(requiredFields))
	for _, field := range requiredFields {
		required[field.Key] = true
	}

	model := Model{
		SchemaVersion: schema.Version,
		Deployment: Deployment{
			Mode: string(context.Mode), Profile: string(context.Profile), Topology: string(context.Topology), Role: string(context.Role),
			PostgresManaged: bootstrap.PostgresManaged, RedisManaged: bootstrap.RedisManaged,
			ObjectStorageManaged: bootstrap.ObjectStorageManaged,
		},
		Fields: make([]Field, 0, len(schema.Fields)),
	}
	for _, runtimeField := range schema.Fields {
		if runtimeField.Owner != config.FieldOwnerSetup {
			continue
		}
		managed := managedSetupField(bootstrap, runtimeField.Key)
		field := Field{
			Key: runtimeField.Key, Group: runtimeField.Group,
			DescriptionZH: runtimeField.DescriptionZH, DescriptionEN: runtimeField.DescriptionEN,
			Owner: string(runtimeField.Owner), Input: inputKind(runtimeField), Secret: runtimeField.Secret,
			Required: required[runtimeField.Key], Managed: managed, ReadOnly: managed,
		}
		if !runtimeField.Secret {
			field.Example = runtimeField.Example
			field.Value = bootstrap.Values[runtimeField.Key]
			if field.Value == "" {
				field.Value = runtimeField.DefaultValue
			}
		}
		if runtimeField.Key == "STORAGE_DRIVER" {
			field.Options = []Option{{Value: "local", Label: "local"}, {Value: "s3", Label: "s3"}}
		}
		model.Fields = append(model.Fields, field)
	}
	return model, nil
}

func (model Model) JSON() []byte {
	encoded, err := json.Marshal(model)
	if err != nil {
		return []byte("null")
	}
	return encoded
}

func inputKind(field config.RuntimeField) string {
	if field.Secret {
		return "password"
	}
	switch field.Key {
	case "STORAGE_DRIVER":
		return "select"
	case "STORAGE_SHARED_VOLUME", "STORAGE_S3_FORCE_PATH_STYLE":
		return "checkbox"
	case "DATABASE_MAX_OPEN_CONNS", "DATABASE_MAX_IDLE_CONNS":
		return "number"
	case "DATABASE_URL", "REDIS_URL", "STORAGE_PUBLIC_BASE_URL", "STORAGE_S3_ENDPOINT", "PUBLIC_API_URL":
		return "url"
	default:
		return "text"
	}
}

func managedSetupField(bootstrap config.BootstrapConfig, key string) bool {
	if bootstrap.PostgresManaged && key == "DATABASE_URL" {
		return true
	}
	if bootstrap.RedisManaged && key == "REDIS_URL" {
		return true
	}
	if !bootstrap.ObjectStorageManaged {
		return false
	}
	switch key {
	case "STORAGE_DRIVER", "STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION", "STORAGE_S3_BUCKET",
		"STORAGE_S3_ACCESS_KEY_ID", "STORAGE_S3_SECRET_ACCESS_KEY", "STORAGE_S3_FORCE_PATH_STYLE", "STORAGE_S3_PREFIX":
		return true
	default:
		return false
	}
}
