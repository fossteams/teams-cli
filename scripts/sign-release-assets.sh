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

shopt -s nullglob
subjects=(
  "${OUT_DIR}"/teams-cli_"${VERSION}"_*.tar.gz
  "${OUT_DIR}"/teams-cli_"${VERSION}"_checksums.txt
)

if [[ ${#subjects[@]} -eq 0 ]]; then
  echo "no release artifacts found in ${OUT_DIR}" >&2
  exit 1
fi

for subject in "${subjects[@]}"; do
  sig_path="${subject}.sig"
  bundle_path="${subject}.sigstore.json"

  cosign sign-blob \
    --oidc-provider github-actions \
    --bundle "${bundle_path}" \
    --yes \
    "${subject}" > "${sig_path}"
done

echo "generated signatures:"
printf '  %s\n' "${OUT_DIR}"/teams-cli_"${VERSION}"_*.sig "${OUT_DIR}"/teams-cli_"${VERSION}"_*.sigstore.json
