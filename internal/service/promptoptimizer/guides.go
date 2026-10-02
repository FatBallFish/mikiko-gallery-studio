package promptoptimizer

import "strings"

// VideoGuideKind identifies the official prompt guide a video model follows.
// Each guide below is distilled from the vendor's own optimization playbook:
// Doubao Seedance 2.5 / Seedance 2.0 (volcengine Ark docs), Wan 3.0 (Alibaba
// Bailian docs) and MiniMax H3 (official expansion guide).
type VideoGuideKind string

const (
	VideoGuideUniversal  VideoGuideKind = "universal"
	VideoGuideSeedance25 VideoGuideKind = "seedance_2_5"
	VideoGuideSeedance20 VideoGuideKind = "seedance_2_0"
	VideoGuideWan30      VideoGuideKind = "wan_3_0"
	VideoGuideMiniMaxH3  VideoGuideKind = "minimax_h3"
)

// VideoGuideForRouteModel maps a platform route model code (route_models.code,
// admin-configured) to the guide its upstream model follows. Matching is
// defensive substring matching on the normalized code so display-name-ish
// values still resolve; anything unrecognized — including empty — degrades to
// the universal platform guide. The raw code is never interpolated anywhere.
func VideoGuideForRouteModel(routeModelCode string) VideoGuideKind {
	code := strings.ToLower(strings.TrimSpace(routeModelCode))
	switch {
	case code == "":
		return VideoGuideUniversal
	case strings.Contains(code, "minimax"), strings.Contains(code, "hailuo"), strings.Contains(code, "h3"):
		return VideoGuideMiniMaxH3
	case strings.Contains(code, "wan"):
		return VideoGuideWan30
	case strings.Contains(code, "seedance"), strings.Contains(code, "seed"):
		if strings.Contains(code, "2.5") || strings.Contains(code, "2-5") {
			return VideoGuideSeedance25
		}
		if strings.Contains(code, "2.0") || strings.Contains(code, "2-0") {
			return VideoGuideSeedance20
		}
		return VideoGuideUniversal
	default:
		return VideoGuideUniversal
	}
}

// SystemPromptForVideo returns the system prompt for a video guide kind.
func SystemPromptForVideo(guide VideoGuideKind) string {
	if prompt, ok := videoGuidePrompts[guide]; ok {
		return prompt
	}
	return videoGuidePrompts[VideoGuideUniversal]
}

var videoGuidePrompts = map[VideoGuideKind]string{
	VideoGuideUniversal:  videoGuidePrompt(universalVideoSystemPrompt),
	VideoGuideSeedance25: videoGuidePrompt(seedance25SystemPrompt),
	VideoGuideSeedance20: videoGuidePrompt(seedance20SystemPrompt),
	VideoGuideWan30:      videoGuidePrompt(wan30SystemPrompt),
	VideoGuideMiniMaxH3:  videoGuidePrompt(minimaxH3SystemPrompt),
}

// promptLengthBudgetRule enforces the upstream video models' hard prompt cap
// (prompt_max_runes=2000 on every currently registered model). Without it the
// structured guide formats routinely overflow and the estimate API then
// rejects the task with prompt too_long.
const promptLengthBudgetRule = "Hard length budget: the final rewritten prompt must fit within 2000 characters total — every CJK character, letter, digit, punctuation mark and space counts as one. When the user's draft is already long, compress deliberately instead of expanding: first keep every protected asset reference, the structural skeleton, all key actions, camera moves, dialogue and negative constraints, then cut decorative adjectives and repeated information until the text is comfortably below the cap (aim for about 1800 characters). "

// videoGuidePrompt appends the shared guards (length budget + protected
// placeholders) to every guide body so no guide can drop them.
func videoGuidePrompt(body string) string {
	return body + promptLengthBudgetRule + protectedPlaceholderRule
}

// universalVideoSystemPrompt is the platform fallback distilled from the
// common ground of all four vendor guides.
const universalVideoSystemPrompt = "Rewrite the user's video-generation prompt as a director-style structured shooting brief in the user's language. " +
	"Structure: (1) one-line overview: subject + location + event + genre/style + signature camera move; " +
	"(2) asset citations: introduce each protected asset marker with its purpose (character appearance / scene / motion or camera style to imitate / art style / voice), and cite the same marker again wherever its content is reused; " +
	"(3) a shot-by-shot timeline labeled 镜头 1 / 镜头 2 (optionally continuous integer-second ranges like 0-3s / 3-8s, each 2-5 seconds, never overlapping or gapped), each shot describing framing, exactly one camera move using standard terms (推/拉/摇/移/跟/环绕/固定/特写/全景), concrete body-level action with speed and amplitude, emotion externalized as physical details, and diegetic sound; " +
	"(4) spoken lines written as 角色台词(情绪):\"内容\" with the speaker named; ambient sound follows the visuals; use （） for music, <> for sound effects, {} for spoken lines, and 【】 for on-screen text; " +
	"(5) a closing pass with consistent camera position, depth of field, ambient sound, atmosphere, art style + color palette + mood, quality words, and negative constraints such as 不要字幕、不要水印、不要Logo. " +
	"Prefer slow, continuous, connected movements over abrupt high-energy bursts, avoid repeating the same action, keep every subject's appearance consistent throughout (avoid identical twin characters), and keep total duration and intent unchanged. "

// seedance25SystemPrompt follows the official Seedance 2.5 prompt guide:
// asset citations first, one-sentence overview, integer-second timeline,
// moderate per-segment load, one camera move per shot, and the guide's
// badcase guardrails.
const seedance25SystemPrompt = "Rewrite the user's video-generation prompt as a Seedance 2.5 director-style shooting brief in the user's language, following the official Seedance 2.5 prompt guide. " +
	"(1) Asset citations come first: give each protected asset marker exactly one clean sentence stating its role (character appearance / scene / the action or camera move to imitate / art style / voice); when only part of an asset should be referenced, say precisely which part (e.g. clothing only, not the face); when an asset is already precise, cite it without re-describing it. " +
	"(2) A one-sentence overview: subject + location + event + genre/style + signature camera move. " +
	"(3) The concrete plot split by 镜头 1 / 镜头 2 … or timeline segments with integer-second ranges (0-3s / 3-8s) that never overlap or gap; keep each segment's information load moderate — over-stuffed segments cause chaotic edits; use positive descriptions only, because reverse wishes are limited to subtitles and audio (不要字幕 / 无背景音乐); close with a pass covering the throughout-the-film camera position, environment, sound and atmosphere. " +
	"(4) Camera language: use common cinematography terms directly (特写/推近/拉远/环绕); follow a rare term with a plain-language explanation; state the transition point and method whenever scenes switch; exactly one camera move per shot. " +
	"(5) Actions: summarize first, then add key body-level detail; externalize emotion as facial and physical detail; avoid idioms and abstract adjectives. " +
	"Seedance-specific guardrails: no more than four referenced people; when emotion runs high add 正常人的眼睛,不发光; prefer delivering on-screen text as a referenced asset, otherwise spell out letters one by one; format spoken lines as 角色台词(情绪):内容 and never repeat the line afterwards; if a reference video carries subtitles add 不要字幕; when audio constraints matter state them once at the beginning and once at the end with exhaustive synonyms; treat first/last-frame, video-edit and extension tasks as locked: keep the input assets' parameters locked and describe the change as A 改成 B with scope and timestamps; for white-model inputs keep 除材质与颜色外保持白模形状; for multi-keyframe inputs write 以图片顺序作为关键帧. When a protected asset marker fills a slot in these templates, keep the marker verbatim in that slot. " +
	"Preserve the user's intent, total duration and language, and end with negative constraints such as 不要字幕、不要水印、不要Logo. "

// seedance20SystemPrompt follows the official Seedance 2.0 prompt guide:
// spatial layer + temporal layer, official reference/edit/extension sentence
// templates, stable-feature subject definitions, shot-ordered storyboard
// (no precise timestamps on 2.0), and mandatory closing constraints.
const seedance20SystemPrompt = "Rewrite the user's video-generation prompt as a Seedance 2.0 full-modal director brief in the user's language, following the official Seedance 2.0 prompt guide with a spatial layer (what each frame shows) and a temporal layer (how the story moves). " +
	"(1) References follow the official sentence templates: 参考<图片N>中的<主体N>… for characters and scenes, 参考<视频N>中的<动作/运镜/风格/音效> for motion transfer, 参考<音频N>中的音色,为<主体>配音 for voice; for video edits write 严格编辑<视频N>,将<原特征>改为<新特征> — never open an edit task with 参考视频, and state that anything unmentioned stays unchanged; for extension write 将<视频N>向前/向后延长N秒 plus how the picture, sound-effect and music tracks continue. " +
	"(2) Define subjects with stable features: 将图片N中的[2-3个稳定静态特征]定义为主体N/角色名, then reuse that exact label every time; people need a face close-up plus a full-body shot, never a multi-view sheet. " +
	"(3) Storyboard: 镜头 1 / 镜头 2 … in event order; Seedance 2.0 does not respond to precise timestamps, so never write 0-3s style ranges; inside each shot order as: camera move and cut type → subject action and expression → position and spatial change → audio information; exactly one camera move per shot. " +
	"(4) Actions: refine to body parts with quantified degree (缓慢抬手/快速转头), prefer slow continuous small movements, write how each movement transitions into the next, and externalize emotion into physical detail. " +
	"(5) Close with visual style, quality words and mandatory constraints (保持无字幕 / 不要生成Logo / 不要生成水印); keep spoken-line language consistent; use （） for music, <> for sound effects, {} for spoken lines, 【】 for on-screen text. " +
	"Reference setup guidance: at most 4-5 assets (1-2 characters + 1 scene + 1 camera-move video + 1 audio), at most four people, and end with the anti-twin constraint 避免出现长相相同的人物. When a protected asset marker fills a slot in these templates, keep the marker verbatim in that slot. " +
	"Preserve the user's intent, total duration and language. "

// wan30SystemPrompt follows the official Wan 3.0 (万相3.0) full prompt
// formula: overview + numbered asset citations + timestamped storyboard +
// dialogue + sound/BGM + style/mood + negative list.
const wan30SystemPrompt = "Rewrite the user's video-generation prompt following the official Wan 3.0 (万相3.0) full prompt formula, in the user's language: " +
	"[总体描述 one-line overview of subject, scene, event, style and mood] + [参考素材引用 citing assets by 图N/视频N/音频N numbers, each citation stating its purpose] + [分镜N（起-止秒) storyboard entries of 2-5 seconds each, timestamps contiguous with no gaps or overlaps, every entry covering 主体+场景+运动+美学控制] + [台词: xx说:\"xxxx\"] + [音效/BGM] + [风格/情绪: 画风+色调+情绪] + [负向清单]. " +
	"Citation rules: image, video and audio assets are numbered separately in upload order (图1、图2、视频1、音频1); the same asset may be cited at multiple spots — cite it once per distinct purpose; a single image asset may be cited as 参考图片. When a protected asset marker fills a citation slot, keep the marker verbatim there. " +
	"Storyboard rules: write 一镜到底 for single-shot videos; write 固定镜头,摄影机静止 when the camera must not move; use plain camera verbs (推近/拉远/环绕/手持跟拍); name transitions as 硬切转场/叠化转场; put pacing into the 总体描述. " +
	"Dialogue rules: speaker + speaking verb + colon + quoted line, one speaker per line; for cloned timbre write 音色参考音频N once; for lip sync add Lip sync; when no dialogue is wanted write 无台词. " +
	"Sound rules: describe actions with materials and weather so ambient sound follows automatically; put specific sound effects in their own sentence, optionally with a timestamp; choose one of three BGM modes — omit it for automatic music, name the style explicitly, or write 无bgm,只生成环境音和动作音. " +
	"The 负向清单 lists only unwanted elements and never repeats positive content (e.g. 无字幕、无水印). " +
	"Wan-specific locked tasks: white-model inputs keep 除材质与颜色外一律不改 with frame-locked shapes; storyboard images are declared 仅作分镜构图指导; video edits apply one verb per pass (增加/删除/替换/改成/重塑) plus 其他保持不变; video extension writes 延长/续写 with direction, the new content, and a re-statement of character features to prevent warping. " +
	"Preserve the user's intent, total duration and language. "

// minimaxH3SystemPrompt follows the official MiniMax H3 expansion guide:
// an alignment instruction plus integrated_multimodal_description /
// overall_soundscape / non_diegetic_music, with per-task anchor structures
// (I2VA / FL2VA / L2VA) and labeled multi-reference definitions.
const minimaxH3SystemPrompt = "Rewrite the user's video-generation prompt as a MiniMax H3 structured expansion in the user's language, with four parts: " +
	"(1) an alignment instruction stating which protected asset marker is the true first frame (or last frame) and that the video's opening frame must match it exactly; " +
	"(2) integrated_multimodal_description — the main body organized as [Shot 1] [Shot 2] … timeline entries covering visuals, body-level action, camera movement (move type + amplitude + speed), speaker-attributed spoken lines or singing, and in-scene sounds; " +
	"(3) overall_soundscape — one short passage summarizing ambient sound, physical action sounds and non-verbal human sounds across the whole video; " +
	"(4) non_diegetic_music — the soundtrack only the audience hears (characters cannot hear it), or an explicit note that there is none. " +
	"Task-specific structure: first-frame tasks follow first-frame anchor → action onset → continuous change → result or reaction, keeping faces, clothing, colors, key objects and spatial relations consistent with that frame; first-last-frame tasks align the first marker to 0.00s and the second to the final second, then follow first state → intermediate change → converging difference → last state, preferring a single shot; last-frame tasks infer a plausible prior state → change path → convergence onto the given final frame; plain text-to-video starts directly at the description. " +
	"For multi-reference tasks, define each protected marker as <Picture N>/<Video N>/<Audio N>/<Subject N> on first mention, open with a one-line summary, add a retention line stating what must be kept from each reference, then expand the shots citing those labels. " +
	"On-screen text must state its content and position; keep spoken-line language consistent. " +
	"Preserve the user's intent, total duration and language. "
