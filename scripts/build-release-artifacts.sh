#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-}"
OUT_DIR="${2:-${ROOT_DIR}/dist}"

if [[ -z "${VERSION}" ]]; then
  echo "usage: $0 <version> [output-dir]" >&2
  exit 1
fi

mkdir -p "${OUT_DIR}"
rm -f "${OUT_DIR}"/teams-cli_*.tar.gz "${OUT_DIR}"/teams-cli_*_checksums.txt

targets=(
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
  "linux arm64"
)

cd "${ROOT_DIR}"

for target in "${targets[@]}"; do
  read -r goos goarch <<<"${target}"

  archive_name="teams-cli_${VERSION}_${goos}_${goarch}.tar.gz"
  stage_dir="$(mktemp -d "${TMPDIR:-/tmp}/teams-cli-release.XXXXXX")"
  trap 'rm -rf "${stage_dir}"' EXIT

  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "${stage_dir}/teams-cli" .

  cp LICENSE README.md "${stage_dir}/"
  tar -C "${stage_dir}" -czf "${OUT_DIR}/${archive_name}" teams-cli LICENSE README.md
  rm -rf "${stage_dir}"
  trap - EXIT
done

checksum_file="${OUT_DIR}/teams-cli_${VERSION}_checksums.txt"
(
  cd "${OUT_DIR}"
  shasum -a 256 teams-cli_"${VERSION}"_*.tar.gz > "$(basename "${checksum_file}")"
)

echo "built release artifacts:"
printf '  %s\n' "${OUT_DIR}"/teams-cli_"${VERSION}"_*.tar.gz "${checksum_file}"
