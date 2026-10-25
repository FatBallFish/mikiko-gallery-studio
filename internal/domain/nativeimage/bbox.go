package nativeimage

import (
	"encoding/json"
	"fmt"
)

func (b *BoundingBox) UnmarshalJSON(data []byte) error {
	var raw struct {
		Absolute   []json.RawMessage `json:"absolute"`
		Normalized []json.RawMessage `json:"normalized"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.Absolute) != 4 || len(raw.Normalized) != 4 {
		return fmt.Errorf("bounding box requires exactly four absolute and normalized coordinates")
	}
	var out BoundingBox
	for i := 0; i < 4; i++ {
		for j, values := range [][]json.RawMessage{raw.Absolute, raw.Normalized} {
			var n *int
			if err := json.Unmarshal(values[i], &n); err != nil {
				return err
			}
			if n == nil {
				return fmt.Errorf("null bounding box coordinate")
			}
			if j == 0 {
				out.Absolute[i] = *n
			} else {
				out.Normalized[i] = *n
			}
		}
	}
	*b = out
	return nil
}
