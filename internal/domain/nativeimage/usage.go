package nativeimage

import "encoding/json"

// MeteringUsage exposes numeric metering only; the complete wire usage remains
// in the private encrypted provider snapshot.
func MeteringUsage(raw json.RawMessage) json.RawMessage {
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	out := map[string]int64{}
	for _, key := range []string{"output_width", "output_height", "input_image_count", "output_image_count", "image_count", "width", "height", "input_images", "generated_images", "output_tokens", "total_tokens"} {
		if v, ok := values[key]; ok {
			var n int64
			if json.Unmarshal(v, &n) == nil && n >= 0 {
				out[key] = n
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	b, _ := json.Marshal(out)
	return b
}
