package openapi

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/api/openapi/schemagen"
	"gopkg.in/yaml.v3"
)

func TestGeneratedNativeContractIsCurrent(t *testing.T) {
	raw, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := schemagen.Sync(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, updated) {
		t.Fatal("OpenAPI is stale: go run ./api/openapi/cmd/generate")
	}
	expected, err := schemagen.Schemas()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile("components/schemas/native.generated.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatal("native schemas are stale: go run ./api/openapi/cmd/generate")
	}
}

// Every reference is resolved against its own document, including references in
// generated nested $defs. This catches root-reference rebasing and missing files.
func TestAllOpenAPIReferencesResolve(t *testing.T) {
	cache := map[string]any{}
	visited := map[string]bool{}
	var load func(string) any
	load = func(file string) any {
		if v, ok := cache[file]; ok {
			return v
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read referenced document %s: %v", file, err)
		}
		var v any
		if strings.HasSuffix(file, ".json") {
			err = json.Unmarshal(raw, &v)
		} else {
			err = yaml.Unmarshal(raw, &v)
		}
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		cache[file] = v
		return v
	}
	var walk func(any, string)
	walk = func(v any, file string) {
		switch x := v.(type) {
		case map[string]any:
			if s, ok := x["$ref"].(string); ok {
				parts := strings.SplitN(s, "#", 2)
				target := file
				if parts[0] != "" {
					if strings.Contains(parts[0], "://") {
						t.Fatalf("uncontrolled remote reference %s", s)
					}
					target = filepath.Clean(filepath.Join(filepath.Dir(file), parts[0]))
				}
				value := load(target)
				if len(parts) == 2 && parts[1] != "" {
					if !strings.HasPrefix(parts[1], "/") {
						t.Fatalf("unsupported pointer %s", s)
					}
					for _, key := range strings.Split(strings.TrimPrefix(parts[1], "/"), "/") {
						key = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
						switch current := value.(type) {
						case map[string]any:
							var exists bool
							value, exists = current[key]
							if !exists {
								t.Fatalf("%s: unresolved %s at %s", file, s, key)
							}
						case []any:
							i, err := strconv.Atoi(key)
							if err != nil || i < 0 || i >= len(current) {
								t.Fatalf("invalid array pointer %s", s)
							}
							value = current[i]
						default:
							t.Fatalf("%s: non-object pointer %s", file, s)
						}
					}
				}
				id := target + "#" + s
				if !visited[id] {
					visited[id] = true
					walk(value, target)
				}
				if !visited[target] {
					visited[target] = true
					walk(load(target), target)
				}
			}
			for _, child := range x {
				walk(child, file)
			}
		case []any:
			for _, child := range x {
				walk(child, file)
			}
		}
	}
	walk(load("openapi.yaml"), "openapi.yaml")
}
func TestOperationIDsAreUniqueAndNewPublicContractsRemain(t *testing.T) {
	raw, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	paths := doc["paths"].(map[string]any)
	seen := map[string]string{}
	for path, item := range paths {
		for method, v := range item.(map[string]any) {
			op, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if id, ok := op["operationId"].(string); ok {
				if old := seen[id]; old != "" {
					t.Errorf("duplicate operationId %s: %s, %s %s", id, old, method, path)
				}
				seen[id] = method + " " + path
			}
		}
	}
	for _, action := range []string{"like", "favorite", "download"} {
		item := paths["/api/public/v1/videos/{asset_id}/"+action].(map[string]any)
		methods := []string{"post"}
		if action == "download" {
			methods = []string{"get", "head"}
		}
		for _, method := range methods {
			op := item[method].(map[string]any)
			security := op["security"].([]any)
			if _, ok := security[0].(map[string]any)["bearerAuth"]; !ok {
				t.Errorf("%s %s lacks browser Bearer", method, action)
			}
			if action == "download" {
				for _, status := range []string{"200", "206", "416"} {
					if op["responses"].(map[string]any)[status] == nil {
						t.Errorf("download missing %s", status)
					}
				}
			}
		}
	}
	if paths["/api/v1/public/developer-endpoints"] == nil {
		t.Fatal("flat endpoint configuration route lost")
	}
}
