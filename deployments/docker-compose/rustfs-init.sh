#!/bin/sh
set -eu

# The managed RustFS deployment intentionally provisions no separate IAM user:
# RustFS user administration uses its own admin API/console and is not
# compatible with `mc admin`, so the application talks to RustFS with the
# managed root credentials over the Docker-internal network.
if ! aws s3api head-bucket --bucket "$STORAGE_S3_BUCKET" >/dev/null 2>&1; then
  aws s3api create-bucket --bucket "$STORAGE_S3_BUCKET" >/dev/null
fi
