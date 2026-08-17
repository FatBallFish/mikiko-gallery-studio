package canvas

import (
	"math"
	"strings"
	"testing"
)

func testCanvasNode(id string, nodeType NodeType) Node {
	width, height := 220.0, 140.0
	switch nodeType {
	case NodeTypeImage, NodeTypeVideo:
		height = 160
	case NodeTypeAudio:
		width, height = 240, 120
	case NodeTypeImageGeneration, NodeTypeVideoGeneration:
		width, height = 280, 200
	}
	return Node{ID: id, Type: nodeType, Size: Size{Width: width, Height: height}}
}

func TestValidateDocumentSupportsSevenP0NodeTypes(t *testing.T) {
	document := DocumentV1{SchemaVersion: 1, Nodes: []Node{
		testCanvasNode("prompt", NodeTypePrompt),
		{ID: "image", Type: NodeTypeImage, AssetID: "asset-image", Size: Size{Width: 220, Height: 160}},
		{ID: "video", Type: NodeTypeVideo, AssetID: "asset-video", Size: Size{Width: 220, Height: 160}},
		{ID: "audio", Type: NodeTypeAudio, AssetID: "asset-audio", Size: Size{Width: 240, Height: 120}},
		testCanvasNode("image-gen", NodeTypeImageGeneration),
		testCanvasNode("video-gen", NodeTypeVideoGeneration),
		testCanvasNode("note", NodeTypeNote),
	}}
	if err := ValidateDocument(document, DefaultLimits()); err != nil {
		t.Fatalf("ValidateDocument() error = %v", err)
	}
}

func TestValidateDocumentRejectsIllegalConnectionsAndCycles(t *testing.T) {
	baseNodes := []Node{
		testCanvasNode("prompt", NodeTypePrompt),
		{ID: "image", Type: NodeTypeImage, AssetID: "asset-image", Size: Size{Width: 220, Height: 160}},
		{ID: "video", Type: NodeTypeVideo, AssetID: "asset-video", Size: Size{Width: 220, Height: 160}},
		testCanvasNode("image-gen", NodeTypeImageGeneration),
		testCanvasNode("video-gen", NodeTypeVideoGeneration),
	}
	valid := DocumentV1{SchemaVersion: 1, Nodes: baseNodes, Edges: []Edge{
		{ID: "prompt-image", Source: "prompt", Target: "image-gen", InputRole: InputRolePrompt},
		{ID: "image-video", Source: "image", Target: "video-gen", InputRole: InputRoleFirstFrame},
	}}
	if err := ValidateDocument(valid, DefaultLimits()); err != nil {
		t.Fatalf("valid graph rejected: %v", err)
	}

	illegal := valid
	illegal.Edges = append([]Edge(nil), valid.Edges...)
	illegal.Edges = append(illegal.Edges, Edge{ID: "video-input", Source: "video", Target: "video-gen", InputRole: InputRoleReference})
	if err := ValidateDocument(illegal, DefaultLimits()); err == nil || err.Code != "illegal_connection" || err.EdgeID != "video-input" {
		t.Fatalf("illegal graph error = %#v", err)
	}

	if !HasDirectedCycle([]string{"a", "b", "c"}, []GraphArc{{Source: "a", Target: "b"}, {Source: "b", Target: "c"}, {Source: "c", Target: "a"}}) {
		t.Fatal("directed cycle was not detected")
	}
}

func TestValidateDocumentEnforcesLimitsAndExtractsAssetReferences(t *testing.T) {
	nodes := make([]Node, 0, 501)
	for index := 0; index < 501; index++ {
		nodes = append(nodes, testCanvasNode(strings.Repeat("n", 1)+string(rune(index+1000)), NodeTypeNote))
	}
	if err := ValidateDocument(DocumentV1{SchemaVersion: 1, Nodes: nodes}, DefaultLimits()); err == nil || err.Code != "too_many_nodes" {
		t.Fatalf("node limit error = %#v", err)
	}

	document := DocumentV1{SchemaVersion: 1, Nodes: []Node{
		{ID: "one", Type: NodeTypeImage, AssetID: "asset-b", Size: Size{Width: 220, Height: 160}},
		{ID: "two", Type: NodeTypeVideo, AssetID: "asset-a", Size: Size{Width: 220, Height: 160}},
		{ID: "three", Type: NodeTypeImage, AssetID: "asset-b", Size: Size{Width: 220, Height: 160}},
	}}
	refs := ExtractAssetReferences(document)
	if len(refs) != 2 || refs[0] != "asset-a" || refs[1] != "asset-b" {
		t.Fatalf("ExtractAssetReferences() = %#v", refs)
	}
}

func TestValidateDocumentRejectsUnsafeNodeGeometry(t *testing.T) {
	tests := []struct {
		name string
		node Node
		code string
	}{
		{name: "non finite position", node: Node{ID: "prompt", Type: NodeTypePrompt, Position: Point{X: math.NaN()}, Size: Size{Width: 220, Height: 140}}, code: "invalid_node_position"},
		{name: "coordinate outside safety range", node: Node{ID: "prompt", Type: NodeTypePrompt, Position: Point{X: 1_000_001}, Size: Size{Width: 220, Height: 140}}, code: "invalid_node_position"},
		{name: "non finite size", node: Node{ID: "image-gen", Type: NodeTypeImageGeneration, Size: Size{Width: math.Inf(1), Height: 200}}, code: "invalid_node_size"},
		{name: "prompt below minimum", node: Node{ID: "prompt", Type: NodeTypePrompt, Size: Size{Width: 219, Height: 140}}, code: "invalid_node_size"},
		{name: "audio below minimum", node: Node{ID: "audio", Type: NodeTypeAudio, AssetID: "asset-audio", Size: Size{Width: 240, Height: 119}}, code: "invalid_node_size"},
		{name: "generation below minimum", node: Node{ID: "image-gen", Type: NodeTypeImageGeneration, Size: Size{Width: 280, Height: 199}}, code: "invalid_node_size"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := DocumentV1{SchemaVersion: 1, Nodes: []Node{test.node}}
			if err := ValidateDocument(document, DefaultLimits()); err == nil || err.Code != test.code || err.NodeID != test.node.ID {
				t.Fatalf("ValidateDocument() error = %#v, want code %q for node %q", err, test.code, test.node.ID)
			}
		})
	}
}

func TestValidateDocumentAllowsOnlyConnectedEmptyImageOutputSlots(t *testing.T) {
	generation := testCanvasNode("image-gen", NodeTypeImageGeneration)
	emptyImage := testCanvasNode("empty-image", NodeTypeImage)
	valid := DocumentV1{SchemaVersion: 1, Nodes: []Node{generation, emptyImage}, Edges: []Edge{{ID: "result", Source: generation.ID, Target: emptyImage.ID, InputRole: InputRoleResult}}}
	if err := ValidateDocument(valid, DefaultLimits()); err != nil {
		t.Fatalf("connected empty image output slot rejected: %v", err)
	}
	if refs := ExtractAssetReferences(valid); len(refs) != 0 {
		t.Fatalf("empty output slot asset references = %#v, want none", refs)
	}

	missingEdge := valid
	missingEdge.Edges = nil
	if err := ValidateDocument(missingEdge, DefaultLimits()); err == nil || err.Code != "asset_required" || err.NodeID != emptyImage.ID {
		t.Fatalf("unconnected empty image error = %#v", err)
	}

	emptyVideo := testCanvasNode("empty-video", NodeTypeVideo)
	videoGeneration := testCanvasNode("video-gen", NodeTypeVideoGeneration)
	invalidVideo := DocumentV1{SchemaVersion: 1, Nodes: []Node{videoGeneration, emptyVideo}, Edges: []Edge{{ID: "video-result", Source: videoGeneration.ID, Target: emptyVideo.ID, InputRole: InputRoleResult}}}
	if err := ValidateDocument(invalidVideo, DefaultLimits()); err == nil || err.Code != "asset_required" || err.NodeID != emptyVideo.ID {
		t.Fatalf("empty video output error = %#v", err)
	}
}

func TestStableResultNodeIDIsIdempotent(t *testing.T) {
	first := StableResultNodeID("run-1", "asset-1")
	second := StableResultNodeID("run-1", "asset-1")
	other := StableResultNodeID("run-1", "asset-2")
	if first != second || first == other || !strings.HasPrefix(first, "result-") {
		t.Fatalf("stable IDs = %q %q %q", first, second, other)
	}
}
