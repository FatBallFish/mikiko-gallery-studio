package media

import (
	"path"
	"path/filepath"
	"strings"
)

const SingleFileHardMaxBytes int64 = 1 << 30

const (
	MediaMaxDimension             = 8192
	MediaMaxPixels          int64 = 40_000_000
	MediaMaxStreams               = 8
	MediaMaxFrameRateMilli        = 120_000
	MediaMaxAudioChannels         = 8
	MediaMaxAudioSampleRate       = 192_000
)

type Policy struct {
	SingleFileMaxBytes       int64
	VideoMaxDurationMS       int64
	AllowedFormats           map[MediaType][]string
	AllowedMIMETypes         map[MediaType][]string
	AllowedVideoCodecs       []string
	AllowedVideoAudioCodecs  []string
	ImageThumbnailWidths     []int
	VideoPosterEnabled       bool
	VideoHoverPreviewEnabled bool
	VideoProxyEnabled        bool
	AudioProxyEnabled        bool
	AudioWaveformEnabled     bool
}

func DefaultPolicy() Policy {
	return Policy{
		SingleFileMaxBytes: SingleFileHardMaxBytes,
		AllowedFormats: map[MediaType][]string{
			MediaTypeImage: {"jpg", "jpeg", "png", "webp"},
			MediaTypeVideo: {"mp4"},
			MediaTypeAudio: {"mp3", "m4a", "wav"},
		},
		AllowedMIMETypes: map[MediaType][]string{
			MediaTypeImage: {"image/jpeg", "image/png", "image/webp"},
			MediaTypeVideo: {"video/mp4"},
			MediaTypeAudio: {"audio/mpeg", "audio/mp4", "audio/x-m4a", "audio/wav", "audio/x-wav"},
		},
		AllowedVideoCodecs:       []string{"h264", "avc", "avc1", "h265", "hevc"},
		AllowedVideoAudioCodecs:  []string{"aac", "mp3"},
		ImageThumbnailWidths:     []int{320, 640, 1280},
		VideoPosterEnabled:       true,
		VideoHoverPreviewEnabled: true,
		VideoProxyEnabled:        true,
		AudioProxyEnabled:        true,
		AudioWaveformEnabled:     true,
	}
}

func (policy Policy) ValidateDeclaration(declaration UploadDeclaration) *ValidationError {
	if declaration.SizeBytes <= 0 {
		return validationError("size_bytes", "invalid", "file size must be positive")
	}
	limit := policy.SingleFileMaxBytes
	if limit <= 0 || limit > SingleFileHardMaxBytes {
		limit = SingleFileHardMaxBytes
	}
	if declaration.SizeBytes > limit {
		return validationError("size_bytes", "too_large", "file exceeds the configured size limit")
	}
	formats, ok := policy.AllowedFormats[declaration.MediaType]
	if !ok {
		return validationError("media_type", "unsupported", "media type is not supported")
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(strings.TrimSpace(declaration.Filename))), ".")
	if extension == "" || !containsFold(formats, extension) {
		return validationError("filename", "unsupported_format", "file extension is not supported")
	}
	if !containsFold(policy.AllowedMIMETypes[declaration.MediaType], declaration.MIMEType) {
		return validationError("mime_type", "unsupported", "declared MIME type is not supported")
	}
	return nil
}

func (policy Policy) ValidateProbe(declaration UploadDeclaration, probe ProbeResult) *ValidationError {
	if probe.MediaType != declaration.MediaType {
		return validationError("media_type", "content_mismatch", "detected media type does not match the declaration")
	}
	if !containsFold(policy.AllowedFormats[probe.MediaType], probe.Format) && !containsFold(policy.AllowedFormats[probe.MediaType], probe.Container) {
		return validationError("format", "unsupported", "detected media format is not supported")
	}
	if probe.SizeBytes > 0 {
		limit := policy.SingleFileMaxBytes
		if limit <= 0 || limit > SingleFileHardMaxBytes {
			limit = SingleFileHardMaxBytes
		}
		if probe.SizeBytes > limit {
			return validationError("size_bytes", "too_large", "detected file size exceeds the configured limit")
		}
	}
	if probe.Width > MediaMaxDimension || probe.Height > MediaMaxDimension {
		return validationError("dimensions", "resource_limit", "detected media dimensions exceed the processing limit")
	}
	if probe.Width > 0 && probe.Height > 0 && int64(probe.Width)*int64(probe.Height) > MediaMaxPixels {
		return validationError("pixels", "resource_limit", "detected media pixel count exceeds the processing limit")
	}
	if probe.StreamCount > MediaMaxStreams {
		return validationError("stream_count", "resource_limit", "detected media stream count exceeds the processing limit")
	}
	if probe.FrameRateMilli > MediaMaxFrameRateMilli {
		return validationError("frame_rate", "resource_limit", "detected media frame rate exceeds the processing limit")
	}
	if probe.Channels > MediaMaxAudioChannels {
		return validationError("channels", "resource_limit", "detected media channel count exceeds the processing limit")
	}
	if probe.SampleRate > MediaMaxAudioSampleRate {
		return validationError("sample_rate", "resource_limit", "detected media sample rate exceeds the processing limit")
	}
	if probe.MediaType == MediaTypeVideo {
		if !containsFold(policy.AllowedVideoCodecs, probe.VideoCodec) {
			return validationError("video_codec", "unsupported", "video codec is not supported")
		}
		if probe.AudioCodec != "" && !containsFold(policy.AllowedVideoAudioCodecs, probe.AudioCodec) {
			return validationError("audio_codec", "unsupported", "video audio codec is not supported")
		}
		if policy.VideoMaxDurationMS > 0 && probe.DurationMS > policy.VideoMaxDurationMS {
			return validationError("duration_ms", "too_long", "video duration exceeds the configured limit")
		}
	}
	return nil
}

func BuildDerivativePlan(mediaType MediaType) []DerivativeSpec {
	return BuildDerivativePlanWithPolicy(mediaType, DefaultPolicy())
}

func BuildDerivativePlanWithPolicy(mediaType MediaType, policy Policy) []DerivativeSpec {
	var kinds []DerivativeKind
	switch mediaType {
	case MediaTypeImage:
		for _, width := range policy.ImageThumbnailWidths {
			switch width {
			case 320:
				kinds = append(kinds, DerivativeThumbnail320)
			case 640:
				kinds = append(kinds, DerivativeThumbnail640)
			case 1280:
				kinds = append(kinds, DerivativePreview1280)
			}
		}
	case MediaTypeVideo:
		if policy.VideoPosterEnabled {
			kinds = append(kinds, DerivativePoster)
		}
		if policy.VideoHoverPreviewEnabled {
			kinds = append(kinds, DerivativeHoverPreview)
		}
		if policy.VideoProxyEnabled {
			kinds = append(kinds, DerivativeProxy)
		}
	case MediaTypeAudio:
		if policy.AudioWaveformEnabled {
			kinds = append(kinds, DerivativeWaveform)
		}
		if policy.AudioProxyEnabled {
			kinds = append(kinds, DerivativeProxy)
		}
	default:
		return nil
	}
	result := make([]DerivativeSpec, 0, len(kinds))
	for _, kind := range kinds {
		result = append(result, DerivativeSpec{Kind: kind, TransformVersion: 1})
	}
	return result
}

var controlledObjectPrefixes = []string{
	"media/original/",
	"media/derivatives/",
	"media/uploads/",
	"canvas/previews/",
}

func IsControlledObjectKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "/") || path.Clean(key) != key {
		return false
	}
	for _, prefix := range controlledObjectPrefixes {
		if strings.HasPrefix(key, prefix) && len(key) > len(prefix) {
			return true
		}
	}
	return false
}

func validationError(field, code, message string) *ValidationError {
	return &ValidationError{Field: field, Code: code, Message: message}
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}
