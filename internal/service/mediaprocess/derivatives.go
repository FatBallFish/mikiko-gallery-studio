package mediaprocess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
)

const derivativeOutputMaxBytes int64 = 512 << 20

type DerivativeCommand struct {
	Kind             domainmedia.DerivativeKind
	TransformVersion int
	Args             []string
	OutputPath       string
}

type DerivativeOutput struct {
	Kind             domainmedia.DerivativeKind
	TransformVersion int
	Path             string
}

func BuildDerivativeCommands(mediaType domainmedia.MediaType, inputPath, outputDirectory string) ([]DerivativeCommand, error) {
	return BuildDerivativeCommandsWithPolicy(mediaType, inputPath, outputDirectory, domainmedia.DefaultPolicy())
}

func BuildDerivativeCommandsWithPolicy(mediaType domainmedia.MediaType, inputPath, outputDirectory string, policy domainmedia.Policy) ([]DerivativeCommand, error) {
	inputPath = filepath.Clean(strings.TrimSpace(inputPath))
	outputDirectory = filepath.Clean(strings.TrimSpace(outputDirectory))
	if !filepath.IsAbs(inputPath) || !filepath.IsAbs(outputDirectory) {
		return nil, errors.New("media derivative paths must be absolute")
	}
	base := []string{"-nostdin", "-y", "-protocol_whitelist", "file,pipe", "-i", inputPath}
	command := func(kind domainmedia.DerivativeKind, filename string, options ...string) DerivativeCommand {
		args := append(append([]string(nil), base...), options...)
		outputPath := filepath.Join(outputDirectory, filename)
		args = append(args, "-threads", "2", "-fs", fmt.Sprint(derivativeOutputMaxBytes), outputPath)
		return DerivativeCommand{Kind: kind, TransformVersion: 1, Args: args, OutputPath: outputPath}
	}
	all := map[domainmedia.DerivativeKind]DerivativeCommand{}
	switch mediaType {
	case domainmedia.MediaTypeImage:
		all[domainmedia.DerivativeThumbnail320] = command(domainmedia.DerivativeThumbnail320, "thumbnail-320.jpg", "-vf", "scale='min(320,iw)':-2", "-frames:v", "1", "-c:v", "mjpeg", "-q:v", "5")
		all[domainmedia.DerivativeThumbnail640] = command(domainmedia.DerivativeThumbnail640, "thumbnail-640.jpg", "-vf", "scale='min(640,iw)':-2", "-frames:v", "1", "-c:v", "mjpeg", "-q:v", "4")
		all[domainmedia.DerivativePreview1280] = command(domainmedia.DerivativePreview1280, "preview-1280.jpg", "-vf", "scale='min(1280,iw)':-2", "-frames:v", "1", "-c:v", "mjpeg", "-q:v", "3")
	case domainmedia.MediaTypeVideo:
		all[domainmedia.DerivativePoster] = command(domainmedia.DerivativePoster, "poster.jpg", "-ss", "0", "-frames:v", "1", "-vf", "scale='min(1280,iw)':-2", "-q:v", "3")
		all[domainmedia.DerivativeHoverPreview] = command(domainmedia.DerivativeHoverPreview, "hover.mp4", "-t", "3", "-an", "-vf", "scale='min(640,iw)':-2", "-c:v", "libx264", "-preset", "veryfast", "-crf", "27", "-pix_fmt", "yuv420p", "-movflags", "+faststart")
		all[domainmedia.DerivativeProxy] = command(domainmedia.DerivativeProxy, "proxy.mp4", "-vf", "scale='min(1280,iw)':-2", "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart")
	case domainmedia.MediaTypeAudio:
		all[domainmedia.DerivativeWaveform] = command(domainmedia.DerivativeWaveform, "waveform.png", "-filter_complex", "showwavespic=s=1280x240:colors=white", "-frames:v", "1")
		all[domainmedia.DerivativeProxy] = command(domainmedia.DerivativeProxy, "proxy.m4a", "-vn", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart")
	default:
		return nil, errors.New("unsupported media type for derivatives")
	}
	plan := domainmedia.BuildDerivativePlanWithPolicy(mediaType, policy)
	commands := make([]DerivativeCommand, 0, len(plan))
	for _, spec := range plan {
		if next, ok := all[spec.Kind]; ok {
			commands = append(commands, next)
		}
	}
	return commands, nil
}

type DerivativeProcessor struct {
	runner  Runner
	timeout time.Duration
}

func NewDerivativeProcessor(runner Runner, timeout time.Duration) *DerivativeProcessor {
	if runner == nil {
		runner = ExecRunner{}
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &DerivativeProcessor{runner: runner, timeout: timeout}
}

func (processor *DerivativeProcessor) Generate(ctx context.Context, mediaType domainmedia.MediaType, inputPath, outputDirectory string) ([]DerivativeOutput, error) {
	return processor.GenerateWithPolicy(ctx, mediaType, inputPath, outputDirectory, domainmedia.DefaultPolicy())
}

func (processor *DerivativeProcessor) GenerateWithPolicy(ctx context.Context, mediaType domainmedia.MediaType, inputPath, outputDirectory string, policy domainmedia.Policy) ([]DerivativeOutput, error) {
	commands, err := BuildDerivativeCommandsWithPolicy(mediaType, inputPath, outputDirectory, policy)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outputDirectory, 0o700); err != nil {
		return nil, err
	}
	outputs := make([]DerivativeOutput, 0, len(commands))
	for _, command := range commands {
		extension := filepath.Ext(command.OutputPath)
		temporaryPath := strings.TrimSuffix(command.OutputPath, extension) + ".partial-" + uuid.NewString() + extension
		args := append([]string(nil), command.Args...)
		args[len(args)-1] = temporaryPath
		commandCtx, cancel := context.WithTimeout(ctx, processor.timeout)
		_, runErr := processor.runner.Run(commandCtx, "ffmpeg", args...)
		cancel()
		if runErr != nil {
			_ = os.Remove(temporaryPath)
			return nil, fmt.Errorf("generate %s derivative: %w", command.Kind, runErr)
		}
		info, err := os.Stat(temporaryPath)
		if err != nil || info.Size() <= 0 || info.Size() > derivativeOutputMaxBytes {
			_ = os.Remove(temporaryPath)
			if err == nil {
				if info.Size() > derivativeOutputMaxBytes {
					err = errors.New("ffmpeg derivative exceeds the output size limit")
				} else {
					err = errors.New("ffmpeg produced an empty derivative")
				}
			}
			return nil, fmt.Errorf("verify %s derivative: %w", command.Kind, err)
		}
		if err := os.Rename(temporaryPath, command.OutputPath); err != nil {
			_ = os.Remove(temporaryPath)
			return nil, fmt.Errorf("commit %s derivative: %w", command.Kind, err)
		}
		outputs = append(outputs, DerivativeOutput{Kind: command.Kind, TransformVersion: command.TransformVersion, Path: command.OutputPath})
	}
	return outputs, nil
}
