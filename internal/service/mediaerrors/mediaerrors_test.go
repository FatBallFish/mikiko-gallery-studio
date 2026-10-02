package mediaerrors

import "testing"

func TestResolveVideoItemMapsKnownCodesToUnifiedResults(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		message     string
		wantCode    string
		wantMessage string
	}{
		{name: "minimax output moderation", code: "1027", message: "output new_sensitive", wantCode: CodeContentBlockedOutput, wantMessage: moderationCopy},
		{name: "minimax input moderation", code: "1026", message: "input new_sensitive", wantCode: CodeContentBlockedInput, wantMessage: inputModeration},
		{name: "minimax rate limit", code: "1002", message: "too many requests", wantCode: CodeProviderBusy, wantMessage: busyCopy},
		{name: "minimax balance", code: "1008", message: "balance insufficient", wantCode: CodeProviderCapacityExceeded, wantMessage: capacityCopy},
		{name: "minimax param error", code: "2013", message: "invalid parameter", wantCode: CodeGenerationRejected, wantMessage: paramCopy},
		{name: "ark data inspection", code: "DataInspectionFailed", message: "content inspection failed", wantCode: CodeContentBlockedOutput, wantMessage: moderationCopy},
		{name: "ark dotted param variant", code: "InvalidParameter.UnsupportedImageFormat", message: "image format is not supported", wantCode: CodeGenerationRejected, wantMessage: paramCopy},
		{name: "ark dotted model variant", code: "InvalidEndpointOrModel.NotFound", message: "model not found", wantCode: CodeModelUnavailable, wantMessage: modelCopy},
		{name: "ark input sensitive", code: "InputTextSensitiveContentDetected", message: "input text may contain sensitive info", wantCode: CodeContentBlockedInput, wantMessage: inputModeration},
		{name: "ark output video sensitive suffix", code: "OutputVideoSensitiveContentDetected.PolicyViolation", message: "the output video may be related to copyright restricted content", wantCode: CodeContentBlockedOutput, wantMessage: moderationCopy},
		{name: "worker submit failure", code: "provider_submit_failed", message: `seedance video provider: The parameter ` + "`content[1]`" + ` is not valid`, wantCode: CodeGenerationRejected, wantMessage: rejectedCopy},
		{name: "worker artifact failure", code: "artifact_persist_failed", message: `download artifact: Get "https://oss.example.com/a.mp4?Signature=abc"`, wantCode: CodeArtifactFailed, wantMessage: artifactCopy},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ResolveVideoItem(test.code, test.message)
			if got.Code != test.wantCode || got.Message != test.wantMessage {
				t.Fatalf("ResolveVideoItem(%q, %q) = %+v, want code=%q message=%q", test.code, test.message, got, test.wantCode, test.wantMessage)
			}
		})
	}
}

func TestResolveVideoItemFallsBackForUnknownCodes(t *testing.T) {
	got := ResolveVideoItem("9999", "totally unknown upstream text")
	if got.Code != CodeProviderUnavailable || got.Message != genericCopy {
		t.Fatalf("unknown non-CJK message should fall back to generic result, got %+v", got)
	}
	kept := ResolveVideoItem("", "第二张生成失败")
	if kept.Code != "" || kept.Message != "第二张生成失败" {
		t.Fatalf("platform-authored Chinese copy must keep the original code and message, got %+v", kept)
	}
	kept = ResolveVideoItem("9999", "部分上游批次未返回有效图片")
	if kept.Code != "" || kept.Message != "部分上游批次未返回有效图片" {
		t.Fatalf("Chinese platform message should pass through, got %+v", kept)
	}
}

// Healthy runs and attempts carry no error fields at all; resolving them must
// stay empty instead of decorating them with the generic failure resolution.
func TestResolveEmptyErrorFieldsStaysEmpty(t *testing.T) {
	for name, resolver := range map[string]func(string, string) Resolution{
		"video": ResolveVideoItem,
		"image": ResolveImageTask,
	} {
		if got := resolver("", ""); got.Code != "" || got.Message != "" {
			t.Fatalf("%s: empty error fields must resolve to no error, got %+v", name, got)
		}
		if got := resolver("  ", "  "); got.Code != "" || got.Message != "" {
			t.Fatalf("%s: whitespace-only error fields must resolve to no error, got %+v", name, got)
		}
	}
}

func TestMessageCopyRedactsLeakMarkers(t *testing.T) {
	if got := Sanitize("生成失败: https://oss.example.com/a.mp4?Expires=1&Signature=secret 已过期"); got != genericCopy {
		t.Fatalf("signed URL must never reach users, got %q", got)
	}
	if got := Sanitize("上游批次失败: The parameter ratio specified in the request is not valid"); got != paramCopy {
		t.Fatalf("mixed-language leak should map to themed copy, got %q", got)
	}
}

func TestResolveImageTaskMapsKnownCodesToUnifiedResults(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		message     string
		wantCode    string
		wantMessage string
	}{
		{name: "content policy", code: "content_policy_violation", message: "your request was rejected", wantCode: CodeContentBlockedOutput, wantMessage: moderationCopy},
		{name: "rate limit", code: "rate_limit_error", message: "too many requests", wantCode: CodeProviderBusy, wantMessage: busyCopy},
		{name: "quota", code: "insufficient_quota", message: "quota exceeded", wantCode: CodeProviderCapacityExceeded, wantMessage: capacityCopy},
		{name: "capability mismatch keeps platform code", code: "IMAGE_CAPABILITY_MISMATCH", message: "quality is unsupported", wantCode: "IMAGE_CAPABILITY_MISMATCH", wantMessage: mismatchCopy},
		{name: "missing reference asset", code: "NOT_FOUND", message: "reference asset not found", wantCode: "NOT_FOUND", wantMessage: notFoundCopy},
		{name: "insufficient points", code: "BILLING_INSUFFICIENT_POINTS", message: "insufficient points", wantCode: "BILLING_INSUFFICIENT_POINTS", wantMessage: insufficientCopy},
		{name: "openai invalid request", code: "invalid_request_error", message: "bad shape", wantCode: CodeGenerationRejected, wantMessage: paramCopy},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ResolveImageTask(test.code, test.message)
			if got.Code != test.wantCode || got.Message != test.wantMessage {
				t.Fatalf("ResolveImageTask(%q, %q) = %+v, want code=%q message=%q", test.code, test.message, got, test.wantCode, test.wantMessage)
			}
		})
	}
	if got := ResolveImageTask("weird_new_code", "some upstream detail"); got.Code != CodeProviderUnavailable || got.Message != genericCopy {
		t.Fatalf("unknown image code should fall back to generic result, got %+v", got)
	}
}

func TestResolveImageTaskKeepsPlatformStorageCode(t *testing.T) {
	got := ResolveImageTask("IMAGE_STORAGE_FAILED", "mkdir /var/folders/xx/runtime-storage: not a directory")
	if got.Code != "IMAGE_STORAGE_FAILED" {
		t.Fatalf("platform storage code must survive translation, got %q", got.Code)
	}
	if got.Message != "作品保存失败，系统会自动重试" {
		t.Fatalf("storage failure copy = %q", got.Message)
	}
}

// Empty code + empty message means "no error" (healthy runs/attempts), while
// a present code with no message is a real failure and keeps the generic copy.
func TestEmptyMessageYieldsGenericCopy(t *testing.T) {
	if got := ResolveVideoItem("", ""); got.Code != "" || got.Message != "" {
		t.Fatalf("empty code and message is a healthy task, got %+v", got)
	}
	if got := ResolveImageTask("", "  "); got.Code != "" || got.Message != "" {
		t.Fatalf("whitespace-only fields are a healthy task, got %+v", got)
	}
	if got := ResolveVideoItem("9999", ""); got.Code != CodeProviderUnavailable || got.Message != genericCopy {
		t.Fatalf("unknown code with empty message must yield generic copy, got %+v", got)
	}
}

func TestKnownCodeWinsOverMessage(t *testing.T) {
	// A known moderation code with an unrelated message still resolves via
	// the curated table, not the message heuristics.
	got := ResolveVideoItem("1027", "")
	if got.Code != CodeContentBlockedOutput || got.Message != moderationCopy {
		t.Fatalf("known code should resolve without a message, got %+v", got)
	}
}
