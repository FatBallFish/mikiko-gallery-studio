package mgsctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	deploymentassets "github.com/fatballfish/pic-gallery/deployments"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReconfigureNginxValidationRealCompose(t *testing.T) {
	if os.Getenv("MGSCTL_TEST_REAL_COMPOSE") != "1" {
		t.Skip("set MGSCTL_TEST_REAL_COMPOSE=1 for isolated real Compose nginx validation")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nginx.conf")
	if err := os.WriteFile(configPath, deploymentassets.NginxDefault(), 0600); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(dir, "compose.json")
	service := map[string]any{"services": map[string]any{"gateway": map[string]any{"image": "nginx:1.27-alpine", "volumes": []string{configPath + ":/etc/nginx/conf.d/default.conf:ro"}}}}
	data, _ := json.Marshal(service)
	if err := os.WriteFile(composePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"compose", "--project-directory", dir, "--file", composePath, "--project-name", fmt.Sprintf("mgsctl-nginx-validation-%d", os.Getpid())}
	var output bytes.Buffer
	runner := OSProcessRunner{Stdout: &output, Stderr: &output}
	defer func() {
		_ = runner.Run(context.Background(), ProcessSpec{Executable: "docker", Arguments: append(append([]string(nil), base...), "down", "--remove-orphans")})
	}()
	if err := validateDockerReconfigureNginx(t.Context(), ProcessSpec{Executable: "docker", Directory: dir}, base, runner); err != nil {
		t.Fatalf("real nginx validation failed: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "test is successful") {
		t.Fatalf("missing actual nginx result: %s", output.String())
	}
	t.Log(output.String())
}

func TestReconfigureNginxValidationUsesPrivateComposeOverride(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			base := []string{"compose", "--file", "/installed/compose.yml", "--project-name", "test", "--profile", "gateway"}
			var overridePath string
			runner := processRunnerFunc(func(_ context.Context, spec ProcessSpec) error {
				if !reflect.DeepEqual(spec.Arguments[:len(base)], base) {
					t.Fatal("compose base changed")
				}
				args := spec.Arguments[len(base):]
				if len(args) != 8 || args[0] != "--file" || !reflect.DeepEqual(args[2:], []string{"run", "--rm", "--no-deps", "gateway", "nginx", "-t"}) {
					t.Fatalf("incorrect arguments: %q", args)
				}
				overridePath = args[1]
				info, err := os.Stat(overridePath)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("override not private")
				}
				data, err := os.ReadFile(overridePath)
				if err != nil {
					t.Fatal(err)
				}
				var actual any
				if json.Unmarshal(data, &actual) != nil {
					t.Fatal("invalid compose JSON")
				}
				var expected any
				_ = json.Unmarshal([]byte(`{"services":{"gateway":{"extra_hosts":{"api":"127.0.0.1","user-web":"127.0.0.1","admin-web":"127.0.0.1","docs-web":"127.0.0.1"}}}}`), &expected)
				if !reflect.DeepEqual(actual, expected) {
					t.Fatal("override contains unexpected configuration")
				}
				if fail {
					return errors.New("private runner error")
				}
				return nil
			})
			err := validateDockerReconfigureNginx(t.Context(), ProcessSpec{Executable: "docker", Directory: filepath.Clean(t.TempDir())}, base, runner)
			if (err != nil) != fail {
				t.Fatal("unexpected validation result")
			}
			if _, err := os.Stat(overridePath); !os.IsNotExist(err) {
				t.Fatal("override was retained")
			}
		})
	}
}
