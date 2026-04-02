## Summary

- Describe the user-visible change.
- Call out any runtime, release, or CI impact.

## Testing

- [ ] `go build ./...`
- [ ] `go test ./...`
- [ ] `go run ./ doctor` when token loading, logging, startup, or refresh behavior changes
- [ ] Other validation performed:

## Documentation

- [ ] `README.md` updated when behavior or flags changed
- [ ] `CHANGELOG.md` updated for user-visible changes
- [ ] No documentation update was needed

## Safety Checklist

- [ ] No JWTs, cookies, auth headers, or private Teams content are included
- [ ] Local binaries and machine-specific artifacts are excluded
- [ ] Logging changes preserve redaction rules
