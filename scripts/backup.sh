#!/usr/bin/env sh
set -eu
DATA_DIR="${1:-./data}"
OUT="${2:-./manual-backup-$(date +%Y%m%d-%H%M%S).zip}"
cd "$DATA_DIR"
zip -r "$OUT" storage.json uploads 2>/dev/null || zip -r "$OUT" storage.json
printf 'Backup written to %s\n' "$OUT"
