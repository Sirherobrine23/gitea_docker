#!/bin/sh
set -eu

source_dir=$1
patch_root=$2
ref=${3:-main}
patch_dir=$patch_root
normalized_ref=${ref#v}
release_series=$(printf '%s\n' "$normalized_ref" | sed -nE 's/^([0-9]+)\.([0-9]+)(\..*)?$/\1.\2/p')

if [ -n "$release_series" ]; then
  for series in "$release_series" "${release_series%%.*}"; do
    if [ -d "$patch_root/$series" ]; then
      patch_dir="$patch_root/$series"
      break
    fi
  done
fi

echo "Source ref: $ref"
echo "Using patches from: $patch_dir"
find "$patch_dir" -maxdepth 1 -type f -name '*.patch' -printf '%f\n' | sort -V | while IFS= read -r patch; do
  echo "Applying patch: $patch"
  git -C "$source_dir" apply --check "$patch_dir/$patch"
  git -C "$source_dir" apply "$patch_dir/$patch"
done
