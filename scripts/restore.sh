#!/usr/bin/env sh
set -eu
ZIP_FILE="${1:?usage: restore.sh backup.zip ./data}"
DATA_DIR="${2:-./data}"
mkdir -p "$DATA_DIR"
unzip -o "$ZIP_FILE" -d "$DATA_DIR"
printf 'Restored into %s. Restart Docs Hub now.\n' "$DATA_DIR"
