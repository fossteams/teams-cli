#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-}"
OUT_DIR="${2:-${ROOT_DIR}/dist}"

if [[ -z "${VERSION}" ]]; then
  echo "usage: $0 <version> [output-dir]" >&2
  exit 1
fi

if ! command -v cosign >/dev/null 2>&1; then
  echo "cosign is required on PATH" >&2
  exit 1
fi

subject="${OUT_DIR}/teams-cli_${VERSION}_checksums.txt"
if [[ ! -f "${subject}" ]]; then
  echo "missing checksum file in ${OUT_DIR}" >&2
  exit 1
fi

bundle_path="${subject}.sigstore.json"

cosign sign-blob \
  --oidc-provider github-actions \
  --bundle "${bundle_path}" \
  --yes \
  "${subject}" >/dev/null

echo "generated signature bundle:"
printf '  %s\n' "${bundle_path}"
