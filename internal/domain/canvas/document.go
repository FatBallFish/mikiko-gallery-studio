package canvas

import "encoding/json"

type NodeType string

const (
	NodeTypePrompt          NodeType = "prompt"
	NodeTypeImage           NodeType = "image"
	NodeTypeVideo           NodeType = "video"
	NodeTypeAudio           NodeType = "audio"
	NodeTypeImageGeneration NodeType = "image_generation"
	NodeTypeVideoGeneration NodeType = "video_generation"
	NodeTypeNote            NodeType = "note"
)

type InputRole string

const (
	InputRolePrompt     InputRole = "prompt"
	InputRoleReference  InputRole = "reference"
	InputRoleFirstFrame InputRole = "first_frame"
	InputRoleLastFrame  InputRole = "last_frame"
	InputRoleResult     InputRole = "result"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Size struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

type Node struct {
	ID       string          `json:"id"`
	Type     NodeType        `json:"type"`
	AssetID  string          `json:"asset_id,omitempty"`
	Position Point           `json:"position"`
	Size     Size            `json:"size"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

type Edge struct {
	ID           string    `json:"id"`
	Source       string    `json:"source"`
	Target       string    `json:"target"`
	SourceHandle string    `json:"source_handle,omitempty"`
	TargetHandle string    `json:"target_handle,omitempty"`
	InputRole    InputRole `json:"input_role"`
	Ordinal      int       `json:"ordinal,omitempty"`
}

type DocumentV1 struct {
	SchemaVersion int      `json:"schema_version"`
	Viewport      Viewport `json:"viewport"`
	Nodes         []Node   `json:"nodes"`
	Edges         []Edge   `json:"edges"`
}

type Limits struct {
	MaxNodes         int
	MaxEdges         int
	MaxDocumentBytes int
	MaxNodeBytes     int
}

func DefaultLimits() Limits {
	return Limits{MaxNodes: 500, MaxEdges: 1000, MaxDocumentBytes: 1 << 20, MaxNodeBytes: 64 << 10}
}

type ValidationError struct {
	Code   string
	NodeID string
	EdgeID string
	Detail string
}

func (err *ValidationError) Error() string {
	if err == nil {
		return ""
	}
	return err.Detail
}
