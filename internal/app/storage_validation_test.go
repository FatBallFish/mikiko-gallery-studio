package app

import (
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestValidateStorageTopology(t *testing.T) {
	if err := validateStorageTopology(config.Config{
		App: config.AppConfig{Env: "local"},
		Database: config.DatabaseConfig{
			URL: "postgres://postgres@localhost:5432/pic_gallery?sslmode=disable",
		},
		Storage: config.StorageConfig{Driver: "local"},
	}); err != nil {
		t.Fatalf("expected local env to allow local storage, got %v", err)
	}

	if err := validateStorageTopology(config.Config{
		App:     config.AppConfig{Env: "prod"},
		Storage: config.StorageConfig{Driver: "local"},
	}); err == nil {
		t.Fatal("expected prod env without shared volume to be rejected")
	}

	if err := validateStorageTopology(config.Config{
		App: config.AppConfig{Env: "local"},
		Database: config.DatabaseConfig{
			URL: "postgres://postgres@db.internal:5432/pic_gallery?sslmode=disable",
		},
		Storage: config.StorageConfig{Driver: "local"},
	}); err == nil {
		t.Fatal("expected remote database topology to be rejected even if app.env falls back to local")
	}

	if err := validateStorageTopology(config.Config{
		App:     config.AppConfig{Env: "prod"},
		Storage: config.StorageConfig{Driver: "local", SharedVolume: true},
	}); err != nil {
		t.Fatalf("expected shared volume to allow local storage in non-local env, got %v", err)
	}

	if err := validateStorageTopology(config.Config{
		App:     config.AppConfig{Env: "prod"},
		Storage: config.StorageConfig{Driver: "s3"},
	}); err == nil {
		t.Fatal("expected incomplete s3 config to be rejected")
	}

	if err := validateStorageTopology(config.Config{
		App: config.AppConfig{Env: "prod"},
		Storage: config.StorageConfig{
			Driver: "s3",
			S3: config.StorageS3Config{
				Endpoint:        "http://minio.internal:9000",
				Region:          "us-east-1",
				Bucket:          "pic-gallery",
				AccessKeyID:     "access",
				SecretAccessKey: "secret",
			},
		},
	}); err != nil {
		t.Fatalf("expected valid s3 config to be accepted, got %v", err)
	}

	if err := validateStorageTopology(config.Config{
		App: config.AppConfig{Env: "local"},
		Database: config.DatabaseConfig{
			URL: "host=db.internal port=5432 dbname=pic_gallery user=postgres sslmode=disable",
		},
		Storage: config.StorageConfig{Driver: "local"},
	}); err == nil {
		t.Fatal("expected remote key/value dsn to be rejected")
	}

	if err := validateStorageTopology(config.Config{
		App: config.AppConfig{Env: "local"},
		Database: config.DatabaseConfig{
			URL: "host=localhost port=5432 dbname=pic_gallery user=postgres sslmode=disable",
		},
		Storage: config.StorageConfig{Driver: "local"},
	}); err != nil {
		t.Fatalf("expected localhost key/value dsn to be allowed, got %v", err)
	}
}
