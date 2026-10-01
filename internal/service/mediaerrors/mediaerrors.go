// Package mediaerrors maps upstream provider and platform error codes to the
// platform-unified generation error codes and user-facing copy. Raw provider
// messages and vendor codes never reach end users: known codes resolve to a
// curated Resolution, unknown non-CJK messages fall back to themed or generic
// results that point at the task id. Raw values stay in the database for
// admin diagnostics. The taxonomy is documented in
// docs/tech/2026-09-29-unified-media-generation-error-codes.md.
package mediaerrors

import (
	"strings"
	"unicode"
)

// Platform-unified generation error codes. Stable contract: vendor codes map
// onto these; they are the only codes user-facing APIs expose for generation
// failures and the foundation for the future open API.
const (
	CodeContentBlockedInput      = "GENERATION_CONTENT_BLOCKED_INPUT"
	CodeContentBlockedOutput     = "GENERATION_CONTENT_BLOCKED_OUTPUT"
	CodeProviderBusy             = "GENERATION_PROVIDER_BUSY"
	CodeProviderTimeout          = "GENERATION_PROVIDER_TIMEOUT"
	CodeProviderUnavailable      = "GENERATION_PROVIDER_UNAVAILABLE"
	CodeProviderCapacityExceeded = "GENERATION_PROVIDER_CAPACITY_EXCEEDED"
	CodeProviderAuthFailed       = "GENERATION_PROVIDER_AUTH_FAILED"
	CodeGenerationRejected       = "GENERATION_REQUEST_REJECTED"
	CodeModelUnavailable         = "GENERATION_MODEL_UNAVAILABLE"
	CodeArtifactFailed           = "GENERATION_ARTIFACT_FAILED"
	CodeInputAssetUnavailable    = "GENERATION_INPUT_ASSET_UNAVAILABLE"
)

// Resolution is the platform-unified result for one upstream failure.
type Resolution struct {
	Code    string
	Message string
}

const (
	genericCopy      = "生成未能完成，请稍后重试；若多次失败，请联系客服并提供任务 ID"
	moderationCopy   = "生成内容未通过安全审核，请调整提示词或素材后重试（避免知名 IP、真实人物等元素）"
	inputModeration  = "提示词或参考素材未通过安全审核，请调整内容后重试"
	busyCopy         = "上游服务繁忙，请稍后重试"
	upstreamCopy     = "上游服务异常，请稍后重试"
	timeoutCopy      = "上游处理超时，请稍后重试"
	capacityCopy     = "上游服务容量不足，请稍后重试"
	configCopy       = "服务暂时不可用，请联系客服"
	modelCopy        = "所选模型暂时不可用，请稍后重试或更换模型"
	paramCopy        = "生成参数未通过上游校验，请调整参数后重试"
	notFoundCopy     = "请求的资源不存在或已被删除，请刷新后重试"
	insufficientCopy = "积分余额不足，请充值后重试"
	mismatchCopy     = "当前模型不支持所选的参数组合，请调整后重试"
	rejectedCopy     = "生成任务被上游拒绝，请调整参数或内容后重试"
	inputAssetCopy   = "输入素材暂时无法读取，请重新选择素材后重试"
	artifactCopy     = "作品保存失败，系统会自动重试"
)

var (
	genericResolution      = Resolution{Code: CodeProviderUnavailable, Message: genericCopy}
	moderationResolution   = Resolution{Code: CodeContentBlockedOutput, Message: moderationCopy}
	inputModerationRes     = Resolution{Code: CodeContentBlockedInput, Message: inputModeration}
	busyResolution         = Resolution{Code: CodeProviderBusy, Message: busyCopy}
	upstreamResolution     = Resolution{Code: CodeProviderUnavailable, Message: upstreamCopy}
	timeoutResolution      = Resolution{Code: CodeProviderTimeout, Message: timeoutCopy}
	capacityResolution     = Resolution{Code: CodeProviderCapacityExceeded, Message: capacityCopy}
	configResolution       = Resolution{Code: CodeProviderAuthFailed, Message: configCopy}
	modelResolution        = Resolution{Code: CodeModelUnavailable, Message: modelCopy}
	paramResolution        = Resolution{Code: CodeGenerationRejected, Message: paramCopy}
	notFoundResolution     = Resolution{Code: CodeGenerationRejected, Message: notFoundCopy}
	insufficientResolution = Resolution{Code: "BILLING_INSUFFICIENT_POINTS", Message: insufficientCopy}
	mismatchResolution     = Resolution{Code: CodeGenerationRejected, Message: mismatchCopy}
	rejectedResolution     = Resolution{Code: CodeGenerationRejected, Message: rejectedCopy}
	inputAssetResolution   = Resolution{Code: CodeInputAssetUnavailable, Message: inputAssetCopy}
	artifactResolution     = Resolution{Code: CodeArtifactFailed, Message: artifactCopy}
)

// videoCodes maps video task item error codes (MiniMax numeric codes, Ark
// task error strings, worker synthetic codes) to unified results. Dotted
// vendor variants (e.g. InvalidParameter.UnsupportedImageFormat) match by
// progressive prefix.
var videoCodes = map[string]Resolution{
	// MiniMax task error codes (platform.minimaxi.com/docs/api-reference/errorcode).
	"1000": upstreamResolution,
	"1001": timeoutResolution,
	"1002": busyResolution,
	"1004": configResolution,
	"1008": capacityResolution,
	"1024": upstreamResolution,
	"1026": inputModerationRes,
	"1027": moderationResolution,
	"1033": upstreamResolution,
	"1039": capacityResolution,
	"1041": busyResolution,
	"2013": paramResolution,
	"2045": busyResolution,
	"2049": configResolution,
	"2056": capacityResolution,
	// Ark task and API error codes (ark.volcengine.com/docs/ark/error-codes).
	"datainspectionfailed":                moderationResolution,
	"inputtextsensitivecontentdetected":   inputModerationRes,
	"outputtextsensitivecontentdetected":  moderationResolution,
	"outputvideosensitivecontentdetected": moderationResolution,
	"outputaudiosensitivecontentdetected": moderationResolution,
	"sensitivecontentdetected":            moderationResolution,
	"internalserviceerror":                upstreamResolution,
	"modeltimeout":                        timeoutResolution,
	"invalidparameter":                    paramResolution,
	"invalidendpointormodel.notfound":     modelResolution,
	"invalidendpointormodel":              modelResolution,
	"modelnotopen":                        modelResolution,
	"modelnotfound":                       modelResolution,
	"quotaexceeded":                       capacityResolution,
	"modelquotaexceeded":                  capacityResolution,
	"accountarrears":                      capacityResolution,
	"flowlimitexceeded":                   busyResolution,
	"accountillegallyblocked":             configResolution,
	"accessdenied":                        configResolution,
	// Worker synthetic codes.
	"provider_submit_failed":     rejectedResolution,
	"provider_unavailable":       upstreamResolution,
	"provider_input_unavailable": inputAssetResolution,
	"artifact_persist_failed":    artifactResolution,
	"provider_artifact_missing":  artifactResolution,
	"usage_normalization_failed": artifactResolution,
}

// imageCodes maps image task error codes (openai_compatible provider codes
// and platform errs codes) to unified results.
var imageCodes = map[string]Resolution{
	// OpenAI-compatible provider codes.
	"content_policy_violation":            moderationResolution,
	"moderation_blocked":                  moderationResolution,
	"content_filter":                      moderationResolution,
	"safety_system":                       moderationResolution,
	"datainspectionfailed":                moderationResolution,
	"inputtextsensitivecontentdetected":   inputModerationRes,
	"outputimagesensitivecontentdetected": moderationResolution,
	"sensitivecontentdetected":            moderationResolution,
	"rate_limit_exceeded":                 busyResolution,
	"rate_limit_error":                    busyResolution,
	"requests_too_high":                   busyResolution,
	"insufficient_quota":                  capacityResolution,
	"billing_hard_limit_reached":          capacityResolution,
	"quota_exceeded":                      capacityResolution,
	"account_deactivated":                 configResolution,
	"invalid_api_key":                     configResolution,
	"authentication_error":                configResolution,
	"unauthorized":                        configResolution,
	"accessdenied":                        configResolution,
	"model_not_found":                     modelResolution,
	"invalid_model_error":                 modelResolution,
	"model_not_open":                      modelResolution,
	"timeout":                             timeoutResolution,
	"request_timeout":                     timeoutResolution,
	"apitimeouterror":                     timeoutResolution,
	"server_error":                        upstreamResolution,
	"api_error":                           upstreamResolution,
	"internal_error":                      upstreamResolution,
	"overloaded_error":                    busyResolution,
	"invalid_request_error":               paramResolution,
	// Platform errs codes surfaced on tasks (already unified; identity codes).
	"image_capability_mismatch":   {Code: "IMAGE_CAPABILITY_MISMATCH", Message: mismatchCopy},
	"image_storage_failed":        {Code: "IMAGE_STORAGE_FAILED", Message: "作品保存失败，系统会自动重试"},
	"video_capability_mismatch":   {Code: "VIDEO_CAPABILITY_MISMATCH", Message: mismatchCopy},
	"not_found":                   {Code: "NOT_FOUND", Message: notFoundCopy},
	"billing_insufficient_points": insufficientResolution,
	"provider_unavailable":        upstreamResolution,
}

// messageRules classify raw messages into unified results.
var messageRules = []struct {
	match      []string
	resolution Resolution
}{
	{[]string{"sensitive", "敏感", "审核", "content_policy", "moderation", "contentfilter", "content_filter", "copyright", "版权"}, moderationResolution},
	{[]string{"rate limit", "ratelimit", "throttl", "429", "限流", "并发"}, busyResolution},
	{[]string{"quota", "balance", "arrears", "billing", "余额", "额度", "欠费"}, capacityResolution},
	{[]string{"timeout", "deadline", "超时"}, timeoutResolution},
	{[]string{"unauthorized", "invalid api key", "api key", "token", "鉴权"}, configResolution},
	{[]string{"not found", "no such", "不存在"}, notFoundResolution},
	{[]string{"internal", "server error", "500", "502", "503", "内部错误"}, upstreamResolution},
	{[]string{"invalid parameter", "the parameter", "parameter ", "参数错误", "unsupported", "not supported", "not valid", "mismatch"}, paramResolution},
}

// leakMarkers flag messages that embed infrastructure details (presigned
// URLs with signatures, provider client prefixes, request identifiers) and
// must never reach end users regardless of any other rule.
var leakMarkers = []string{"http://", "https://", "signature", "accesskey", "x-request-id", "request id: ", "requestid"}

// moderationCodeMarkers promote dot-suffixed or undocumented vendor code
// variants into the content-blocked family.
var moderationCodeMarkers = []string{"sensitive", "moderation", "contentfilter", "content_filter", "policyviolation", "policy_violation"}

// Resolution types.

// ResolveVideoItem resolves a video task item error for user-facing APIs.
func ResolveVideoItem(code, message string) Resolution {
	return resolve(code, message, videoCodes)
}

// ResolveImageTask resolves an image task error for user-facing APIs.
func ResolveImageTask(code, message string) Resolution {
	return resolve(code, message, imageCodes)
}

// Sanitize rewrites a code-less message (task progress lines) with the same
// safety rules and returns the user-facing copy.
func Sanitize(message string) string {
	return messageCopy(message, moderationResolution).Message
}

func resolve(code, message string, table map[string]Resolution) Resolution {
	trimmedCode := strings.TrimSpace(code)
	trimmedMessage := strings.TrimSpace(message)
	if resolution, ok := lookupCode(trimmedCode, table); ok {
		return refineModerationSide(resolution, trimmedCode, trimmedMessage)
	}
	return messageCopy(trimmedMessage, moderationResolution)
}

// lookupCode tries the exact table, then moderation marker substrings, then
// progressive dot-prefix matching for dotted vendor variants.
func lookupCode(code string, table map[string]Resolution) (Resolution, bool) {
	lower := strings.ToLower(code)
	if lower == "" {
		return Resolution{}, false
	}
	if resolution, ok := table[lower]; ok {
		return resolution, true
	}
	for _, marker := range moderationCodeMarkers {
		if strings.Contains(lower, marker) {
			return moderationResolution, true
		}
	}
	for {
		dot := strings.LastIndex(lower, ".")
		if dot <= 0 {
			return Resolution{}, false
		}
		lower = lower[:dot]
		if resolution, ok := table[lower]; ok {
			return resolution, true
		}
	}
}

// refineModerationSide distinguishes input-side from output-side moderation
// when the vendor code or message makes the side explicit.
func refineModerationSide(resolution Resolution, code, message string) Resolution {
	if resolution.Code != CodeContentBlockedOutput {
		return resolution
	}
	lower := strings.ToLower(code + " " + message)
	for _, marker := range []string{"input", "输入", "prompt", "提示词", "reference", "素材"} {
		if strings.Contains(lower, marker) {
			return inputModerationRes
		}
	}
	return resolution
}

// messageCopy inspects a raw message: messages authored by this platform
// (han-dominant) pass through with an empty Code (callers keep the original
// code); leak-marked or raw upstream text falls back to themed or generic copy.
func messageCopy(message string, _ Resolution) Resolution {
	message = strings.TrimSpace(message)
	if message == "" {
		return genericResolution
	}
	for _, marker := range leakMarkers {
		if strings.Contains(strings.ToLower(message), marker) {
			return genericResolution
		}
	}
	lower := strings.ToLower(message)
	for _, rule := range messageRules {
		for _, needle := range rule.match {
			if strings.Contains(lower, needle) {
				return refineModerationSide(rule.resolution, message, message)
			}
		}
	}
	if hanDominant(message) {
		return Resolution{Message: message}
	}
	return genericResolution
}

// hanDominant reports whether the message is primarily platform-authored
// Chinese. Mixed-language strings that merely prefix upstream English text
// (e.g. "上游批次失败: The parameter ...") stay suppressed.
func hanDominant(value string) bool {
	han, letters := 0, 0
	for _, r := range value {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.Is(unicode.Han, r) {
			han++
		}
	}
	return letters > 0 && han*4 >= letters
}
