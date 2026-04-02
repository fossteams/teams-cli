#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-}"
OUT_DIR="${2:-${ROOT_DIR}/dist}"

if [[ -z "${VERSION}" ]]; then
  echo "usage: $0 <version> [output-dir]" >&2
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "gh is required on PATH" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required on PATH" >&2
  exit 1
fi

repo="${GITHUB_REPOSITORY:-}"
if [[ -z "${repo}" ]]; then
  remote_url="$(git -C "${ROOT_DIR}" config --get remote.origin.url || true)"
  if [[ -n "${remote_url}" ]]; then
    repo="$(printf '%s\n' "${remote_url}" | sed -nE 's#.*github.com[:/]([^/]+/[^/.]+)(\.git)?$#\1#p')"
  fi
fi

if [[ -z "${repo}" ]]; then
  repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
fi

if [[ -z "${repo}" ]]; then
  echo "unable to determine GitHub repository" >&2
  exit 1
fi

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "GH_TOKEN must be set" >&2
  exit 1
fi

release_endpoint="repos/${repo}/releases/tags/${VERSION}"
release_json="$(gh api "${release_endpoint}")"
upload_url="$(
  python3 -c 'import json,sys; print(json.load(sys.stdin)["upload_url"].split("{", 1)[0])' \
    <<<"${release_json}"
)"

shopt -s nullglob
assets=(
  "${OUT_DIR}"/teams-cli_"${VERSION}"_*.tar.gz
  "${OUT_DIR}"/teams-cli_"${VERSION}"_checksums.txt
  "${OUT_DIR}"/teams-cli_"${VERSION}"_checksums.txt.sigstore.json
  "${OUT_DIR}"/teams-cli_"${VERSION}"_sboms.tar.gz
)

if [[ ${#assets[@]} -eq 0 ]]; then
  echo "no release assets found in ${OUT_DIR}" >&2
  exit 1
fi

for asset in "${assets[@]}"; do
  if [[ ! -f "${asset}" ]]; then
    continue
  fi

  name="$(basename "${asset}")"

  existing_asset_id="$(
    gh api "${release_endpoint}" \
      --jq ".assets[]? | select(.name == \"${name}\") | .id" 2>/dev/null || true
  )"

  if [[ -n "${existing_asset_id}" ]]; then
    gh api -X DELETE "repos/${repo}/releases/assets/${existing_asset_id}" >/dev/null

    for _ in {1..10}; do
      sleep 1
      existing_asset_id="$(
        gh api "${release_endpoint}" \
          --jq ".assets[]? | select(.name == \"${name}\") | .id" 2>/dev/null || true
      )"
      if [[ -z "${existing_asset_id}" ]]; then
        break
      fi
    done

    if [[ -n "${existing_asset_id}" ]]; then
      echo "asset ${name} still exists after delete attempt" >&2
      exit 1
    fi
  fi

  encoded_name="$(python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1]))' "${name}")"
  content_type="$(file --brief --mime-type "${asset}" 2>/dev/null || true)"
  if [[ -z "${content_type}" ]]; then
    content_type="application/octet-stream"
  fi
  content_length="$(wc -c < "${asset}" | tr -d ' ')"

  curl --fail --silent --show-error \
    -X POST \
    -H "Accept: application/vnd.github+json" \
    -H "Authorization: Bearer ${GH_TOKEN}" \
    -H "Content-Type: ${content_type}" \
    -H "Content-Length: ${content_length}" \
    --data-binary @"${asset}" \
    "${upload_url}?name=${encoded_name}" >/dev/null
done

echo "uploaded release assets:"
printf '  %s\n' "${assets[@]}"
