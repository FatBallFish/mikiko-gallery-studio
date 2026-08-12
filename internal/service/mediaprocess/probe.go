package mediaprocess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	var output cappedBuffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return output.Bytes(), ctxErr
		}
		return output.Bytes(), fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(output.String()))
	}
	return output.Bytes(), nil
}

type cappedBuffer struct{ bytes.Buffer }

func (buffer *cappedBuffer) Write(payload []byte) (int, error) {
	const maximum = 64 << 10
	original := len(payload)
	remaining := maximum - buffer.Len()
	if remaining > 0 {
		if len(payload) > remaining {
			payload = payload[:remaining]
		}
		_, _ = buffer.Buffer.Write(payload)
	}
	return original, nil
}

type Probe struct {
	runner  Runner
	timeout time.Duration
}

func NewProbe(runner Runner, timeout time.Duration) *Probe {
	if runner == nil {
		runner = ExecRunner{}
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Probe{runner: runner, timeout: timeout}
}

func (probe *Probe) Inspect(ctx context.Context, inputPath string, sizeBytes int64, declaredType domainmedia.MediaType) (domainmedia.ProbeResult, error) {
	inputPath = filepath.Clean(strings.TrimSpace(inputPath))
	if !filepath.IsAbs(inputPath) || strings.ContainsRune(inputPath, '\x00') {
		return domainmedia.ProbeResult{}, errors.New("media probe input must be an absolute local path")
	}
	probeCtx, cancel := context.WithTimeout(ctx, probe.timeout)
	defer cancel()
	output, err := probe.runner.Run(probeCtx, "ffprobe",
		"-v", "error", "-protocol_whitelist", "file,pipe",
		"-show_format", "-show_streams", "-of", "json", inputPath,
	)
	if err != nil {
		return domainmedia.ProbeResult{}, fmt.Errorf("inspect media: %w", err)
	}
	return parseProbeOutput(output, inputPath, sizeBytes, declaredType)
}

type ffprobePayload struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
	} `json:"format"`
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
		Channels     int    `json:"channels"`
		SampleRate   string `json:"sample_rate"`
	} `json:"streams"`
}

func parseProbeOutput(output []byte, inputPath string, sizeBytes int64, declaredType domainmedia.MediaType) (domainmedia.ProbeResult, error) {
	var payload ffprobePayload
	decoder := json.NewDecoder(bytes.NewReader(output))
	if err := decoder.Decode(&payload); err != nil {
		return domainmedia.ProbeResult{}, fmt.Errorf("decode ffprobe output: %w", err)
	}
	if len(payload.Streams) == 0 {
		return domainmedia.ProbeResult{}, errors.New("ffprobe output contains no streams")
	}
	result := domainmedia.ProbeResult{SizeBytes: sizeBytes}
	formatNames := strings.Split(strings.ToLower(payload.Format.FormatName), ",")
	result.Container, result.Format = normalizeProbeFormat(formatNames, inputPath)
	hasVideo, hasAudio := false, false
	for _, stream := range payload.Streams {
		switch strings.ToLower(stream.CodecType) {
		case "video":
			hasVideo = true
			result.VideoCodec = normalizeCodec(stream.CodecName)
			result.Width, result.Height = stream.Width, stream.Height
			result.FrameRateMilli = rationalMilli(stream.AvgFrameRate)
		case "audio":
			hasAudio = true
			result.AudioCodec = normalizeCodec(stream.CodecName)
			result.Channels = stream.Channels
			result.SampleRate, _ = strconv.Atoi(stream.SampleRate)
		}
	}
	if hasAudio && !hasVideo {
		result.MediaType = domainmedia.MediaTypeAudio
	} else if hasVideo && isStillImageFormat(result.Format, result.Container, inputPath) {
		result.MediaType = domainmedia.MediaTypeImage
	} else if hasVideo {
		result.MediaType = domainmedia.MediaTypeVideo
	} else {
		result.MediaType = declaredType
	}
	if duration, err := strconv.ParseFloat(payload.Format.Duration, 64); err == nil && duration > 0 {
		result.DurationMS = int64(duration*1000 + 0.5)
	}
	if parsedSize, err := strconv.ParseInt(payload.Format.Size, 10, 64); err == nil && parsedSize > 0 {
		result.SizeBytes = parsedSize
	}
	return result, nil
}

func normalizeProbeFormat(names []string, inputPath string) (container, format string) {
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(inputPath)), ".")
	for _, name := range names {
		name = strings.TrimSpace(name)
		switch name {
		case "mp3", "wav", "webp", "png", "gif", "jpeg", "jpg", "bmp", "tiff", "heic", "heif":
			if name == "jpeg" {
				name = "jpg"
			}
			return name, name
		case "mov", "mp4", "m4a", "3gp", "3g2", "mj2":
			if extension == "mov" {
				return "mov", "mov"
			}
			if extension == "m4a" {
				return "m4a", "m4a"
			}
			return "mp4", "mp4"
		case "image2", "image2pipe":
			if extension == "jpeg" {
				extension = "jpg"
			}
			return extension, extension
		}
	}
	if len(names) > 0 {
		first := strings.TrimSpace(names[0])
		return first, first
	}
	return extension, extension
}

func normalizeCodec(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "avc", "avc1":
		return "h264"
	case "h265":
		return "hevc"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func rationalMilli(value string) int {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 2 {
		parsed, _ := strconv.ParseFloat(value, 64)
		return int(parsed*1000 + 0.5)
	}
	numerator, numeratorErr := strconv.ParseFloat(parts[0], 64)
	denominator, denominatorErr := strconv.ParseFloat(parts[1], 64)
	if numeratorErr != nil || denominatorErr != nil || denominator == 0 {
		return 0
	}
	return int(numerator/denominator*1000 + 0.5)
}

func isStillImageFormat(format, container, inputPath string) bool {
	value := strings.ToLower(format + "," + container + "," + filepath.Ext(inputPath))
	for _, token := range []string{"jpg", "jpeg", "png", "webp", "gif", "bmp", "tiff", "heic", "heif"} {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}
