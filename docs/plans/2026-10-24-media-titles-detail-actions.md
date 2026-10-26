# Media titles, detail actions and video artifact policy

## Requirement source

Repository owner request, 2026-10-24: generated images use a text-model title of at most 15 characters when a default text model is configured, otherwise YYYYMMdd-xxxxx with five random alphanumeric characters. Uploaded images use filenames without extension. Names are editable and may repeat; historical names remain available. Creation and canvas references show the name. Video details show prompts. Image details expose rename, grouping, edit, first-frame generation, project transfer, configuration reuse, primary download and delete; remove duplicate prompt/download actions. Video details remove add-to-canvas and move configuration reuse into more actions. Configuration reuse restores prompt, compatible parameters and references. Video accounts expose optional domain checking, disabled by default, with common documented suggestions and custom multi-selection.

## Technical design

Reuse media_assets.name and existing versioned mutation APIs, preserve duplicate names and historical values, and display name independently of prompt. Strip only the final upload extension. Title generation must use existing default text-model resolution, bounded context and sanitized output, remain nonfatal, and never reissue image generation. Preserve artifact recovery and billing invariants. Ensure HTTP download filenames retain appropriate file extensions independently of display names.

Backend implementation uses one default-text-model title request per new image task result batch, with an eight-second child context, a 2,000-rune prompt limit and a 512-token output budget (including reasoning). Sanitize and truncate the title to 15 Unicode characters. Resolution/provider failures use YYYYMMdd plus five cryptographically random alphanumeric characters for each image; unavailable system entropy uses a nonfatal fixed suffix. The paid upstream image response is checkpointed before title enrichment; the named results are then saved into the existing encrypted recovery envelope on a best-effort basis. Artifact retries reuse checkpointed names and never invoke image generation. No title queue or schema migration is needed. A crash before the optional named checkpoint can repeat the text request during recovery.

Generated MediaAsset IDs equal ImageResult IDs. Task and gallery projections expose optional name/media_asset_id from the current canonical asset, including gallery reference names. Versioned MediaAsset rename remains authoritative, and subsequent task persistence never replaces existing asset names. ReferenceAsset.name remains its stable, unique prompt-template binding; optional display_name projects the canonical media title, which may duplicate or contain template-invalid characters. Gallery reference names are display-only. New uploads strip only the final filename extension; historical names are left untouched. Media download filenames derive the extension from the delivered object/MIME type independently of the display name.

Use shared media detail and creation navigation contracts so all entry points receive working actions. Reuse source task metadata and validate restored capabilities. Keep user/project ownership enforcement on server mutations.

Store video whitelist-enabled state in existing account extra metadata. Propagate it into production and model-test download paths. Disabled host-list checks must retain HTTPS, public-IP and redirect validation. Enabled checks require configured exact hosts. Existing accounts with explicit hosts retain checking when no explicit switch exists. Suggested domains must be evidence-based and never silently trusted; custom domains remain supported.

## Implementation and validation

1. Inspect backend naming and title integration; implement fallback, text-title enrichment and upload/display separation.
2. Implement shared image/video details, reference titles and configuration reuse.
3. Implement optional video artifact host policy and administrator controls.
4. Run focused tests, full verification, local review and isolated API smoke; inspect running UI where available.

No commit, push or PR update is authorized by this request. Existing uncommitted artifact-transfer changes are retained.
