package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/google/jsonschema-go/jsonschema"
)

// Catalogue returns the shared operations with the MCP structured result envelope.
// Payloads are references and upload targets; tools never accept binary/base64 blobs
// or arbitrary import URLs. The shared service enforces ownership and idempotency.
func Catalogue() []openplatform.OperationSpec {
	specs := openplatform.Specs()
	for i := range specs {
		// MCP advertises a compact envelope; the service schema remains validated by the handler.
		specs[i].OutputSchema = boundedOutputSchema
	}
	return append(specs, readToolSpec())
}

func resolveSchema(schema map[string]any) (*jsonschema.Resolved, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var parsed jsonschema.Schema
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return parsed.Resolve(nil)
}
func validateInput(spec openplatform.OperationSpec, input json.RawMessage) error {
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	var value any
	if err := json.Unmarshal(input, &value); err != nil {
		return errors.New("tool arguments must be a JSON object")
	}
	if _, ok := value.(map[string]any); !ok {
		return errors.New("tool arguments must be a JSON object")
	}
	resolved, err := resolveSchema(spec.InputSchema)
	if err != nil {
		return errors.New("invalid configured tool schema")
	}
	if err := resolved.Validate(value); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}
