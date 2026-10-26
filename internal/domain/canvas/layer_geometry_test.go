package canvas

import (
	"encoding/json"
	"math"
	"testing"
)

func TestLayerNodesAllowExactSmallGeometry(t *testing.T) {
	valid := `{"layer":{"z_index":2,"is_base":false,"bounding_box":{"absolute":[100,200,102,203],"normalized":[100,200,102,203]}},"width":2,"height":3}`
	for _, tc := range []struct {
		name, payload string
		size          Size
		ok            bool
	}{
		{"small crop", valid, Size{Width: .5, Height: .75}, true},
		{"small base", `{"layer":{"z_index":0,"is_base":true},"width":1000,"height":500}`, Size{Width: 20, Height: 10}, true},
		{"ordinary small", `{}`, Size{Width: 79, Height: 79}, false},
		{"ordinary normal", `{}`, Size{Width: 80, Height: 40}, true},
		{"missing dimensions", `{"layer":{"is_base":true}}`, Size{Width: 2, Height: 3}, false},
		{"missing bbox", `{"layer":{"is_base":false},"width":2,"height":3}`, Size{Width: 2, Height: 3}, false},
		{"inverted bbox", `{"layer":{"bounding_box":{"absolute":[3,2,1,4]}},"width":2,"height":3}`, Size{Width: 2, Height: 3}, false},
		{"unbounded bbox", `{"layer":{"bounding_box":{"absolute":[0,0,1000001,4]}},"width":2,"height":3}`, Size{Width: 2, Height: 3}, false},
		{"unbounded normalized", `{"layer":{"bounding_box":{"absolute":[0,0,2,4],"normalized":[0,0,1001,4]}},"width":2,"height":3}`, Size{Width: 2, Height: 3}, false},
		{"zero size", valid, Size{Width: 0, Height: 3}, false},
		{"infinite size", valid, Size{Width: math.Inf(1), Height: 3}, false},
		{"unbounded size", valid, Size{Width: 1000001, Height: 3}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := DocumentV1{SchemaVersion: 1, Nodes: []Node{{ID: "layer", Type: NodeTypeImageV2, AssetID: "asset", Size: tc.size, Payload: json.RawMessage(tc.payload)}}}
			err := ValidateDocument(doc, DefaultLimits())
			if (err == nil) != tc.ok {
				t.Fatalf("ValidateDocument()=%v, want valid=%v", err, tc.ok)
			}
		})
	}
}
