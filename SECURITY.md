# Security Policy

This repository is a maintained fork of the archived `fossteams/teams-cli`
project. Security issues should be reported privately and must not include
Teams JWTs, authentication headers, cookies, or private message data.

## Supported Versions

| Version | Supported |
| --- | --- |
| `main` | Yes |
| `dev` | Best effort |
| Latest tagged release | Yes |
| Older tags and archived upstream snapshots | No |

## Reporting a Vulnerability

Preferred path:

1. Use GitHub private vulnerability reporting on this fork if it is available.
2. Otherwise contact the maintainer account directly on GitHub before opening a
   public issue.

Do not open a public issue for:

- token leakage or token-handling flaws
- authentication bypasses
- log redaction failures
- release artifact or CI supply-chain concerns
- anything that would require posting secrets or private Teams data

## What to Include

Share only the minimum details needed to reproduce and triage the issue:

- affected version, tag, or commit
- operating system and architecture
- whether the issue affects runtime behavior, logging, release artifacts, or CI
- sanitized reproduction steps
- impact and expected severity
- any proposed mitigation if you already have one

If logs are needed, redact or replace:

- JWTs and bearer tokens
- `Authorization` or `Authentication` headers
- cookies
- tenant-specific secrets or confidential message content

## Response Targets

- Initial acknowledgement: within 5 business days
- Triage update: within 14 calendar days
- Fix timeline: depends on severity and reproducibility

## Scope

The most security-sensitive areas in this fork are:

- token discovery and export
- request authentication and retry behavior
- local log storage and redaction
- release artifacts and CI workflows
- repository governance and protected-branch policy

## Preventive Controls

This maintained fork uses layered preventive controls in addition to manual
review:

- protected `dev` and `main` branches
- CODEOWNERS coverage for repository-critical paths
- required CI status checks before normal merges
- required signed commits on protected branches
- CodeQL analysis for Go code
- dependency review on pull requests
- secret scanning in CI
- protected `release` environment approval before publication
- release smoke tests, SPDX SBOMs, keyless cosign signatures, and GitHub
  provenance attestations for published release artifacts

These checks reduce risk, but they do not replace careful review of token
handling, request authentication, or release artifacts.

## Trusted Release Verification

Consumers should prefer release assets that can be validated through all of the
following:

- checksum verification
- cosign bundle verification against the GitHub Actions workflow identity
- GitHub attestation verification for build provenance
- SBOM review for the specific archive being installed

If any of these materials are missing or fail validation, treat the release as
suspect until it is reviewed.

## Disclosure

Please wait for a fix or an agreed disclosure date before publishing details.
If a report turns out to be non-sensitive hardening work, it can be converted
into a normal issue after the risk is understood.
