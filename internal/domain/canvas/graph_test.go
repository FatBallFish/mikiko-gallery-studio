package canvas

import (
	"strings"
	"testing"
)

func TestValidateDocumentSupportsSevenP0NodeTypes(t *testing.T) {
	document := DocumentV1{SchemaVersion: 1, Nodes: []Node{
		{ID: "prompt", Type: NodeTypePrompt},
		{ID: "image", Type: NodeTypeImage, AssetID: "asset-image"},
		{ID: "video", Type: NodeTypeVideo, AssetID: "asset-video"},
		{ID: "audio", Type: NodeTypeAudio, AssetID: "asset-audio"},
		{ID: "image-gen", Type: NodeTypeImageGeneration},
		{ID: "video-gen", Type: NodeTypeVideoGeneration},
		{ID: "note", Type: NodeTypeNote},
	}}
	if err := ValidateDocument(document, DefaultLimits()); err != nil {
		t.Fatalf("ValidateDocument() error = %v", err)
	}
}

func TestValidateDocumentRejectsIllegalConnectionsAndCycles(t *testing.T) {
	baseNodes := []Node{
		{ID: "prompt", Type: NodeTypePrompt},
		{ID: "image", Type: NodeTypeImage, AssetID: "asset-image"},
		{ID: "video", Type: NodeTypeVideo, AssetID: "asset-video"},
		{ID: "image-gen", Type: NodeTypeImageGeneration},
		{ID: "video-gen", Type: NodeTypeVideoGeneration},
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
		nodes = append(nodes, Node{ID: strings.Repeat("n", 1) + string(rune(index+1000)), Type: NodeTypeNote})
	}
	if err := ValidateDocument(DocumentV1{SchemaVersion: 1, Nodes: nodes}, DefaultLimits()); err == nil || err.Code != "too_many_nodes" {
		t.Fatalf("node limit error = %#v", err)
	}

	document := DocumentV1{SchemaVersion: 1, Nodes: []Node{
		{ID: "one", Type: NodeTypeImage, AssetID: "asset-b"},
		{ID: "two", Type: NodeTypeVideo, AssetID: "asset-a"},
		{ID: "three", Type: NodeTypeImage, AssetID: "asset-b"},
	}}
	refs := ExtractAssetReferences(document)
	if len(refs) != 2 || refs[0] != "asset-a" || refs[1] != "asset-b" {
		t.Fatalf("ExtractAssetReferences() = %#v", refs)
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
