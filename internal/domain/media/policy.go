package media

import (
	"path"
	"path/filepath"
	"strings"
)

const SingleFileHardMaxBytes int64 = 1 << 30

type Policy struct {
	SingleFileMaxBytes      int64
	VideoMaxDurationMS      int64
	AllowedFormats          map[MediaType][]string
	AllowedMIMETypes        map[MediaType][]string
	AllowedVideoCodecs      []string
	AllowedVideoAudioCodecs []string
}

func DefaultPolicy() Policy {
	return Policy{
		SingleFileMaxBytes: SingleFileHardMaxBytes,
		AllowedFormats: map[MediaType][]string{
			MediaTypeImage: {"jpg", "jpeg", "png", "webp", "heic", "heif", "bmp", "tiff", "gif"},
			MediaTypeVideo: {"mp4", "mov"},
			MediaTypeAudio: {"mp3", "m4a", "wav"},
		},
		AllowedMIMETypes: map[MediaType][]string{
			MediaTypeImage: {"image/jpeg", "image/png", "image/webp", "image/heic", "image/heif", "image/bmp", "image/tiff", "image/gif"},
			MediaTypeVideo: {"video/mp4", "video/quicktime"},
			MediaTypeAudio: {"audio/mpeg", "audio/mp4", "audio/x-m4a", "audio/wav", "audio/x-wav"},
		},
		AllowedVideoCodecs:      []string{"h264", "avc", "avc1", "h265", "hevc"},
		AllowedVideoAudioCodecs: []string{"aac", "mp3"},
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
	var kinds []DerivativeKind
	switch mediaType {
	case MediaTypeImage:
		kinds = []DerivativeKind{DerivativeThumbnail320, DerivativeThumbnail640, DerivativePreview1280}
	case MediaTypeVideo:
		kinds = []DerivativeKind{DerivativePoster, DerivativeHoverPreview, DerivativeProxy}
	case MediaTypeAudio:
		kinds = []DerivativeKind{DerivativeWaveform, DerivativeProxy}
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
