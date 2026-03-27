package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveTokenPrefersEnvironmentByDefault(t *testing.T) {
	t.Setenv(tokenEnvName(tokenTypeTeams), "env-token")

	token, err := resolveToken(tokenTypeTeams, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.Source != tokenSourceEnv {
		t.Fatalf("expected env token source, got %s", token.Source)
	}
	if token.Value != "env-token" {
		t.Fatalf("expected env token value, got %q", token.Value)
	}
}

func TestResolveTokenUsesExplicitDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(tokenEnvName(tokenTypeTeams), "env-token")
	if err := os.WriteFile(filepath.Join(dir, "token-teams.jwt"), []byte("file-token"), 0o600); err != nil {
		t.Fatalf("unable to write token file: %v", err)
	}

	token, err := resolveToken(tokenTypeTeams, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.Source != tokenSourceFile {
		t.Fatalf("expected file token source, got %s", token.Source)
	}
	if token.Value != "file-token" {
		t.Fatalf("expected file token value, got %q", token.Value)
	}
}

func TestParseJWTMetadata(t *testing.T) {
	expiresAt := time.Date(2026, time.March, 27, 12, 34, 56, 0, time.UTC)
	token := testJWT(t, map[string]any{
		"aud":                "https://teams.microsoft.com",
		"preferred_username": "dev@example.com",
		"exp":                expiresAt.Unix(),
	})

	meta, err := parseJWTMetadata(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Audience != "https://teams.microsoft.com" {
		t.Fatalf("expected audience, got %q", meta.Audience)
	}
	if meta.Principal != "dev@example.com" {
		t.Fatalf("expected principal, got %q", meta.Principal)
	}
	if !meta.HasExpiry {
		t.Fatal("expected expiry to be present")
	}
	if !meta.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expiry %s, got %s", expiresAt, meta.ExpiresAt)
	}
}

func testJWT(t *testing.T, claims map[string]any) string {
	t.Helper()

	headerBytes, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatalf("unable to marshal header: %v", err)
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("unable to marshal claims: %v", err)
	}

	return base64.RawURLEncoding.EncodeToString(headerBytes) + "." +
		base64.RawURLEncoding.EncodeToString(claimsBytes) + ".signature"
}
