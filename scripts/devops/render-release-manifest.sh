#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
RELEASE_VERSION=${RELEASE_VERSION:?RELEASE_VERSION is required}
RELEASE_COMMIT=${RELEASE_COMMIT:?RELEASE_COMMIT is required}
RELEASE_ASSET_DIR=${RELEASE_ASSET_DIR:-"$ROOT/target/release"}
RELEASE_IMAGE_METADATA=${RELEASE_IMAGE_METADATA:?RELEASE_IMAGE_METADATA is required}
RELEASE_MANIFEST_OUTPUT=${RELEASE_MANIFEST_OUTPUT:-"$RELEASE_ASSET_DIR/release-manifest.json"}

python3 - "$RELEASE_VERSION" "$RELEASE_COMMIT" "$RELEASE_ASSET_DIR" "$RELEASE_IMAGE_METADATA" "$RELEASE_MANIFEST_OUTPUT" <<'PY'
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import tempfile

version, commit, asset_root_raw, image_metadata_raw, output_raw = sys.argv[1:]
asset_root = Path(asset_root_raw).resolve()
image_metadata = Path(image_metadata_raw).resolve()
output = Path(output_raw).resolve()
version_pattern = re.compile(r"^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$")
digest_pattern = re.compile(r"^sha256:[0-9a-f]{64}$")
hex_pattern = re.compile(r"^[0-9a-f]{64}$")

if not version_pattern.fullmatch(version):
    raise SystemExit("RELEASE_VERSION must be a vX.Y.Z SemVer tag")
if not commit.strip():
    raise SystemExit("RELEASE_COMMIT must not be empty")
if not asset_root.is_dir():
    raise SystemExit(f"RELEASE_ASSET_DIR is not a directory: {asset_root}")

with image_metadata.open(encoding="utf-8") as source:
    image_rows = json.load(source)
if not isinstance(image_rows, list):
    raise SystemExit("RELEASE_IMAGE_METADATA must contain a JSON array")

required_components = {"api", "worker", "user-web", "admin-web", "docs-web"}
images = {}
for row in image_rows:
    if not isinstance(row, dict):
        raise SystemExit("image metadata entries must be objects")
    expected_keys = {"component", "repository", "tag", "digest", "version", "revision"}
    if set(row) != expected_keys:
        raise SystemExit(f"image metadata keys are invalid: {sorted(row)}")
    component = row.pop("component")
    if component in images:
        raise SystemExit(f"duplicate image metadata for {component}")
    if component not in required_components:
        raise SystemExit(f"unexpected image component: {component}")
    if not row["repository"].strip():
        raise SystemExit(f"image repository is required for {component}")
    if row["tag"] != version or row["version"] != version:
        raise SystemExit(f"image version mismatch for {component}")
    if row["revision"] != commit:
        raise SystemExit(f"image revision mismatch for {component}")
    if not digest_pattern.fullmatch(row["digest"]):
        raise SystemExit(f"image digest must be sha256:<64 lowercase hex> for {component}")
    images[component] = row
if set(images) != required_components:
    missing = sorted(required_components - set(images))
    raise SystemExit(f"image metadata is incomplete; missing: {', '.join(missing)}")

assets = {}
for checksum_path in sorted(asset_root.rglob("*.sha256")):
    if checksum_path.resolve() == Path(str(output) + ".sha256"):
        continue
    asset_path = Path(str(checksum_path)[:-7])
    if not asset_path.is_file():
        raise SystemExit(f"checksum has no matching asset: {checksum_path}")
    checksum_fields = checksum_path.read_text(encoding="utf-8").split()
    if not checksum_fields or not hex_pattern.fullmatch(checksum_fields[0].lower()):
        raise SystemExit(f"invalid SHA-256 file: {checksum_path}")
    expected = checksum_fields[0].lower()
    actual = hashlib.sha256(asset_path.read_bytes()).hexdigest()
    if actual != expected:
        raise SystemExit(f"asset checksum mismatch: {asset_path}")
    name = asset_path.relative_to(asset_root).as_posix()
    if name in assets:
        raise SystemExit(f"duplicate release asset: {name}")
    assets[name] = {"name": name, "sha256": actual}
if not assets:
    raise SystemExit("release manifest requires at least one checksummed asset")

manifest = {
    "schema_version": 1,
    "application_version": version,
    "commit": commit,
    "images": images,
    "assets": assets,
}
output.parent.mkdir(parents=True, exist_ok=True)
encoded = (json.dumps(manifest, indent=2, sort_keys=True, separators=(",", ": ")) + "\n").encode()
fd, temporary_raw = tempfile.mkstemp(prefix=f".{output.name}.", dir=output.parent)
temporary = Path(temporary_raw)
try:
    with os.fdopen(fd, "wb") as destination:
        destination.write(encoded)
        destination.flush()
        os.fsync(destination.fileno())
    os.replace(temporary, output)
finally:
    temporary.unlink(missing_ok=True)
PY

manifest_name=$(basename "$RELEASE_MANIFEST_OUTPUT")
manifest_dir=$(dirname "$RELEASE_MANIFEST_OUTPUT")
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$manifest_dir" && sha256sum "$manifest_name" > "$manifest_name.sha256")
elif command -v shasum >/dev/null 2>&1; then
  (cd "$manifest_dir" && shasum -a 256 "$manifest_name" > "$manifest_name.sha256")
else
  echo "sha256 tool is required to render a release manifest" >&2
  exit 1
fi

echo "Rendered $RELEASE_MANIFEST_OUTPUT and $RELEASE_MANIFEST_OUTPUT.sha256"
