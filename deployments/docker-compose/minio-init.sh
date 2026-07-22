#!/bin/sh
set -eu

mc alias set managed http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
mc mb --ignore-existing "managed/$STORAGE_S3_BUCKET" >/dev/null
if ! mc admin user info managed "$STORAGE_S3_ACCESS_KEY_ID" >/dev/null 2>&1; then
  mc admin user add managed "$STORAGE_S3_ACCESS_KEY_ID" "$STORAGE_S3_SECRET_ACCESS_KEY" >/dev/null
fi
policy_file=$(mktemp)
trap 'rm -f "$policy_file"' EXIT HUP INT TERM
cat >"$policy_file" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:ListBucket", "s3:GetBucketLocation"],
      "Resource": ["arn:aws:s3:::$STORAGE_S3_BUCKET"]
    },
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": ["arn:aws:s3:::$STORAGE_S3_BUCKET/*"]
    }
  ]
}
EOF
policy_name="app-$STORAGE_S3_BUCKET-rw"
mc admin policy create managed "$policy_name" "$policy_file" >/dev/null
mc admin policy attach managed "$policy_name" --user "$STORAGE_S3_ACCESS_KEY_ID" >/dev/null
