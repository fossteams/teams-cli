# Development & CI/CD Workflow

## CI/CD Pipeline
This repository is supported by an automated GitHub Actions CI/CD pipeline defined in `.github/workflows/ci-release.yml`. 

### Why do we need it?
1. **Quality Assurance**: Automatically ensures strict Go formatting and runs unit tests.
2. **Security**: Runs CodeQL and Govulncheck to catch critical security and dependency vulnerabilities. Scans for accidentally pushed secret tokens using `gitleaks`.
3. **Consistency**: Automates the creation of predictable, digitally-signed releases across platforms (Linux, macOS, various architectures).
4. **Governance**: Enforces branch protections and reviews, preventing broken code from reaching the `main` branch.

### How it works for branches:
- **`dev` (and PRs):** Pushing to `dev` runs code formatting checks, tests, coverage validation, race condition detection, and security checks. It verifies that the code builds correctly but **does not publish a release**.
- **`main`:** Pushing to `main` executes all the validations above. Once they all pass, it checks the version in `version.go`. If it's a valid semantic version (and not "dev"), it generates the archives, signs them, attests them, and dynamically publishes a GitHub Release for the public.

## Contributing

Contributions should be opened against this fork, not the archived upstream repository.
- Open pull requests against this repository's `dev` branch
- Expect CODEOWNERS review on repository, workflow, and release-path changes
- Run `go build ./...` and `go test ./...` before sending changes
- Keep local Teams JWT files outside the repository
- See `CONTRIBUTING.md` for contributor setup and workflow

## Governance And Security Gates

Phase 1 governance for this maintained fork includes:
- `CODEOWNERS` coverage for the repository, workflows, and release scripts
- Protected `main` and `dev` branches with required status checks and pull-request review policy
- Required signed commits on protected branches
- One combined `CI and Release` workflow that runs quality, security, and release jobs

## Release Flow

Releases now come from `main`, not from pushed tags.

1. Develop on `dev` and keep `version.go` at `dev`.
2. When the next release is ready, change `version.go` to the next version such as `v0.2.1` and update `CHANGELOG.md`.
3. Merge or push that versioned change to `main`.
4. The combined GitHub Actions workflow runs CI and holds the release job behind the protected `release` environment for manual approval.
5. After approval, the workflow automatically publishes a signed release with SBOMs and checksums.

## Runtime Hardening and Token Safety

- The CI pipeline runs secret detection to protect your auth.
- The TUI reads `~/.config/fossteams` correctly but skips `.gitignore` tracked tokens.
- Never commit `token-skype.jwt` or `token-teams.jwt` to the repository.
