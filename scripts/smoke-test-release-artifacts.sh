#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-}"
OUT_DIR="${2:-${ROOT_DIR}/dist}"

if [[ -z "${VERSION}" ]]; then
  echo "usage: $0 <version> [output-dir]" >&2
  exit 1
fi

targets=(
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
  "linux arm64"
)

case "$(uname -s)" in
  Darwin)
    host_goos="darwin"
    ;;
  Linux)
    host_goos="linux"
    ;;
  *)
    echo "unsupported host OS for executable smoke test: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64)
    host_goarch="amd64"
    ;;
  arm64|aarch64)
    host_goarch="arm64"
    ;;
  *)
    echo "unsupported host architecture for executable smoke test: $(uname -m)" >&2
    exit 1
    ;;
esac

checksum_file="${OUT_DIR}/teams-cli_${VERSION}_checksums.txt"
if [[ ! -f "${checksum_file}" ]]; then
  echo "missing checksums file: ${checksum_file}" >&2
  exit 1
fi

for target in "${targets[@]}"; do
  read -r goos goarch <<<"${target}"
  archive="${OUT_DIR}/teams-cli_${VERSION}_${goos}_${goarch}.tar.gz"

  if [[ ! -f "${archive}" ]]; then
    echo "missing release archive: ${archive}" >&2
    exit 1
  fi

  contents="$(tar -tzf "${archive}")"
  for required in teams-cli LICENSE README.md; do
    if ! grep -qx "${required}" <<<"${contents}"; then
      echo "archive ${archive} is missing ${required}" >&2
      exit 1
    fi
  done

  if ! grep -q " $(basename "${archive}")\$" "${checksum_file}"; then
    echo "checksums file does not include $(basename "${archive}")" >&2
    exit 1
  fi
done

smoke_dir="$(mktemp -d "${TMPDIR:-/tmp}/teams-cli-smoke.XXXXXX")"
trap 'rm -rf "${smoke_dir}"' EXIT

host_archive="${OUT_DIR}/teams-cli_${VERSION}_${host_goos}_${host_goarch}.tar.gz"
tar -C "${smoke_dir}" -xzf "${host_archive}"

version_output="$("${smoke_dir}/teams-cli" --version | tr -d '\r')"
if [[ "${version_output}" != "teams-cli ${VERSION}" ]]; then
  echo "unexpected ${host_goos}_${host_goarch} binary version: ${version_output} (expected ${VERSION})" >&2
  exit 1
fi

echo "release smoke tests passed for ${VERSION}"
