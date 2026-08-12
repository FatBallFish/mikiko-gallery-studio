package media

type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
	MediaTypeAudio MediaType = "audio"
)

type UploadDeclaration struct {
	Filename  string
	MediaType MediaType
	MIMEType  string
	SizeBytes int64
}

type ProbeResult struct {
	MediaType      MediaType
	Format         string
	Container      string
	VideoCodec     string
	AudioCodec     string
	SizeBytes      int64
	Width          int
	Height         int
	DurationMS     int64
	StreamCount    int
	FrameRateMilli int
	Channels       int
	SampleRate     int
}

type ValidationError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (err *ValidationError) Error() string {
	if err == nil {
		return ""
	}
	return err.Message
}

type DerivativeKind string

const (
	DerivativeThumbnail320 DerivativeKind = "thumbnail_320"
	DerivativeThumbnail640 DerivativeKind = "thumbnail_640"
	DerivativePreview1280  DerivativeKind = "preview_1280"
	DerivativePoster       DerivativeKind = "poster"
	DerivativeHoverPreview DerivativeKind = "hover_preview"
	DerivativeProxy        DerivativeKind = "proxy"
	DerivativeWaveform     DerivativeKind = "waveform"
)

type DerivativeSpec struct {
	Kind             DerivativeKind
	TransformVersion int
}
