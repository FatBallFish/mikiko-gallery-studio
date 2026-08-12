package mediaprocess

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
)

func TestDerivativeCommandsCoverImageVideoAndAudioP0Outputs(t *testing.T) {
	tests := []struct {
		mediaType domainmedia.MediaType
		wantKinds []domainmedia.DerivativeKind
	}{
		{domainmedia.MediaTypeImage, []domainmedia.DerivativeKind{domainmedia.DerivativeThumbnail320, domainmedia.DerivativeThumbnail640, domainmedia.DerivativePreview1280}},
		{domainmedia.MediaTypeVideo, []domainmedia.DerivativeKind{domainmedia.DerivativePoster, domainmedia.DerivativeHoverPreview, domainmedia.DerivativeProxy}},
		{domainmedia.MediaTypeAudio, []domainmedia.DerivativeKind{domainmedia.DerivativeWaveform, domainmedia.DerivativeProxy}},
	}
	for _, test := range tests {
		t.Run(string(test.mediaType), func(t *testing.T) {
			commands, err := BuildDerivativeCommands(test.mediaType, "/tmp/original", "/tmp/output")
			if err != nil {
				t.Fatal(err)
			}
			if len(commands) != len(test.wantKinds) {
				t.Fatalf("commands=%#v", commands)
			}
			for index, command := range commands {
				if command.Kind != test.wantKinds[index] || command.TransformVersion != 1 {
					t.Fatalf("command[%d]=%#v", index, command)
				}
				joined := strings.Join(command.Args, " ")
				if !strings.Contains(joined, "-nostdin") || !strings.Contains(joined, "-protocol_whitelist file,pipe") || !strings.Contains(joined, "-y") {
					t.Fatalf("unsafe ffmpeg args: %q", joined)
				}
				if filepath.Dir(command.OutputPath) != "/tmp/output" {
					t.Fatalf("output escaped target directory: %q", command.OutputPath)
				}
				if test.mediaType == domainmedia.MediaTypeImage && (filepath.Ext(command.OutputPath) != ".jpg" || !strings.Contains(joined, "-c:v mjpeg")) {
					t.Fatalf("image derivatives must use the broadly available JPEG encoder: %#v", command)
				}
			}
		})
	}
}

func TestProcessorUsesTimeoutAndAtomicOutputPaths(t *testing.T) {
	runner := &recordingDerivativeRunner{}
	processor := NewDerivativeProcessor(runner, 100*time.Millisecond)
	outputs, err := processor.Generate(t.Context(), domainmedia.MediaTypeVideo, "/tmp/input.mp4", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 3 || len(runner.calls) != 3 {
		t.Fatalf("outputs=%#v calls=%#v", outputs, runner.calls)
	}
	for _, call := range runner.calls {
		if !strings.Contains(strings.Join(call, " "), ".partial-") {
			t.Fatalf("runner must write a temporary output before rename: %#v", call)
		}
	}
}

type recordingDerivativeRunner struct{ calls [][]string }

func (runner *recordingDerivativeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.calls = append(runner.calls, append([]string{name}, args...))
	output := args[len(args)-1]
	return nil, os.WriteFile(output, []byte("fixture"), 0o600)
}
