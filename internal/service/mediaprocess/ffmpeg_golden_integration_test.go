package mediaprocess

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
)

func TestFFmpegGoldenMediaProbeAndP0Derivatives(t *testing.T) {
	ffmpeg, ffprobe := requireFFmpegTools(t)
	fixtures := createGoldenMediaFixtures(t, ffmpeg)
	runner := mappedGoldenRunner{ffmpeg: ffmpeg, ffprobe: ffprobe}
	probe := NewProbe(runner, 15*time.Second)
	processor := NewDerivativeProcessor(runner, 30*time.Second)

	tests := []struct {
		name      string
		mediaType domainmedia.MediaType
		path      string
		wantKinds []domainmedia.DerivativeKind
	}{
		{"image", domainmedia.MediaTypeImage, fixtures.image, []domainmedia.DerivativeKind{domainmedia.DerivativeThumbnail320, domainmedia.DerivativeThumbnail640, domainmedia.DerivativePreview1280}},
		{"video", domainmedia.MediaTypeVideo, fixtures.video, []domainmedia.DerivativeKind{domainmedia.DerivativePoster, domainmedia.DerivativeHoverPreview, domainmedia.DerivativeProxy}},
		{"audio", domainmedia.MediaTypeAudio, fixtures.audio, []domainmedia.DerivativeKind{domainmedia.DerivativeWaveform, domainmedia.DerivativeProxy}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info, err := os.Stat(test.path)
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := probe.Inspect(t.Context(), test.path, info.Size(), test.mediaType)
			if err != nil || metadata.MediaType != test.mediaType {
				t.Fatalf("probe=%#v err=%v", metadata, err)
			}
			outputs, err := processor.Generate(t.Context(), test.mediaType, test.path, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if len(outputs) != len(test.wantKinds) {
				t.Fatalf("outputs=%#v", outputs)
			}
			for index, output := range outputs {
				if output.Kind != test.wantKinds[index] {
					t.Fatalf("output[%d]=%#v want kind %s", index, output, test.wantKinds[index])
				}
				outputInfo, statErr := os.Stat(output.Path)
				if statErr != nil || outputInfo.Size() <= 0 {
					t.Fatalf("invalid derivative %s: size=%d err=%v", output.Path, outputInfo.Size(), statErr)
				}
				if _, runErr := runner.Run(t.Context(), "ffprobe", "-v", "error", "-show_streams", "-of", "json", output.Path); runErr != nil {
					t.Fatalf("derivative %s is not probeable: %v", output.Kind, runErr)
				}
			}
		})
	}
}

type goldenMediaFixtures struct{ image, video, audio string }

func createGoldenMediaFixtures(t *testing.T, ffmpeg string) goldenMediaFixtures {
	t.Helper()
	directory := t.TempDir()
	fixtures := goldenMediaFixtures{
		image: filepath.Join(directory, "image.png"),
		video: filepath.Join(directory, "video.mp4"),
		audio: filepath.Join(directory, "audio.wav"),
	}
	runGoldenCommand(t, ffmpeg, "-nostdin", "-y", "-f", "lavfi", "-i", "color=c=blue:s=96x64:d=0.1", "-frames:v", "1", fixtures.image)
	runGoldenCommand(t, ffmpeg, "-nostdin", "-y", "-f", "lavfi", "-i", "testsrc2=s=160x90:r=12:d=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-shortest", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", fixtures.video)
	runGoldenCommand(t, ffmpeg, "-nostdin", "-y", "-f", "lavfi", "-i", "sine=frequency=880:duration=1", "-c:a", "pcm_s16le", fixtures.audio)
	return fixtures
}

func requireFFmpegTools(t *testing.T) (string, string) {
	t.Helper()
	ffmpeg, ffmpegErr := exec.LookPath("ffmpeg")
	ffprobe, ffprobeErr := exec.LookPath("ffprobe")
	if ffmpegErr != nil || ffprobeErr != nil {
		t.Skip("FFmpeg golden integration requires ffmpeg and ffprobe")
	}
	return ffmpeg, ffprobe
}

func runGoldenCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, name, args...).CombinedOutput(); err != nil {
		t.Fatalf("create golden media: %v: %s", err, output)
	}
}

type mappedGoldenRunner struct{ ffmpeg, ffprobe string }

func (runner mappedGoldenRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "ffmpeg" {
		name = runner.ffmpeg
	} else if name == "ffprobe" {
		name = runner.ffprobe
	}
	return (ExecRunner{}).Run(ctx, name, args...)
}
