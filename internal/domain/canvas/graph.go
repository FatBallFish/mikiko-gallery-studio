package canvas

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

const maxCanvasCoordinate = 1_000_000

type GraphArc struct {
	Source string
	Target string
}

func ValidateDocument(document DocumentV1, limits Limits) *ValidationError {
	if document.SchemaVersion != 1 {
		return graphError("unsupported_schema", "", "", "unsupported canvas schema version")
	}
	if limits.MaxNodes > 0 && len(document.Nodes) > limits.MaxNodes {
		return graphError("too_many_nodes", "", "", "canvas contains too many nodes")
	}
	if limits.MaxEdges > 0 && len(document.Edges) > limits.MaxEdges {
		return graphError("too_many_edges", "", "", "canvas contains too many edges")
	}
	nodes := make(map[string]Node, len(document.Nodes))
	for _, node := range document.Nodes {
		if strings.TrimSpace(node.ID) == "" || len(node.ID) > 64 {
			return graphError("invalid_node_id", node.ID, "", "canvas node ID is invalid")
		}
		if _, ok := nodes[node.ID]; ok {
			return graphError("duplicate_node", node.ID, "", "canvas node ID is duplicated")
		}
		if !isNodeType(node.Type) {
			return graphError("unsupported_node_type", node.ID, "", "canvas node type is unsupported")
		}
		if !isFinite(node.Position.X) || !isFinite(node.Position.Y) || math.Abs(node.Position.X) > maxCanvasCoordinate || math.Abs(node.Position.Y) > maxCanvasCoordinate {
			return graphError("invalid_node_position", node.ID, "", "canvas node position is outside the safety range")
		}
		minimum := minimumNodeSize(node.Type)
		if !isFinite(node.Size.Width) || !isFinite(node.Size.Height) || node.Size.Width < minimum.Width || node.Size.Height < minimum.Height {
			return graphError("invalid_node_size", node.ID, "", "canvas node size is below the supported minimum")
		}
		if isMediaNode(node.Type) && strings.TrimSpace(node.AssetID) == "" {
			return graphError("asset_required", node.ID, "", "media node must reference an asset")
		}
		if limits.MaxNodeBytes > 0 && len(node.Payload) > limits.MaxNodeBytes {
			return graphError("node_too_large", node.ID, "", "canvas node payload exceeds the size limit")
		}
		nodes[node.ID] = node
	}
	if limits.MaxDocumentBytes > 0 {
		raw, err := json.Marshal(document)
		if err != nil {
			return graphError("invalid_document", "", "", "canvas document cannot be encoded")
		}
		if len(raw) > limits.MaxDocumentBytes {
			return graphError("document_too_large", "", "", "canvas document exceeds the size limit")
		}
	}
	edges := make(map[string]struct{}, len(document.Edges))
	arcs := make([]GraphArc, 0, len(document.Edges))
	roleTargets := make(map[string]string)
	for _, edge := range document.Edges {
		if strings.TrimSpace(edge.ID) == "" || len(edge.ID) > 64 {
			return graphError("invalid_edge_id", "", edge.ID, "canvas edge ID is invalid")
		}
		if _, ok := edges[edge.ID]; ok {
			return graphError("duplicate_edge", "", edge.ID, "canvas edge ID is duplicated")
		}
		edges[edge.ID] = struct{}{}
		source, sourceOK := nodes[edge.Source]
		target, targetOK := nodes[edge.Target]
		if !sourceOK || !targetOK {
			return graphError("node_not_found", "", edge.ID, "canvas edge references a missing node")
		}
		if !isLegalConnection(source.Type, target.Type, edge.InputRole) {
			return graphError("illegal_connection", "", edge.ID, "canvas connection is not allowed")
		}
		if edge.InputRole == InputRoleFirstFrame || edge.InputRole == InputRoleLastFrame {
			key := edge.Target + "/" + string(edge.InputRole)
			if previous, ok := roleTargets[key]; ok && previous != edge.ID {
				return graphError("input_role_conflict", edge.Target, edge.ID, "video generation input role is already connected")
			}
			roleTargets[key] = edge.ID
		}
		arcs = append(arcs, GraphArc{Source: edge.Source, Target: edge.Target})
	}
	nodeIDs := make([]string, 0, len(nodes))
	for id := range nodes {
		nodeIDs = append(nodeIDs, id)
	}
	if HasDirectedCycle(nodeIDs, arcs) {
		return graphError("cycle", "", "", "canvas generation relationship cannot contain a cycle")
	}
	return nil
}

func minimumNodeSize(nodeType NodeType) Size {
	switch nodeType {
	case NodeTypeImage, NodeTypeVideo:
		return Size{Width: 220, Height: 160}
	case NodeTypeAudio:
		return Size{Width: 240, Height: 120}
	case NodeTypeImageGeneration, NodeTypeVideoGeneration:
		return Size{Width: 280, Height: 200}
	default:
		return Size{Width: 220, Height: 140}
	}
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func HasDirectedCycle(nodeIDs []string, arcs []GraphArc) bool {
	adjacency := make(map[string][]string, len(nodeIDs))
	for _, id := range nodeIDs {
		adjacency[id] = nil
	}
	for _, arc := range arcs {
		adjacency[arc.Source] = append(adjacency[arc.Source], arc.Target)
		if _, ok := adjacency[arc.Target]; !ok {
			adjacency[arc.Target] = nil
		}
	}
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make(map[string]int, len(adjacency))
	var visit func(string) bool
	visit = func(id string) bool {
		switch state[id] {
		case visiting:
			return true
		case visited:
			return false
		}
		state[id] = visiting
		for _, next := range adjacency[id] {
			if visit(next) {
				return true
			}
		}
		state[id] = visited
		return false
	}
	for id := range adjacency {
		if state[id] == unvisited && visit(id) {
			return true
		}
	}
	return false
}

func ExtractAssetReferences(document DocumentV1) []string {
	seen := make(map[string]struct{})
	for _, node := range document.Nodes {
		assetID := strings.TrimSpace(node.AssetID)
		if isMediaNode(node.Type) && assetID != "" {
			seen[assetID] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for assetID := range seen {
		result = append(result, assetID)
	}
	sort.Strings(result)
	return result
}

func StableResultNodeID(runID, assetID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(runID) + "\x00" + strings.TrimSpace(assetID)))
	return "result-" + hex.EncodeToString(digest[:12])
}

func isNodeType(nodeType NodeType) bool {
	switch nodeType {
	case NodeTypePrompt, NodeTypeImage, NodeTypeVideo, NodeTypeAudio, NodeTypeImageGeneration, NodeTypeVideoGeneration, NodeTypeNote:
		return true
	default:
		return false
	}
}

func isMediaNode(nodeType NodeType) bool {
	return nodeType == NodeTypeImage || nodeType == NodeTypeVideo || nodeType == NodeTypeAudio
}

func isLegalConnection(source, target NodeType, role InputRole) bool {
	switch {
	case source == NodeTypePrompt && (target == NodeTypeImageGeneration || target == NodeTypeVideoGeneration):
		return role == InputRolePrompt
	case source == NodeTypeImage && target == NodeTypeImageGeneration:
		return role == InputRoleReference
	case source == NodeTypeImage && target == NodeTypeVideoGeneration:
		return role == InputRoleFirstFrame || role == InputRoleLastFrame
	case (source == NodeTypeImageGeneration && target == NodeTypeImage) || (source == NodeTypeVideoGeneration && target == NodeTypeVideo):
		return role == InputRoleResult
	default:
		return false
	}
}

func graphError(code, nodeID, edgeID, detail string) *ValidationError {
	return &ValidationError{Code: code, NodeID: nodeID, EdgeID: edgeID, Detail: fmt.Sprintf("%s", detail)}
}
