package config

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeEnvRoundTripComplexValues(t *testing.T) {
	input := []byte(`PLAIN=plain
SPACES="two words"
HASH='value # retained'
EQUALS="left=right"
QUOTES="single ' and double \" quotes"
UNICODE="初始化配置"
EMPTY=
DATABASE_URL='postgres://app:p%40ss%23word@db:5432/app?sslmode=disable&application_name=setup'
`)

	document, err := ParseRuntimeEnv(input)
	if err != nil {
		t.Fatalf("ParseRuntimeEnv returned error: %v", err)
	}
	want := map[string]string{
		"PLAIN":        "plain",
		"SPACES":       "two words",
		"HASH":         "value # retained",
		"EQUALS":       "left=right",
		"QUOTES":       `single ' and double " quotes`,
		"UNICODE":      "初始化配置",
		"EMPTY":        "",
		"DATABASE_URL": "postgres://app:p%40ss%23word@db:5432/app?sslmode=disable&application_name=setup",
	}
	for key, value := range want {
		if got := document.Values[key]; got != value {
			t.Errorf("%s: got %q, want %q", key, got, value)
		}
	}
}

func TestRuntimeEnvRendererAddsBilingualCommentsAndRoundTrips(t *testing.T) {
	schema := DefaultRuntimeSchema()
	values := map[string]string{
		"RUNTIME_SCHEMA_VERSION": "1",
		"DEPLOYMENT_MODE":        "docker",
		"DATABASE_URL":           "postgres://app:p%40ss%23word@db:5432/app?sslmode=disable",
		"SETUP_TOKEN":            `token with # and = and "quotes"`,
		"CORS_ALLOWED_ORIGINS":   "http://127.0.0.1:5173, http://localhost:5174",
		"STORAGE_S3_PREFIX":      "作品/2026 July",
	}

	rendered, err := RenderRuntimeEnv(schema, values, nil)
	if err != nil {
		t.Fatalf("RenderRuntimeEnv returned error: %v", err)
	}
	for _, field := range schema.Fields {
		zh := "# [中文] " + field.DescriptionZH
		en := "# [English] " + field.DescriptionEN
		if bytes.Count(rendered, []byte(zh)) != 1 || bytes.Count(rendered, []byte(en)) != 1 {
			t.Errorf("field %s must render exactly one pair of bilingual comments", field.Key)
		}
	}
	if bytes.Contains(rendered, []byte(values["SETUP_TOKEN"])) {
		t.Fatal("complex setup token must be safely quoted, not emitted verbatim")
	}
	if bytes.Contains(bytes.ToLower(rendered), []byte("example: sk-")) || bytes.Contains(rendered, []byte("Example: "+values["SETUP_TOKEN"])) {
		t.Fatal("renderer exposed an unredacted secret example")
	}

	parsed, err := ParseRuntimeEnv(rendered)
	if err != nil {
		t.Fatalf("parse rendered env: %v\n%s", err, rendered)
	}
	for key, want := range values {
		if got := parsed.Values[key]; got != want {
			t.Errorf("round-trip %s: got %q, want %q", key, got, want)
		}
	}
}

func TestRuntimeEnvUpgradePreservesValuesAndUnknownExtensions(t *testing.T) {
	previous := []byte(`# legacy comments are replaced by current schema comments
RUNTIME_SCHEMA_VERSION=1
DATABASE_URL="postgres://existing@db:5432/app?sslmode=disable"
VENDOR_FEATURE_FLAG="enabled # keep"
VENDOR_ENDPOINT='https://example.test/path?a=b'
`)
	document, err := ParseRuntimeEnv(previous)
	if err != nil {
		t.Fatalf("parse previous runtime env: %v", err)
	}

	schema := DefaultRuntimeSchema()
	schema.Version++
	rendered, err := RenderRuntimeEnv(schema, document.Values, document.Extensions)
	if err != nil {
		t.Fatalf("render upgraded runtime env: %v", err)
	}
	if !bytes.Contains(rendered, []byte("# Extension fields / 扩展字段")) {
		t.Fatalf("missing extension section:\n%s", rendered)
	}
	upgraded, err := ParseRuntimeEnv(rendered)
	if err != nil {
		t.Fatalf("parse upgraded runtime env: %v", err)
	}
	for key, want := range map[string]string{
		"DATABASE_URL":        "postgres://existing@db:5432/app?sslmode=disable",
		"VENDOR_FEATURE_FLAG": "enabled # keep",
		"VENDOR_ENDPOINT":     "https://example.test/path?a=b",
	} {
		if got := upgraded.Values[key]; got != want {
			t.Errorf("upgrade changed %s: got %q, want %q", key, got, want)
		}
	}
	if got := upgraded.Values["RUNTIME_SCHEMA_VERSION"]; got != "2" {
		t.Fatalf("schema version must advance to 2, got %q", got)
	}
}

func TestRuntimeEnvParserRejectsMalformedOrDuplicateEntries(t *testing.T) {
	for _, input := range []string{
		"NOT AN ENV LINE\n",
		"1INVALID=value\n",
		"DUPLICATE=first\nDUPLICATE=second\n",
		"BROKEN=\"unterminated\n",
		"TRAILING='value' unexpected\n",
	} {
		if _, err := ParseRuntimeEnv([]byte(input)); err == nil {
			t.Errorf("expected parser to reject %q", input)
		}
	}
}

func TestWriteRuntimeEnvAtomicCreatesPrivateFileAndReplacesContent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	path := filepath.Join(dir, "runtime.env")
	if err := WriteRuntimeEnvAtomic(path, []byte("VALUE=first\n")); err != nil {
		t.Fatalf("first atomic write: %v", err)
	}
	if err := WriteRuntimeEnvAtomic(path, []byte("VALUE=second\n")); err != nil {
		t.Fatalf("replace atomic write: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime env: %v", err)
	}
	if string(content) != "VALUE=second\n" {
		t.Fatalf("unexpected content %q", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat runtime env: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("runtime env permissions = %04o, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read config directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".runtime.env.tmp-") {
			t.Errorf("temporary file leaked after successful write: %s", entry.Name())
		}
	}
}
