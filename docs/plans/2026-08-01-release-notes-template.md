# Templated Release Notes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Generate complete Chinese GitHub Release Notes from a repository template for every SemVer tag release.

**Architecture:** A deterministic Bash renderer reads the current tag, finds the previous reachable SemVer tag, classifies non-merge Conventional Commit subjects, and expands standalone template placeholders. The tagged release workflow renders the Notes before release creation and uses the same file for both initial creation and idempotent updates.

**Tech Stack:** Bash, Git, GitHub Actions, GitHub CLI, Markdown, existing shell release contracts.

---

### Task 1: Add failing Release Notes contracts

**Files:**
- Create: `scripts/devops/release-notes-contract-test.sh`
- Modify: `scripts/devops/release-contract-test.sh`

**Step 1: Write the renderer behavior contract**

Create a temporary Git repository with a baseline tag followed by `feat`, `fix`, `refactor`, `docs`, and unknown-prefix commits. Invoke the wished-for `scripts/devops/render-release-notes.sh` API with `RELEASE_NOTES_GIT_ROOT`, `RELEASE_NOTES_TEMPLATE`, `RELEASE_NOTES_OUTPUT`, `RELEASE_NOTES_REPOSITORY`, and `RELEASE_NOTES_TAG` overrides.

Assert that the output contains all six Chinese sections, the concrete version, the expected commit subjects and links under the correct categories, “本版本暂无” for empty categories, the pinned installer URL, `mgsctl upgrade`, and `mgsctl doctor`. Add separate fixtures for a first release, an invalid tag, a missing template, and an unknown tag.

**Step 2: Extend the workflow release contract**

Require the template, renderer, behavior contract, full-history checkout, render step, `--notes-file`, and `gh release edit`. Forbid `--generate-notes`.

**Step 3: Run tests to verify RED**

Run:

```bash
./scripts/devops/release-notes-contract-test.sh
./scripts/devops/release-contract-test.sh
```

Expected: FAIL because the template and renderer do not exist and the workflow still uses `--generate-notes`.

**Step 4: Commit the failing contracts**

```bash
git add scripts/devops/release-notes-contract-test.sh scripts/devops/release-contract-test.sh
git commit -m "test(release): define templated notes contract"
```

### Task 2: Implement the deterministic Notes renderer

**Files:**
- Create: `.github/release-notes-template.md`
- Create: `scripts/devops/render-release-notes.sh`
- Test: `scripts/devops/release-notes-contract-test.sh`

**Step 1: Add the six-section Markdown template**

Use standalone `{{FEATURES}}`, `{{BUGFIXES}}`, and `{{OPTIMIZATIONS}}` lines plus inline `{{VERSION}}` placeholders. Keep prose Chinese and commands English. Pin the raw installer URL and `MGSCTL_VERSION` to `{{VERSION}}`.

**Step 2: Implement validation and range resolution**

Validate `vX.Y.Z` SemVer, template readability, tag resolvability, repository name, and output parent creation. Resolve the previous reachable `v*` tag; for a first release include history from the root commit.

**Step 3: Implement classification and rendering**

Read non-merge commits oldest-first. Classify `feat` and `fix` separately, send the approved optimization prefixes and unknown subjects to optimization, and emit each subject with a full GitHub commit link. Render placeholder lines with `awk`, replace the version, reject leftover placeholders, and atomically move the completed file into place.

**Step 4: Run the behavior contract to verify GREEN**

Run:

```bash
./scripts/devops/release-notes-contract-test.sh
```

Expected: `OK: templated release notes contract verified`.

**Step 5: Commit**

```bash
git add .github/release-notes-template.md scripts/devops/render-release-notes.sh
git commit -m "feat(release): render structured Chinese notes"
```

### Task 3: Integrate rendered Notes into tagged releases

**Files:**
- Modify: `.github/workflows/release.yml`
- Modify: `scripts/devops/release-contract-test.sh`

**Step 1: Add full-history checkout and rendering**

In the `release` job, checkout the tag with `fetch-depth: 0` before downloading artifacts. Render to `target/release-notes.md` with `RELEASE_NOTES_TAG=${{ github.ref_name }}` and `RELEASE_NOTES_REPOSITORY=${{ github.repository }}`.

**Step 2: Use the same Notes file for create and update**

Create missing Releases with `gh release create --verify-tag --notes-file target/release-notes.md`. For an existing Release, run `gh release edit --notes-file target/release-notes.md` before asset synchronization. Preserve the current byte-for-byte asset checks and `promote-latest` dependencies.

**Step 3: Run release contracts to verify GREEN**

Run:

```bash
./scripts/devops/release-notes-contract-test.sh
./scripts/devops/release-contract-test.sh
bash -n scripts/devops/render-release-notes.sh scripts/devops/release-notes-contract-test.sh scripts/devops/release-contract-test.sh
```

Expected: all contracts pass and no `--generate-notes` remains.

**Step 4: Commit**

```bash
git add .github/workflows/release.yml scripts/devops/release-contract-test.sh
git commit -m "ci: publish templated release notes"
```

### Task 4: Preview the next release and run repository verification

**Files:**
- Generated only: `target/release-notes-v0.0.4.md`

**Step 1: Render a local v0.0.4 preview without creating a tag**

Use a temporary local tag or the renderer overrides so the preview covers changes since `v0.0.3`. Confirm all placeholders are expanded and the six sections read correctly.

**Step 2: Run complete verification**

Use `dev-verify`:

```bash
./scripts/workflow/verify.sh
```

Expected: `OK: verification passed`.

**Step 3: Inspect the committed diff**

Confirm generated preview files are not staged and only the approved template, renderer, contracts, workflow, requirements, design, and plan are committed.

### Task 5: Run delivery gate and open the PR

**Files:**
- Generated: `.review/gate.json`

**Step 1: Run the final delivery workflow**

Use `dev-ship`:

```bash
./scripts/workflow/ship-guard.sh
```

Expected: verification, committed review gate, and applicable smoke checks pass.

**Step 2: Push and open a PR to main**

```bash
git push --set-upstream origin codex/release-notes-template
gh pr create --base main --head codex/release-notes-template
```

**Step 3: After merge, publish v0.0.4**

Create annotated tag `v0.0.4` on the exact merge commit, push it, monitor Tagged Release to success, and verify the Release body, 44 assets, Manifest checksum, five multi-architecture version images, and five matching `latest` digests.
