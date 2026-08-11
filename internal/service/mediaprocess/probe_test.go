package mediaprocess

import (
	"context"
	"strings"
	"testing"
	"time"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
)

func TestProbeParsesVideoMetadataAndRestrictsProtocols(t *testing.T) {
	runner := &recordingRunner{output: []byte(`{
  "format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"5.125","size":"12345"},
  "streams":[
    {"codec_type":"video","codec_name":"h264","width":1920,"height":1080,"avg_frame_rate":"30000/1001"},
    {"codec_type":"audio","codec_name":"aac","channels":2,"sample_rate":"48000"}
  ]
}`)}
	probe := NewProbe(runner, 2*time.Second)
	result, err := probe.Inspect(t.Context(), "/tmp/input.mp4", 12345, domainmedia.MediaTypeVideo)
	if err != nil {
		t.Fatal(err)
	}
	if result.MediaType != domainmedia.MediaTypeVideo || result.Format != "mp4" || result.Container != "mp4" || result.VideoCodec != "h264" || result.AudioCodec != "aac" {
		t.Fatalf("unexpected probe result: %#v", result)
	}
	if result.Width != 1920 || result.Height != 1080 || result.DurationMS != 5125 || result.FrameRateMilli != 29970 || result.Channels != 2 || result.SampleRate != 48000 {
		t.Fatalf("unexpected numeric probe metadata: %#v", result)
	}
	joined := strings.Join(runner.args, " ")
	for _, required := range []string{"-v error", "-protocol_whitelist file,pipe", "-show_format", "-show_streams", "-of json", "/tmp/input.mp4"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("ffprobe args %q missing %q", joined, required)
		}
	}
}

func TestProbeRejectsMalformedOrMismatchedContent(t *testing.T) {
	malformed := NewProbe(&recordingRunner{output: []byte(`{"streams":`)}, time.Second)
	if _, err := malformed.Inspect(t.Context(), "/tmp/bad.mp4", 10, domainmedia.MediaTypeVideo); err == nil {
		t.Fatal("expected malformed ffprobe JSON")
	}
	audio := NewProbe(&recordingRunner{output: []byte(`{
  "format":{"format_name":"mp3","duration":"1.0","size":"10"},
  "streams":[{"codec_type":"audio","codec_name":"mp3","channels":2,"sample_rate":"44100"}]
}`)}, time.Second)
	result, err := audio.Inspect(t.Context(), "/tmp/disguised.png", 10, domainmedia.MediaTypeImage)
	if err != nil {
		t.Fatal(err)
	}
	if result.MediaType != domainmedia.MediaTypeAudio {
		t.Fatalf("probe must report detected content, got %#v", result)
	}
}

type recordingRunner struct {
	name   string
	args   []string
	output []byte
	err    error
}

func (runner *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.name = name
	runner.args = append([]string(nil), args...)
	return runner.output, runner.err
}
