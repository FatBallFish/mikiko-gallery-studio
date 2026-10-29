package mcpserver

import (
	"encoding/json"
	"testing"
	"unicode/utf8"
)

func TestUTF8FragmentSmallLimitAndInvalidOffsets(t *testing.T) {
	const source = "中😀文"
	offset := 0
	rebuilt := ""
	for offset < len(source) {
		fragment, next, complete, err := utf8Fragment(source, offset, 1)
		if err != nil {
			t.Fatalf("limit=1 must make full-rune progress: %v", err)
		}
		if !utf8.ValidString(fragment) || utf8.RuneCountInString(fragment) != 1 {
			t.Fatal("small limit split a UTF-8 rune")
		}
		if next != offset+len(fragment) || next <= offset {
			t.Fatal("fragment next_offset does not advance by consumed bytes")
		}
		if complete != (next == len(source)) {
			t.Fatal("incorrect fragment completion flag")
		}
		rebuilt += fragment
		offset = next
	}
	if rebuilt != source {
		t.Fatal("small-limit fragments do not reconstruct original string")
	}
	fragment, next, complete, err := utf8Fragment(source, len(source), 1)
	if err != nil || fragment != "" || next != len(source) || !complete {
		t.Fatal("terminal byte offset must return empty completed fragment")
	}
	for _, tc := range []struct {
		name          string
		offset, limit int
	}{
		{"negative offset", -1, 1}, {"past end", len(source) + 1, 1}, {"zero limit", 0, 0}, {"negative limit", 0, -1},
		{"Chinese continuation byte", 1, 1}, {"Chinese final byte", 2, 1}, {"emoji continuation byte", 4, 1}, {"emoji final byte", 6, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, err := utf8Fragment(source, tc.offset, tc.limit); err == nil {
				t.Fatal("invalid UTF-8 fragment request accepted")
			}
		})
	}
}

func TestReadPageRejectsInvalidOffsets(t *testing.T) {
	for _, tc := range []struct {
		name          string
		value         any
		offset, limit int
	}{
		{"negative offset", []any{1}, -1, 1}, {"negative limit", []any{1}, 0, -1},
		{"array past end", []any{1}, 2, 1}, {"object past end", map[string]any{"one": 1}, 2, 1},
		{"string past end", "中", 4, 1}, {"string inside rune", "中", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readPage("fixture-token", "", tc.offset, tc.limit, tc.value); err == nil {
				t.Fatal("invalid read offset or limit accepted")
			}
		})
	}
	page, err := readPage("fixture-token", "/prompt", 0, 1, "中文")
	if err != nil {
		t.Fatalf("small-limit string page: %v", err)
	}
	if page["value"] != "中" || page["complete"] != false {
		t.Fatal("small-limit read did not emit exactly first complete rune")
	}
	wire, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]any
	if err := json.Unmarshal(wire, &encoded); err != nil {
		t.Fatal(err)
	}
	if encoded["next_offset"] != float64(3) {
		t.Fatal("small-limit read returned incorrect byte continuation")
	}
}

func TestDecodeResultPreservesLargeNumbers(t *testing.T) {
	value, err := decodeResult([]byte(`{"id":9007199254740993,"nested":[18446744073709551615]}`))
	if err != nil {
		t.Fatal(err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatal("decoded result is not an object")
	}
	id, ok := object["id"].(json.Number)
	if !ok || id.String() != "9007199254740993" {
		t.Fatal("decoder rounded integer beyond float64 precision")
	}
	nested := object["nested"].([]any)
	n, ok := nested[0].(json.Number)
	if !ok || n.String() != "18446744073709551615" {
		t.Fatal("decoder rounded nested large integer")
	}
	for _, raw := range []string{`{"id":`, `{"id":01}`, `not-json`} {
		if _, err := decodeResult([]byte(raw)); err == nil {
			t.Error("malformed result JSON accepted")
		}
	}
	receipt := receipt(map[string]any{"id": json.Number("9007199254740993"), "status": "queued"}, "create_image_task")
	if id, ok := receipt["id"].(json.Number); !ok || id.String() != "9007199254740993" {
		t.Fatal("write receipt rounded durable ID")
	}
	if receipt["operation_completed"] != true || receipt["query_tool"] != "get_image_task" {
		t.Fatal("write receipt lost completion or recovery tool")
	}
}
