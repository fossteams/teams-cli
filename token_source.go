package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type tokenSource string

const (
	tokenTypeTeams      = "teams"
	tokenTypeSkype      = "skype"
	tokenTypeChatSvcAgg = "chatsvcagg"
)

var runtimeTokenTypes = []string{tokenTypeSkype, tokenTypeChatSvcAgg}
var optionalTokenTypes = []string{tokenTypeTeams}

type resolvedToken struct {
	TokenType string
	Source    tokenSource
	Location  string
	Value     string
}

const (
	tokenSourceEnv  tokenSource = "env"
	tokenSourceFile tokenSource = "file"
)

func defaultTokenDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot retrieve user home directory")
	}

	return filepath.Join(homeDir, ".config", "fossteams"), nil
}

func tokenEnvName(tokenType string) string {
	return "MS_TEAMS_" + strings.ToUpper(tokenType) + "_TOKEN"
}

func tokenFilePath(tokenDir, tokenType string) string {
	return filepath.Join(tokenDir, "token-"+tokenType+".jwt")
}

func resolveToken(tokenType, tokenDir string) (resolvedToken, error) {
	envName := tokenEnvName(tokenType)
	if tokenDir == "" {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			return resolvedToken{
				TokenType: tokenType,
				Source:    tokenSourceEnv,
				Location:  envName,
				Value:     value,
			}, nil
		}
	}

	if tokenDir == "" {
		var err error
		tokenDir, err = defaultTokenDir()
		if err != nil {
			return resolvedToken{}, err
		}
	}

	tokenPath := tokenFilePath(tokenDir, tokenType)
	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return resolvedToken{}, fmt.Errorf("unable to read %s: %v", tokenPath, err)
	}

	value := strings.TrimSpace(string(tokenBytes))
	if value == "" {
		return resolvedToken{}, fmt.Errorf("%s is empty", tokenPath)
	}

	return resolvedToken{
		TokenType: tokenType,
		Source:    tokenSourceFile,
		Location:  tokenPath,
		Value:     value,
	}, nil
}

func applyTokenDirToEnv(tokenDir string) error {
	if strings.TrimSpace(tokenDir) == "" {
		return nil
	}

	for _, tokenType := range runtimeTokenTypes {
		token, err := resolveToken(tokenType, tokenDir)
		if err != nil {
			return err
		}
		if err := os.Setenv(tokenEnvName(tokenType), token.Value); err != nil {
			return fmt.Errorf("unable to export %s from %s: %v", tokenEnvName(tokenType), token.Location, err)
		}
	}

	return nil
}

type jwtMetadata struct {
	Audience  string
	Subject   string
	Principal string
	ExpiresAt time.Time
	HasExpiry bool
	Claims    map[string]any
}

func parseJWTMetadata(token string) (jwtMetadata, error) {
	segments := strings.Split(strings.TrimSpace(token), ".")
	if len(segments) < 2 {
		return jwtMetadata{}, fmt.Errorf("token does not look like a JWT")
	}

	claimsBytes, err := decodeJWTSection(segments[1])
	if err != nil {
		return jwtMetadata{}, fmt.Errorf("unable to decode JWT claims: %v", err)
	}

	claims := map[string]any{}
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return jwtMetadata{}, fmt.Errorf("unable to parse JWT claims JSON: %v", err)
	}

	meta := jwtMetadata{
		Audience:  firstStringClaim(claims, "aud"),
		Subject:   firstStringClaim(claims, "sub"),
		Principal: firstStringClaim(claims, "preferred_username", "upn", "unique_name"),
		Claims:    claims,
	}

	if expValue, ok := claims["exp"]; ok {
		expUnix, ok := numericUnixClaim(expValue)
		if !ok {
			return jwtMetadata{}, fmt.Errorf("invalid exp claim")
		}
		meta.ExpiresAt = time.Unix(expUnix, 0)
		meta.HasExpiry = true
	}

	return meta, nil
}

func decodeJWTSection(raw string) ([]byte, error) {
	if data, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
		return data, nil
	}

	padded := raw
	if rem := len(raw) % 4; rem != 0 {
		padded += strings.Repeat("=", 4-rem)
	}

	return base64.URLEncoding.DecodeString(padded)
}

func firstStringClaim(claims map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := claims[key].(type) {
		case string:
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		case []any:
			for _, item := range value {
				if raw, ok := item.(string); ok && strings.TrimSpace(raw) != "" {
					return strings.TrimSpace(raw)
				}
			}
		}
	}

	return ""
}

func numericUnixClaim(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed, true
		}
	case string:
		var parsed int64
		_, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		if err == nil {
			return parsed, true
		}
	}

	return 0, false
}
