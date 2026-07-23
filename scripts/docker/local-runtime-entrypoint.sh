#!/bin/sh
set -eu

source_dir=/run/pic-gallery-config
target_dir=/app/config

install -d -o picgallery -g picgallery -m 700 "$target_dir"
for name in runtime.env install-state.json; do
  source_path="$source_dir/$name"
  target_path="$target_dir/$name"
  temporary_path="$target_dir/.$name.tmp.$$"
  if [ ! -f "$source_path" ] || [ -L "$source_path" ]; then
    echo "local runtime source is missing or unsafe: $name" >&2
    exit 1
  fi
  install -m 600 -o picgallery -g picgallery "$source_path" "$temporary_path"
  mv -f "$temporary_path" "$target_path"
done

exec su-exec picgallery "$@"
