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

sbom_stage="$(mktemp -d "${TMPDIR:-/tmp}/teams-cli-sboms.XXXXXX")"
trap 'rm -rf "${sbom_stage}"' EXIT

for archive in "${archives[@]}"; do
  sbom_path="${sbom_stage}/$(basename "${archive%.tar.gz}.spdx.json")"
  syft "${archive}" -o "spdx-json=${sbom_path}"
done

sbom_bundle="${OUT_DIR}/teams-cli_${VERSION}_sboms.tar.gz"
tar -C "${sbom_stage}" -czf "${sbom_bundle}" .

echo "generated SBOM bundle:"
printf '  %s\n' "${sbom_bundle}"
