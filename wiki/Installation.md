# Installation

## Requirements

- [Golang](https://golang.org/) 1.26.1 or newer
- Valid Teams JWT files generated with [teams-token](https://github.com/fossteams/teams-token)
- A terminal with cursor-addressing support, for example Terminal.app or iTerm2

## Install From Release

Releases publish tarballs and checksums for macOS Apple Silicon, macOS Intel, and Linux (amd64, arm64).

Replace `<VERSION>` with a real tag such as `v0.2.1`, then download the archive that matches your machine plus the checksum file from the release page.

**Example for macOS Apple Silicon:**

```bash
VERSION=v0.2.1
curl -LO "https://github.com/vaishnavucv/teams-cli/releases/download/${VERSION}/teams-cli_${VERSION}_darwin_arm64.tar.gz"
curl -LO "https://github.com/vaishnavucv/teams-cli/releases/download/${VERSION}/teams-cli_${VERSION}_checksums.txt"
grep " teams-cli_${VERSION}_darwin_arm64.tar.gz$" "teams-cli_${VERSION}_checksums.txt" | shasum -a 256 -c
tar -xzf "teams-cli_${VERSION}_darwin_arm64.tar.gz"
install -m 0755 teams-cli /usr/local/bin/teams-cli
teams-cli --version
```

To verify the signed checksum file:

```bash
cosign verify-blob "teams-cli_${VERSION}_checksums.txt" \
  --bundle "teams-cli_${VERSION}_checksums.txt.sigstore.json" \
  --certificate-identity "https://github.com/vaishnavucv/teams-cli/.github/workflows/ci-release.yml@refs/heads/main" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

To verify GitHub build provenance for the same archive:

```bash
gh attestation verify "teams-cli_${VERSION}_darwin_arm64.tar.gz" \
  --repo vaishnavucv/teams-cli \
  --signer-workflow vaishnavucv/teams-cli/.github/workflows/ci-release.yml
```

**Example for Linux x86_64:**

```bash
VERSION=v0.2.1
curl -LO "https://github.com/vaishnavucv/teams-cli/releases/download/${VERSION}/teams-cli_${VERSION}_linux_amd64.tar.gz"
curl -LO "https://github.com/vaishnavucv/teams-cli/releases/download/${VERSION}/teams-cli_${VERSION}_checksums.txt"
grep " teams-cli_${VERSION}_linux_amd64.tar.gz$" "teams-cli_${VERSION}_checksums.txt" | shasum -a 256 -c
tar -xzf "teams-cli_${VERSION}_linux_amd64.tar.gz"
install -m 0755 teams-cli /usr/local/bin/teams-cli
teams-cli --version
```

## Running Locally

If `install` is not appropriate for your setup, extract the archive and move `teams-cli` to any directory already on your `PATH`.

```bash
go run ./
```

To build and run the local binary:

```bash
go build -o teams-cli .
TERM=xterm-256color ./teams-cli msg=20
```
