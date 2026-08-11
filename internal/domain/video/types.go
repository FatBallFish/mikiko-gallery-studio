package video

type TaskType string

const (
	TaskTypeTextToVideo           TaskType = "text_to_video"
	TaskTypeImageToVideo          TaskType = "image_to_video"
	TaskTypeFirstLastFrameToVideo TaskType = "first_last_frame_to_video"
)

type Resolution string

const (
	Resolution480P  Resolution = "480p"
	Resolution720P  Resolution = "720p"
	Resolution768P  Resolution = "768p"
	Resolution1080P Resolution = "1080p"
	Resolution2K    Resolution = "2k"
	Resolution4K    Resolution = "4k"
)

type AspectRatio string

const (
	AspectRatio21x9     AspectRatio = "21:9"
	AspectRatio16x9     AspectRatio = "16:9"
	AspectRatio4x3      AspectRatio = "4:3"
	AspectRatio1x1      AspectRatio = "1:1"
	AspectRatio3x4      AspectRatio = "3:4"
	AspectRatio9x16     AspectRatio = "9:16"
	AspectRatioAdaptive AspectRatio = "adaptive"
)

type AudioMode string

const (
	AudioModeSilent    AudioMode = "silent"
	AudioModeGenerated AudioMode = "generated"
)

type InputRole string

const (
	InputRoleFirstFrame InputRole = "first_frame"
	InputRoleLastFrame  InputRole = "last_frame"
)

type Input struct {
	AssetID   string
	Role      InputRole
	Ordinal   int
	MediaType string
	Format    string
	SizeBytes int64
	Width     int
	Height    int
}

type Request struct {
	TaskType        TaskType
	Prompt          string
	DurationSeconds int
	Resolution      Resolution
	AspectRatio     AspectRatio
	AudioMode       AudioMode
	OutputCount     int
	Inputs          []Input
}

type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type MatchResult struct {
	Matches     bool
	FieldErrors []FieldError
}
