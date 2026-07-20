package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type EnvEntry struct {
	Key   string
	Value string
}

type RuntimeEnvDocument struct {
	Values     map[string]string
	Entries    []EnvEntry
	Extensions []EnvEntry
}

func ParseRuntimeEnv(data []byte) (RuntimeEnvDocument, error) {
	document := RuntimeEnvDocument{Values: make(map[string]string)}
	known := runtimeSchemaKeySet(DefaultRuntimeSchema())
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if lineNumber == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		equals := strings.IndexByte(line, '=')
		if equals < 0 {
			return RuntimeEnvDocument{}, fmt.Errorf("parse runtime env line %d: expected KEY=VALUE", lineNumber)
		}
		key := strings.TrimSpace(line[:equals])
		if !runtimeFieldKeyPattern.MatchString(key) {
			return RuntimeEnvDocument{}, fmt.Errorf("parse runtime env line %d: invalid key %q", lineNumber, key)
		}
		if _, exists := document.Values[key]; exists {
			return RuntimeEnvDocument{}, fmt.Errorf("parse runtime env line %d: duplicate key %q", lineNumber, key)
		}
		value, err := parseRuntimeEnvValue(line[equals+1:])
		if err != nil {
			return RuntimeEnvDocument{}, fmt.Errorf("parse runtime env line %d key %q: %w", lineNumber, key, err)
		}
		entry := EnvEntry{Key: key, Value: value}
		document.Values[key] = value
		document.Entries = append(document.Entries, entry)
		if _, exists := known[key]; !exists {
			document.Extensions = append(document.Extensions, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return RuntimeEnvDocument{}, fmt.Errorf("scan runtime env: %w", err)
	}
	return document, nil
}

func RenderRuntimeEnv(schema RuntimeSchema, values map[string]string, extensions []EnvEntry) ([]byte, error) {
	if err := schema.Validate(); err != nil {
		return nil, fmt.Errorf("validate runtime schema: %w", err)
	}

	known := runtimeSchemaKeySet(schema)
	var output bytes.Buffer
	output.WriteString("# Generated runtime configuration. Update values through deployctl or the setup interface.\n")
	output.WriteString("# 自动生成的运行时配置。请通过 deployctl 或初始化界面更新配置值。\n")

	lastGroup := ""
	for _, runtimeField := range schema.Fields {
		if runtimeField.Group != lastGroup {
			fmt.Fprintf(&output, "\n# ---- %s ----\n", runtimeField.Group)
			lastGroup = runtimeField.Group
		}
		fmt.Fprintf(&output, "# [中文] %s\n", runtimeField.DescriptionZH)
		fmt.Fprintf(&output, "# [English] %s\n", runtimeField.DescriptionEN)
		fmt.Fprintf(&output, "# Owner: %s; Restart required: %t\n", runtimeField.Owner, runtimeField.RestartRequired)
		if runtimeField.Secret {
			output.WriteString("# Example: <redacted>\n")
		} else if runtimeField.Example != "" {
			fmt.Fprintf(&output, "# Example: %s\n", runtimeField.Example)
		}

		value, exists := values[runtimeField.Key]
		if runtimeField.Key == "RUNTIME_SCHEMA_VERSION" {
			value = strconv.Itoa(schema.Version)
			exists = true
		}
		if !exists {
			value = runtimeField.DefaultValue
		}
		if value != "" {
			if err := runtimeField.Validate(value); err != nil {
				return nil, fmt.Errorf("validate runtime field %s: %w", runtimeField.Key, err)
			}
		}
		encoded, err := encodeRuntimeEnvValue(value)
		if err != nil {
			return nil, fmt.Errorf("encode runtime field %s: %w", runtimeField.Key, err)
		}
		fmt.Fprintf(&output, "%s=%s\n", runtimeField.Key, encoded)
	}

	extensionEntries, err := collectRuntimeExtensions(values, extensions, known)
	if err != nil {
		return nil, err
	}
	if len(extensionEntries) > 0 {
		output.WriteString("\n# Extension fields / 扩展字段\n")
		for _, entry := range extensionEntries {
			encoded, err := encodeRuntimeEnvValue(entry.Value)
			if err != nil {
				return nil, fmt.Errorf("encode extension field %s: %w", entry.Key, err)
			}
			fmt.Fprintf(&output, "%s=%s\n", entry.Key, encoded)
		}
	}

	return output.Bytes(), nil
}

func WriteRuntimeEnvAtomic(path string, data []byte) (returnErr error) {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("runtime env path must not be empty")
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create runtime env directory %q: %w", directory, err)
	}

	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary runtime env in %q: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	temporaryOpen := true
	defer func() {
		if temporaryOpen {
			if closeErr := temporary.Close(); returnErr == nil && closeErr != nil {
				returnErr = fmt.Errorf("close temporary runtime env: %w", closeErr)
			}
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && returnErr == nil {
			returnErr = fmt.Errorf("remove temporary runtime env: %w", removeErr)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("set temporary runtime env permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary runtime env: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary runtime env: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary runtime env before rename: %w", err)
	}
	temporaryOpen = false
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace runtime env %q: %w", path, err)
	}
	if runtime.GOOS != "windows" {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func parseRuntimeEnvValue(raw string) (string, error) {
	trimmedLeft := strings.TrimLeft(raw, " \t")
	if trimmedLeft == "" {
		return "", nil
	}
	switch trimmedLeft[0] {
	case '\'':
		closing := strings.IndexByte(trimmedLeft[1:], '\'')
		if closing < 0 {
			return "", fmt.Errorf("unterminated single-quoted value")
		}
		closing++
		if err := validateQuotedRemainder(trimmedLeft[closing+1:]); err != nil {
			return "", err
		}
		return trimmedLeft[1:closing], nil
	case '"':
		closing := findDoubleQuoteEnd(trimmedLeft)
		if closing < 0 {
			return "", fmt.Errorf("unterminated double-quoted value")
		}
		if err := validateQuotedRemainder(trimmedLeft[closing+1:]); err != nil {
			return "", err
		}
		value, err := strconv.Unquote(trimmedLeft[:closing+1])
		if err != nil {
			return "", fmt.Errorf("decode double-quoted value: %w", err)
		}
		return value, nil
	default:
		return strings.TrimSpace(raw), nil
	}
}

func validateQuotedRemainder(remainder string) error {
	remainder = strings.TrimSpace(remainder)
	if remainder == "" || strings.HasPrefix(remainder, "#") {
		return nil
	}
	return fmt.Errorf("unexpected content after quoted value")
}

func findDoubleQuoteEnd(value string) int {
	escaped := false
	for index := 1; index < len(value); index++ {
		switch {
		case escaped:
			escaped = false
		case value[index] == '\\':
			escaped = true
		case value[index] == '"':
			return index
		}
	}
	return -1
}

func encodeRuntimeEnvValue(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("dotenv values must not contain NUL or newlines")
	}
	if value == "" {
		return "", nil
	}
	if isPlainRuntimeEnvValue(value) {
		return value, nil
	}
	return strconv.Quote(value), nil
}

func isPlainRuntimeEnvValue(value string) bool {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') {
			continue
		}
		switch character {
		case '_', '-', '.', '/', ':', '@', '%', '+', ',':
			continue
		default:
			return false
		}
	}
	return true
}

func collectRuntimeExtensions(values map[string]string, extensions []EnvEntry, known map[string]struct{}) ([]EnvEntry, error) {
	result := make([]EnvEntry, 0, len(extensions))
	seen := make(map[string]struct{}, len(extensions))
	for _, entry := range extensions {
		if !runtimeFieldKeyPattern.MatchString(entry.Key) {
			return nil, fmt.Errorf("extension field key %q is invalid", entry.Key)
		}
		if _, isKnown := known[entry.Key]; isKnown {
			continue
		}
		if _, duplicate := seen[entry.Key]; duplicate {
			return nil, fmt.Errorf("extension field key %q is duplicated", entry.Key)
		}
		seen[entry.Key] = struct{}{}
		if value, exists := values[entry.Key]; exists {
			entry.Value = value
		}
		result = append(result, entry)
	}

	extraKeys := make([]string, 0)
	for key := range values {
		if _, isKnown := known[key]; isKnown {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		if !runtimeFieldKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("extension field key %q is invalid", key)
		}
		extraKeys = append(extraKeys, key)
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		result = append(result, EnvEntry{Key: key, Value: values[key]})
	}
	return result, nil
}

func runtimeSchemaKeySet(schema RuntimeSchema) map[string]struct{} {
	keys := make(map[string]struct{}, len(schema.Fields))
	for _, runtimeField := range schema.Fields {
		keys[runtimeField.Key] = struct{}{}
	}
	return keys
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open runtime env directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return fmt.Errorf("sync runtime env directory: %w", err)
	}
	return nil
}
