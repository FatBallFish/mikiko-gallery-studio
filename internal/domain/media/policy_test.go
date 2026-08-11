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

	video := UploadDeclaration{Filename: "clip.mov", MediaType: MediaTypeVideo, MIMEType: "video/quicktime", SizeBytes: 4096}
	if err := policy.ValidateProbe(video, ProbeResult{MediaType: MediaTypeVideo, Format: "mov", Container: "mov", VideoCodec: "vp9"}); err == nil || err.Field != "video_codec" {
		t.Fatalf("unsupported codec error = %#v", err)
	}

	if err := policy.ValidateProbe(video, ProbeResult{MediaType: MediaTypeVideo, Format: "mov", Container: "mov", VideoCodec: "hevc", AudioCodec: "aac", DurationMS: 5000}); err != nil {
		t.Fatalf("supported MOV/HEVC probe rejected: %v", err)
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
