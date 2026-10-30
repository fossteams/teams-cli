package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"
)

func refreshOptionsFromApp(options AppOptions, interactive bool, progress func(string)) refreshOptions {
	return refreshOptions{
		TokenDir:    options.TokenDir,
		ProfileDir:  options.ProfileDir,
		BrowserPath: options.BrowserPath,
		Interactive: interactive,
		ForceHeaded: options.ForceHeaded,
		Progress:    progress,
	}
}

// runRefreshTokens implements `teams-cli refresh-token`.
func runRefreshTokens(ctx context.Context, out io.Writer, in io.Reader, options AppOptions) error {
	if options.Logout {
		profileDir := options.ProfileDir
		if profileDir == "" {
			var err error
			if profileDir, err = defaultProfileDir(); err != nil {
				return err
			}
		}
		if err := os.RemoveAll(profileDir); err != nil {
			return fmt.Errorf("unable to remove browser profile %s: %v", profileDir, err)
		}
		// The saved sign-in cookies live beside the profile and would sign the
		// next run straight back in, so a logout has to drop them too.
		jarPath := cookieJarPath(profileDir)
		if err := os.Remove(jarPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("unable to remove saved sign-in cookies %s: %v", jarPath, err)
		}
		fmt.Fprintf(out, "Removed browser profile %s\nRemoved saved sign-in cookies %s\n", profileDir, jarPath)
		return nil
	}

	opts := refreshOptionsFromApp(options, !options.SilentOnly, func(message string) { fmt.Fprintln(out, message) })
	opts.ChooseTenant = func(tenants []tenantInfo) (string, error) { return promptForTenant(out, in, tenants) }

	result, err := refreshTokens(ctx, opts, chromeFlowLauncher(opts))
	if err != nil {
		return err
	}

	tokenDir := options.TokenDir
	if tokenDir == "" {
		tokenDir, _ = defaultTokenDir()
	}
	if result.Principal != "" {
		fmt.Fprintf(out, "Signed in as %s (tenant %s)\n", result.Principal, result.TenantID)
	}
	now := time.Now()
	for _, tokenType := range []string{tokenTypeTeams, tokenTypeSkype, tokenTypeChatSvcAgg} {
		if token, ok := result.Tokens[tokenType]; ok {
			fmt.Fprintf(out, "  %s  %s\n", tokenFilePath(tokenDir, tokenType), describeTokenExpiry(token, now))
		}
	}
	return nil
}

func promptForTenant(out io.Writer, in io.Reader, tenants []tenantInfo) (string, error) {
	if len(tenants) == 0 {
		return "", fmt.Errorf("account has no Teams tenants")
	}
	if len(tenants) == 1 {
		return tenants[0].TenantID, nil
	}

	fmt.Fprintln(out, "\nThis account belongs to several tenants:")
	for idx, tenant := range tenants {
		fmt.Fprintf(out, "  %d. %s  (%s)\n", idx+1, tenant.TenantName, tenant.TenantID)
	}
	reader := bufio.NewReader(in)
	for {
		fmt.Fprintf(out, "Pick a tenant [1-%d]: ", len(tenants))
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("no tenant chosen")
		}
		choice, err := strconv.Atoi(strings.TrimSpace(line))
		if err == nil && choice >= 1 && choice <= len(tenants) {
			return tenants[choice-1].TenantID, nil
		}
	}
}
