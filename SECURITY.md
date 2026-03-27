# Security Policy

This repository is a maintained fork of the archived `fossteams/teams-cli`
project. Security issues should be reported privately and must not include
Teams JWTs, authentication headers, cookies, or private message data.

## Supported Versions

| Version | Supported |
| --- | --- |
| `master` | Yes |
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

## Disclosure

Please wait for a fix or an agreed disclosure date before publishing details.
If a report turns out to be non-sensitive hardening work, it can be converted
into a normal issue after the risk is understood.
