package media

import "testing"

func TestPolicyEnforcesOneGiBHardLimitAndDeclaredFormat(t *testing.T) {
	policy := DefaultPolicy()

	if err := policy.ValidateDeclaration(UploadDeclaration{
		Filename:  "clip.mp4",
		MediaType: MediaTypeVideo,
		MIMEType:  "video/mp4",
		SizeBytes: 1 << 30,
	}); err != nil {
		t.Fatalf("valid declaration rejected: %v", err)
	}

	tooLarge := UploadDeclaration{Filename: "clip.mp4", MediaType: MediaTypeVideo, MIMEType: "video/mp4", SizeBytes: (1 << 30) + 1}
	if err := policy.ValidateDeclaration(tooLarge); err == nil || err.Field != "size_bytes" || err.Code != "too_large" {
		t.Fatalf("oversized declaration error = %#v", err)
	}

	unsupported := UploadDeclaration{Filename: "clip.avi", MediaType: MediaTypeVideo, MIMEType: "video/x-msvideo", SizeBytes: 1024}
	if err := policy.ValidateDeclaration(unsupported); err == nil || err.Field != "filename" || err.Code != "unsupported_format" {
		t.Fatalf("unsupported declaration error = %#v", err)
	}
}

func TestPolicyUsesProbeAsFinalMediaAuthority(t *testing.T) {
	policy := DefaultPolicy()
	declared := UploadDeclaration{Filename: "portrait.png", MediaType: MediaTypeImage, MIMEType: "image/png", SizeBytes: 4096}

	if err := policy.ValidateProbe(declared, ProbeResult{MediaType: MediaTypeVideo, Format: "mp4", Container: "mp4", VideoCodec: "h264"}); err == nil || err.Field != "media_type" {
		t.Fatalf("disguised video error = %#v", err)
	}

	unsupportedContainer := UploadDeclaration{Filename: "clip.mov", MediaType: MediaTypeVideo, MIMEType: "video/quicktime", SizeBytes: 4096}
	if err := policy.ValidateDeclaration(unsupportedContainer); err == nil || err.Field != "filename" || err.Code != "unsupported_format" {
		t.Fatalf("unsupported video container error = %#v", err)
	}

	video := UploadDeclaration{Filename: "clip.mp4", MediaType: MediaTypeVideo, MIMEType: "video/mp4", SizeBytes: 4096}
	if err := policy.ValidateProbe(video, ProbeResult{MediaType: MediaTypeVideo, Format: "mp4", Container: "mp4", VideoCodec: "hevc", AudioCodec: "aac", DurationMS: 5000}); err != nil {
		t.Fatalf("supported MP4/HEVC probe rejected: %v", err)
	}
}

func TestDefaultPolicyAllowsOnlyP0UploadFormats(t *testing.T) {
	policy := DefaultPolicy()
	valid := []UploadDeclaration{
		{Filename: "photo.jpg", MediaType: MediaTypeImage, MIMEType: "image/jpeg", SizeBytes: 1},
		{Filename: "photo.jpeg", MediaType: MediaTypeImage, MIMEType: "image/jpeg", SizeBytes: 1},
		{Filename: "photo.png", MediaType: MediaTypeImage, MIMEType: "image/png", SizeBytes: 1},
		{Filename: "photo.webp", MediaType: MediaTypeImage, MIMEType: "image/webp", SizeBytes: 1},
		{Filename: "clip.mp4", MediaType: MediaTypeVideo, MIMEType: "video/mp4", SizeBytes: 1},
		{Filename: "track.mp3", MediaType: MediaTypeAudio, MIMEType: "audio/mpeg", SizeBytes: 1},
		{Filename: "track.m4a", MediaType: MediaTypeAudio, MIMEType: "audio/mp4", SizeBytes: 1},
		{Filename: "track.wav", MediaType: MediaTypeAudio, MIMEType: "audio/wav", SizeBytes: 1},
	}
	for _, declaration := range valid {
		if err := policy.ValidateDeclaration(declaration); err != nil {
			t.Errorf("valid P0 declaration %s rejected: %v", declaration.Filename, err)
		}
	}
	for _, declaration := range []UploadDeclaration{
		{Filename: "photo.gif", MediaType: MediaTypeImage, MIMEType: "image/gif", SizeBytes: 1},
		{Filename: "photo.heic", MediaType: MediaTypeImage, MIMEType: "image/heic", SizeBytes: 1},
		{Filename: "clip.mov", MediaType: MediaTypeVideo, MIMEType: "video/quicktime", SizeBytes: 1},
	} {
		if err := policy.ValidateDeclaration(declaration); err == nil {
			t.Errorf("non-P0 declaration %s unexpectedly accepted", declaration.Filename)
		}
	}
}

func TestPolicyRejectsMediaBombMetadata(t *testing.T) {
	policy := DefaultPolicy()
	declaration := UploadDeclaration{Filename: "clip.mp4", MediaType: MediaTypeVideo, MIMEType: "video/mp4", SizeBytes: 4096}
	base := ProbeResult{MediaType: MediaTypeVideo, Format: "mp4", Container: "mp4", VideoCodec: "h264", Width: 3840, Height: 2160, StreamCount: 2, FrameRateMilli: 60000, Channels: 2, SampleRate: 48000}
	tests := []struct {
		name  string
		patch func(*ProbeResult)
		field string
	}{
		{name: "dimension", patch: func(result *ProbeResult) { result.Width = 8193 }, field: "dimensions"},
		{name: "pixels", patch: func(result *ProbeResult) { result.Width, result.Height = 8000, 6000 }, field: "pixels"},
		{name: "streams", patch: func(result *ProbeResult) { result.StreamCount = 9 }, field: "stream_count"},
		{name: "frame rate", patch: func(result *ProbeResult) { result.FrameRateMilli = 120001 }, field: "frame_rate"},
		{name: "channels", patch: func(result *ProbeResult) { result.Channels = 9 }, field: "channels"},
		{name: "sample rate", patch: func(result *ProbeResult) { result.SampleRate = 192001 }, field: "sample_rate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := base
			test.patch(&result)
			if err := policy.ValidateProbe(declaration, result); err == nil || err.Field != test.field || err.Code != "resource_limit" {
				t.Fatalf("ValidateProbe error = %#v, want %s resource_limit", err, test.field)
			}
		})
	}
}

func TestDerivativePlanAvoidsOriginalsOnListSurfaces(t *testing.T) {
	tests := []struct {
		mediaType MediaType
		want      []DerivativeKind
	}{
		{mediaType: MediaTypeImage, want: []DerivativeKind{DerivativeThumbnail320, DerivativeThumbnail640, DerivativePreview1280}},
		{mediaType: MediaTypeVideo, want: []DerivativeKind{DerivativePoster, DerivativeHoverPreview, DerivativeProxy}},
		{mediaType: MediaTypeAudio, want: []DerivativeKind{DerivativeWaveform, DerivativeProxy}},
	}
	for _, tt := range tests {
		t.Run(string(tt.mediaType), func(t *testing.T) {
			got := BuildDerivativePlan(tt.mediaType)
			if len(got) != len(tt.want) {
				t.Fatalf("BuildDerivativePlan() = %#v, want %#v", got, tt.want)
			}
			for index := range tt.want {
				if got[index].Kind != tt.want[index] || got[index].TransformVersion != 1 {
					t.Fatalf("derivative[%d] = %#v, want %q version 1", index, got[index], tt.want[index])
				}
			}
		})
	}
}

func TestControlledObjectKeyRejectsTraversalAndUnknownPrefixes(t *testing.T) {
	for _, key := range []string{
		"media/original/user/task/file.mp4",
		"media/derivatives/user/asset/proxy.mp4",
		"media/uploads/user/session/part-1",
		"canvas/previews/user/canvas.webp",
	} {
		if !IsControlledObjectKey(key) {
			t.Errorf("controlled key %q rejected", key)
		}
	}
	for _, key := range []string{"other/file.mp4", "media/original/../secret", "/media/original/file", "media/original/"} {
		if IsControlledObjectKey(key) {
			t.Errorf("unsafe key %q accepted", key)
		}
	}
}
