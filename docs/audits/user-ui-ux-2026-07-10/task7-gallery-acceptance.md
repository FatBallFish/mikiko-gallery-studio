# Task 7 Home And Gallery Acceptance

Date: 2026-07-10

## Scope

- Authenticated Home continuation, readiness, recent task, and curated inspiration.
- Private asset filtering, selection, grouping, publish, delete, download, reuse, and detail.
- Public gallery filtering, infinite loading, authenticated detail, reactions, download, and same-generation parameter import.
- Shared filter toolbar, image geometry, loading/error handling, and image detail/lightbox behavior.

## Data Provenance

- `task7-home-empty-*`, `task7-assets-empty-*`, and `task7-public-empty-*` use the local API at `http://127.0.0.1:8088`. The local database had no tasks, assets, models, or approved public work.
- Files containing `injected` and populated/error screenshots use a browser-local GET override only for task, capability, private-gallery, public-gallery list, and public-detail reads. They verify presentation and interaction states; they are not evidence of live backend records.
- `task7-home-populated-injected-1440.png` proves only that the Home composition remains bounded when injected rows are present and that unavailable images expose the image-error/retry treatment. It is not evidence of a successfully rendered populated gallery.
- A follow-up injection referenced current-project assets, but the injected gallery state did not mount in the application. No valid-image populated Home capture was produced at desktop or 390px, and this audit does not claim one.
- The files `/landing/hero-gallery.webp` (1280 x 720) and `/landing/workspace.webp` (1291 x 808) were independently validated as local WebP assets. Their file validity and dimensions do not prove that a populated Home gallery rendered them.
- The injected public list returned 24 items on page 1 and 4 on page 2. Browser text confirmed `已加载 28 张` and `已显示全部作品` after the application scroll container reached its sentinel.
- `task7-assets-pagination-selection-injected-1440.png` uses an in-page `fetch` override for three private-gallery pages. Page 2 intentionally repeats one ID and the final browser state contains 120 unique assets, proving the UI can render more than 100 items without duplicating the repeated row. This is injected presentation and interaction evidence, not evidence of 120 backend records.
- `task7-lightbox-*-injected-*` forces an already-mounted image element to a missing local URL. These captures verify the full-image and zoom fallback presentation and accessible retry control; they are not evidence of a backend image failure.

## Browser Checks

- Real email-code authentication completed against the local API and returned to Home.
- Home empty and running-task compositions were checked at 1440 and 390 widths. The available populated Home screenshot covers bounded rows plus image failure/retry behavior only; successful gallery imagery remains unverified.
- Private assets were checked empty at 1440/320 and populated at 1440, including selection, batch actions, detail, valid local images, and one failed image with retry.
- Private pagination was additionally checked at 1440 with reduced motion enabled. Explicit load-more actions produced 50, 99, then 120 unique visible assets after an intentional duplicate ID; the first selection remained `已选择 1 项`, the terminal copy read `已显示全部资产`, and the page reported no horizontal overflow (`1440/1440`).
- Public gallery was checked empty at 1440/390, populated/infinite at 1440, and detail at 1440/390 using the explicit injected reads.
- Rapid public-filter stale-response rejection is executable-contract evidence only. No browser race is claimed.
- A broken full-size lightbox at 1440 exposed `重试加载`, and retry restored the valid full-size image. At 390, the zoom viewer exposed the same accessible fallback with reduced motion enabled and no horizontal overflow (`390/390`). The forced-DOM zoom retry remained in its fallback, so successful zoom recovery is covered by the retry/remount contract rather than claimed as browser evidence.
- Preservation of a deep-linked task older than the rolling history limit is executable-contract evidence only. No browser deep-link history fixture is claimed.
- Width probes reported no horizontal overflow: `1440/1440`, `390/390`, and `320/320`.
- Dark and light themes were exercised. The new private-pagination and lightbox checks ran with `prefers-reduced-motion: reduce`; executable contracts additionally assert that both lightbox entrance animations use the reduced-motion override.
- All local image actions are semantic buttons and remain in the accessibility tree without hover. Coarse-pointer CSS makes the shared action tray visible on touch devices.

## Fixes From Visual Review

- Shortened the Home headline to avoid a one-character orphan at desktop width.
- Forced search and filters onto stable full-width mobile rows to prevent Chinese filter labels from collapsing vertically.
- Replaced broken detail images with a local unavailable state instead of exposing browser broken-image UI.

## Representative Screenshots

- `screenshots/task7-home-empty-1440.png`
- `screenshots/task7-home-empty-390.png`
- `screenshots/task7-home-populated-injected-1440.png` (bounded rows and image-error/retry evidence only)
- `screenshots/task7-assets-empty-320.png`
- `screenshots/task7-assets-populated-selected-dark-1440.png`
- `screenshots/task7-assets-detail-dark-1440.png`
- `screenshots/task7-assets-pagination-selection-injected-1440.png`
- `screenshots/task7-public-empty-1440.png`
- `screenshots/task7-public-empty-dark-390.png`
- `screenshots/task7-public-infinite-error-dark-1440.png`
- `screenshots/task7-public-detail-injected-dark-1440.png`
- `screenshots/task7-lightbox-full-error-reduced-injected-1440.png`
- `screenshots/task7-lightbox-zoom-error-reduced-injected-390.png`

## Automated Evidence

- Ten focused Task 7 and quality contracts pass: private pagination, public stale-request guards, lightbox media and focus layers, workspace task-history preservation, Home task continuation URL wiring, shared gallery experience, private rows, Home cards, and public cards.
- `npm --prefix web/user run typecheck`: pass.
- `npm --prefix web/user run build`: pass, including the landing bundle split contract.
- Vite reports the existing main-chunk size warning; it is not introduced as a functional failure by this task.
