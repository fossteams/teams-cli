#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-}"
OUT_DIR="${2:-${ROOT_DIR}/dist}"

if [[ -z "${VERSION}" ]]; then
  echo "usage: $0 <version> [output-dir]" >&2
  exit 1
fi

if ! command -v syft >/dev/null 2>&1; then
  echo "syft is required on PATH" >&2
  exit 1
fi

shopt -s nullglob
archives=( "${OUT_DIR}"/teams-cli_"${VERSION}"_*.tar.gz )

if [[ ${#archives[@]} -eq 0 ]]; then
  echo "no release archives found in ${OUT_DIR}" >&2
  exit 1
fi

for archive in "${archives[@]}"; do
  sbom_path="${archive%.tar.gz}.spdx.json"
  syft "${archive}" -o "spdx-json=${sbom_path}"
done

echo "generated SBOMs:"
printf '  %s\n' "${OUT_DIR}"/teams-cli_"${VERSION}"_*.spdx.json
