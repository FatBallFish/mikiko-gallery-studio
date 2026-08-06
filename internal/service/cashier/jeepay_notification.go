package cashier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/url"
	"strconv"
	"strings"
)

const maxJeePayNotificationBodyBytes = 1 << 20

// ParseJeePayNotification normalizes official JSON callbacks and compatible
// form callbacks without changing the scalar text used for signature checks.
func ParseJeePayNotification(body []byte, contentType string) (map[string]string, error) {
	if len(body) > maxJeePayNotificationBodyBytes {
		return nil, fmt.Errorf("jeepay notification body exceeds %d bytes", maxJeePayNotificationBodyBytes)
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("jeepay notification body is empty")
	}
	if trimmed[0] == '{' || trimmed[0] == '[' || trimmed[0] == '"' {
		return parseJeePayJSONNotification(body)
	}

	jsonFirst := false
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
		switch strings.ToLower(mediaType) {
		case "application/json", "text/json":
			jsonFirst = true
		case "application/x-www-form-urlencoded":
			if trimmed[0] != '{' {
				jsonFirst = false
			}
		}
	}

	first, second := parseJeePayFormNotification, parseJeePayJSONNotification
	if jsonFirst {
		first, second = parseJeePayJSONNotification, parseJeePayFormNotification
	}
	values, firstErr := first(body)
	if firstErr == nil {
		return values, nil
	}
	values, secondErr := second(body)
	if secondErr == nil {
		return values, nil
	}
	return nil, fmt.Errorf("invalid jeepay notification body: %v; fallback: %v", firstErr, secondErr)
}

func parseJeePayJSONNotification(body []byte) (map[string]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	start, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode JSON object: %w", err)
	}
	if delimiter, ok := start.(json.Delim); !ok || delimiter != '{' {
		return nil, fmt.Errorf("JSON notification must be an object")
	}

	values := make(map[string]string)
	for decoder.More() {
		rawName, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode JSON field name: %w", err)
		}
		name, ok := rawName.(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("JSON notification contains an invalid field name")
		}
		if _, exists := values[name]; exists {
			return nil, fmt.Errorf("JSON notification contains duplicate field %q", name)
		}

		var rawValue any
		if err := decoder.Decode(&rawValue); err != nil {
			return nil, fmt.Errorf("decode JSON field %q: %w", name, err)
		}
		value, err := jeepayScalarString(rawValue)
		if err != nil {
			return nil, fmt.Errorf("decode JSON field %q: %w", name, err)
		}
		values[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("decode JSON object end: %w", err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("JSON notification object is empty")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("JSON notification contains trailing data")
		}
		return nil, fmt.Errorf("decode trailing JSON data: %w", err)
	}
	return values, nil
}

func parseJeePayFormNotification(body []byte) (map[string]string, error) {
	raw := string(body)
	for _, field := range strings.Split(raw, "&") {
		if field == "" || !strings.Contains(field, "=") {
			return nil, fmt.Errorf("form notification contains an invalid field")
		}
	}
	parsed, err := url.ParseQuery(raw)
	if err != nil {
		return nil, fmt.Errorf("decode form notification: %w", err)
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("form notification is empty")
	}
	values := make(map[string]string, len(parsed))
	for name, candidates := range parsed {
		if name == "" || len(candidates) != 1 {
			return nil, fmt.Errorf("form notification contains ambiguous field %q", name)
		}
		values[name] = candidates[0]
	}
	return values, nil
}

func jeepayScalarString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case json.Number:
		return typed.String(), nil
	case bool:
		return strconv.FormatBool(typed), nil
	default:
		return "", fmt.Errorf("notification values must be strings, numbers, or booleans")
	}
}
