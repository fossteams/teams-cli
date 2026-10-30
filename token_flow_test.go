package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthorizeURLBuildsImplicitGrantRequest(t *testing.T) {
	for _, tokenType := range []string{tokenTypeTeams, tokenTypeSkype, tokenTypeChatSvcAgg} {
		parsed, err := url.Parse(authorizeURL(tokenType, "contoso"))
		if err != nil {
			t.Fatal(err)
		}
		query := parsed.Query()
		if parsed.Host != "login.microsoftonline.com" || parsed.Path != "/contoso/oauth2/authorize" {
			t.Fatalf("unexpected authorize endpoint %s", parsed)
		}
		if query.Get("client_id") != teamsClientID || query.Get("redirect_uri") != tokenRedirectURI {
			t.Fatalf("unexpected client or redirect in %s", parsed)
		}
		if tokenType == tokenTypeTeams {
			if query.Get("response_type") != "id_token" || query.Get("resource") != "" {
				t.Fatalf("expected id_token request for teams, got %s", parsed)
			}
			continue
		}
		if query.Get("response_type") != "token" || query.Get("resource") != tokenAudiences[tokenType] {
			t.Fatalf("expected access token request for %s, got %s", tokenType, parsed)
		}
	}
}

func TestIsTokenRedirectAcceptsAnyTeamsPathWithTokenFragment(t *testing.T) {
	cases := map[string]bool{
		"https://teams.microsoft.com/go#id_token=a.b.c":                 true,
		"https://teams.microsoft.com/v2/#access_token=a.b.c&state=x":    true,
		"https://teams.microsoft.com/v2/#error=interaction_required":    true,
		"https://teams.microsoft.com/v2/":                               false,
		"https://teams.microsoft.com/v2/#other=1":                       false,
		"https://evil.example/#id_token=a.b.c":                          false,
		"https://login.microsoftonline.com/common/oauth2/authorize?x=1": false,
	}
	for raw, want := range cases {
		if got := isTokenRedirect(raw); got != want {
			t.Fatalf("isTokenRedirect(%q) = %t, want %t", raw, got, want)
		}
	}
}

// fakeBrowser replays scripted redirects for each navigation.
type fakeBrowser struct {
	t         *testing.T
	redirects []string
	visited   []string
}

func (f *fakeBrowser) ObtainRedirect(ctx context.Context, target string) (string, error) {
	f.visited = append(f.visited, target)
	if len(f.redirects) == 0 {
		return "", errors.New("no scripted redirect")
	}
	next := f.redirects[0]
	f.redirects = f.redirects[1:]
	if next == "" {
		return "", errTokenFlowTimeout
	}
	return next, nil
}

func TestAcquireTokensWalksTeamsThenSkypeThenChatSvcAgg(t *testing.T) {
	teams := testJWT(t, map[string]any{"aud": teamsClientID, "tid": "tenant-1", "upn": "me@contoso.com"})
	skype := testJWT(t, map[string]any{"aud": skypeResource, "tid": "tenant-1"})
	chat := testJWT(t, map[string]any{"aud": chatSvcAggResource, "tid": "tenant-1"})
	browser := &fakeBrowser{t: t, redirects: []string{
		"https://teams.microsoft.com/go#id_token=" + teams,
		"https://teams.microsoft.com/v2/#access_token=" + skype,
		"https://teams.microsoft.com/go#access_token=" + chat,
	}}

	result, err := acquireTokens(context.Background(), browser, tokenFlowOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Tokens[tokenTypeTeams] != teams || result.Tokens[tokenTypeSkype] != skype || result.Tokens[tokenTypeChatSvcAgg] != chat {
		t.Fatalf("unexpected tokens %v", result.Tokens)
	}
	if result.Principal != "me@contoso.com" || result.TenantID != "tenant-1" {
		t.Fatalf("unexpected identity %+v", result)
	}
	if len(browser.visited) != 3 || !strings.Contains(browser.visited[0], "/common/") || !strings.Contains(browser.visited[1], "/tenant-1/") {
		t.Fatalf("unexpected navigation sequence %v", browser.visited)
	}
}

func TestAcquireTokensRejectsWrongAudience(t *testing.T) {
	wrong := testJWT(t, map[string]any{"aud": "someone-else", "tid": "tenant-1"})
	browser := &fakeBrowser{t: t, redirects: []string{"https://teams.microsoft.com/go#id_token=" + wrong}}

	if _, err := acquireTokens(context.Background(), browser, tokenFlowOptions{}); err == nil || !strings.Contains(err.Error(), "audience") {
		t.Fatalf("expected audience error, got %v", err)
	}
}

func TestAcquireTokensSurfacesEntraError(t *testing.T) {
	browser := &fakeBrowser{t: t, redirects: []string{"https://teams.microsoft.com/go#error=interaction_required&error_description=AADSTS50058%3a+silent+sign-in+failed"}}

	_, err := acquireTokens(context.Background(), browser, tokenFlowOptions{})
	if err == nil || !strings.Contains(err.Error(), "interaction_required") || !strings.Contains(err.Error(), "silent sign-in failed") {
		t.Fatalf("expected Entra error details, got %v", err)
	}
	if !errors.Is(err, errSignInRequired) {
		t.Fatalf("expected interaction_required to mean sign-in is required, got %v", err)
	}
}

func TestAcquireTokensTimeoutMeansSignInRequired(t *testing.T) {
	browser := &fakeBrowser{t: t, redirects: []string{""}}

	_, err := acquireTokens(context.Background(), browser, tokenFlowOptions{})
	if !errors.Is(err, errSignInRequired) {
		t.Fatalf("expected timeout to be reported as sign-in required, got %v", err)
	}
}

func TestAcquireTokensResolvesTenantForPersonalAccounts(t *testing.T) {
	teams := testJWT(t, map[string]any{"aud": teamsClientID, "tid": microsoftPersonalTenantID})
	probe := testJWT(t, map[string]any{"aud": skypeResource, "tid": microsoftPersonalTenantID})
	skype := testJWT(t, map[string]any{"aud": skypeResource, "tid": "org-tenant"})
	chat := testJWT(t, map[string]any{"aud": chatSvcAggResource, "tid": "org-tenant"})
	browser := &fakeBrowser{t: t, redirects: []string{
		"https://teams.microsoft.com/go#id_token=" + teams,
		"https://teams.microsoft.com/go#access_token=" + probe,
		"https://teams.microsoft.com/go#access_token=" + skype,
		"https://teams.microsoft.com/go#access_token=" + chat,
	}}
	var probed string
	opts := tokenFlowOptions{
		ListTenants: func(ctx context.Context, bearer string) ([]tenantInfo, error) {
			probed = bearer
			return []tenantInfo{{TenantID: "org-tenant", TenantName: "Org"}}, nil
		},
	}

	result, err := acquireTokens(context.Background(), browser, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if probed != probe || result.TenantID != "org-tenant" {
		t.Fatalf("expected tenant lookup with probe token, got probed=%t tenant=%q", probed != "", result.TenantID)
	}
	if !strings.Contains(browser.visited[2], "/org-tenant/") {
		t.Fatalf("expected skype request against resolved tenant, got %v", browser.visited)
	}
}

func TestSaveTokensWritesPrivateFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	tokens := map[string]string{tokenTypeTeams: "t", tokenTypeSkype: "s", tokenTypeChatSvcAgg: "c"}

	if err := saveTokens(dir, tokens); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for tokenType, want := range tokens {
		path := tokenFilePath(dir, tokenType)
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("expected %s to hold %q, got %q err=%v", path, want, data, err)
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("expected 0600 on %s, got %v", path, info.Mode().Perm())
		}
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Fatalf("expected 0700 token dir, got %v", info.Mode().Perm())
	}
}

func TestFindBrowserExecutablePrefersOverrideThenKnownThenPlaywright(t *testing.T) {
	root := t.TempDir()
	override := filepath.Join(root, "custom-chrome")
	known := filepath.Join(root, "known-chrome")
	pwOld := filepath.Join(root, "ms-playwright", "chromium-1200", "chrome-mac-arm64", "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")
	pwNew := filepath.Join(root, "ms-playwright", "chromium-1248", "chrome-mac-arm64", "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")
	for _, p := range []string{override, known, pwOld, pwNew} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	candidates := browserCandidates{Known: []string{filepath.Join(root, "missing"), known}, PlaywrightCache: filepath.Join(root, "ms-playwright")}

	if got, _ := findBrowserExecutable(override, candidates); got != override {
		t.Fatalf("expected override to win, got %q", got)
	}
	if got, _ := findBrowserExecutable("", candidates); got != known {
		t.Fatalf("expected known browser, got %q", got)
	}
	candidates.Known = nil
	if got, _ := findBrowserExecutable("", candidates); got != pwNew {
		t.Fatalf("expected newest playwright chromium, got %q", got)
	}
	candidates.PlaywrightCache = filepath.Join(root, "nowhere")
	if _, err := findBrowserExecutable("", candidates); err == nil {
		t.Fatal("expected an error when no browser is available")
	}
}

type scriptedLaunch struct {
	headless []bool
	results  []tokenFlowResult
	errs     []error
}

func (s *scriptedLaunch) run(ctx context.Context, headless bool) (tokenFlowResult, error) {
	s.headless = append(s.headless, headless)
	idx := len(s.headless) - 1
	return s.results[idx], s.errs[idx]
}

func TestRefreshTokensFallsBackToHeadedWhenAllowed(t *testing.T) {
	launch := &scriptedLaunch{
		results: []tokenFlowResult{{}, {Tokens: map[string]string{tokenTypeSkype: "s", tokenTypeChatSvcAgg: "c", tokenTypeTeams: "t"}}},
		errs:    []error{errSignInRequired, nil},
	}
	dir := t.TempDir()

	result, err := refreshTokens(context.Background(), refreshOptions{TokenDir: dir, Interactive: true}, launch.run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(launch.headless) != 2 || !launch.headless[0] || launch.headless[1] {
		t.Fatalf("expected headless attempt then headed attempt, got %v", launch.headless)
	}
	if result.Tokens[tokenTypeSkype] != "s" {
		t.Fatalf("unexpected result %+v", result)
	}
	if _, err := os.Stat(tokenFilePath(dir, tokenTypeSkype)); err != nil {
		t.Fatalf("expected tokens saved, got %v", err)
	}
}

func TestRefreshTokensReportsSignInRequiredWhenNotInteractive(t *testing.T) {
	launch := &scriptedLaunch{results: []tokenFlowResult{{}}, errs: []error{errSignInRequired}}

	_, err := refreshTokens(context.Background(), refreshOptions{TokenDir: t.TempDir(), Interactive: false}, launch.run)
	if !errors.Is(err, errSignInRequired) || len(launch.headless) != 1 {
		t.Fatalf("expected a single headless attempt reporting sign-in required, got %v after %v", err, launch.headless)
	}
}

func TestRefreshTokensHeadedFlagSkipsHeadlessAttempt(t *testing.T) {
	launch := &scriptedLaunch{
		results: []tokenFlowResult{{Tokens: map[string]string{tokenTypeSkype: "s", tokenTypeChatSvcAgg: "c"}}},
		errs:    []error{nil},
	}

	if _, err := refreshTokens(context.Background(), refreshOptions{TokenDir: t.TempDir(), Interactive: true, ForceHeaded: true}, launch.run); err != nil {
		t.Fatal(err)
	}
	if len(launch.headless) != 1 || launch.headless[0] {
		t.Fatalf("expected one headed attempt, got %v", launch.headless)
	}
}

func TestTokensExpireWithin(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeTestJWT(t, dir, tokenTypeSkype, now.Add(30*time.Minute))
	writeTestJWT(t, dir, tokenTypeChatSvcAgg, now.Add(3*time.Minute))

	if !tokensExpireWithin(dir, now, 5*time.Minute) {
		t.Fatal("expected chatsvcagg token nearing expiry to be detected")
	}
	if tokensExpireWithin(dir, now, time.Minute) {
		t.Fatal("expected tokens outside the window to pass")
	}
	if !tokensExpireWithin(filepath.Join(dir, "missing"), now, time.Minute) {
		t.Fatal("expected missing tokens to count as needing refresh")
	}
}

func TestIsAuthFailure(t *testing.T) {
	cases := map[error]bool{
		&messageFetchStatusError{StatusCode: 401, Status: "401 Unauthorized"}:                                    true,
		&messageFetchStatusError{StatusCode: 403, Status: "403 Forbidden"}:                                       true,
		&messageFetchStatusError{StatusCode: 500, Status: "500 Internal Server Error"}:                           false,
		errors.New("unable to get pinned channels: invalid status code returned: 200 expected but 401 returned"): true,
		errors.New("unable to send message: unexpected status 403 Forbidden: nope"):                              true,
		errors.New("dial tcp: timeout"):                                                                          false,
		nil:                                                                                                      false,
	}
	for err, want := range cases {
		if got := isAuthFailure(err); got != want {
			t.Fatalf("isAuthFailure(%v) = %t, want %t", err, got, want)
		}
	}
}

func TestEnsureProfilePreferencesKeepsSessionCookies(t *testing.T) {
	profile := t.TempDir()

	if err := ensureProfilePreferences(profile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	prefs := readPrefs(t, profile)
	if got := prefs["session"].(map[string]any)["restore_on_startup"]; got != float64(1) {
		t.Fatalf("expected restore_on_startup=1 so session cookies survive restarts, got %v", got)
	}

	// Existing preferences are merged, not replaced.
	path := filepath.Join(profile, "Default", "Preferences")
	if err := os.WriteFile(path, []byte(`{"profile":{"name":"keep"},"session":{"other":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureProfilePreferences(profile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	prefs = readPrefs(t, profile)
	if prefs["profile"].(map[string]any)["name"] != "keep" || prefs["session"].(map[string]any)["other"] != true {
		t.Fatalf("expected existing preferences preserved, got %v", prefs)
	}
	if got := prefs["session"].(map[string]any)["restore_on_startup"]; got != float64(1) {
		t.Fatalf("expected restore_on_startup set alongside existing keys, got %v", got)
	}
}

func readPrefs(t *testing.T, profile string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(profile, "Default", "Preferences"))
	if err != nil {
		t.Fatal(err)
	}
	var prefs map[string]any
	if err := json.Unmarshal(data, &prefs); err != nil {
		t.Fatal(err)
	}
	return prefs
}

func TestExportTokensToEnvOverridesStaleEnvironment(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeTestJWT(t, dir, tokenTypeSkype, now.Add(time.Hour))
	writeTestJWT(t, dir, tokenTypeChatSvcAgg, now.Add(time.Hour))
	t.Setenv(tokenEnvName(tokenTypeSkype), "stale-skype")
	t.Setenv(tokenEnvName(tokenTypeChatSvcAgg), "stale-chat")

	if err := exportTokensToEnv(dir); err != nil {
		t.Fatalf("export: %v", err)
	}

	for _, tokenType := range runtimeTokenTypes {
		want, _ := os.ReadFile(tokenFilePath(dir, tokenType))
		if got := os.Getenv(tokenEnvName(tokenType)); got != string(want) {
			t.Fatalf("expected %s exported from file, got %q", tokenEnvName(tokenType), got)
		}
	}
}

func TestExportTokensToEnvUsesDefaultDirWhenUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "fossteams")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	writeTestJWT(t, dir, tokenTypeSkype, now.Add(time.Hour))
	writeTestJWT(t, dir, tokenTypeChatSvcAgg, now.Add(time.Hour))
	t.Setenv(tokenEnvName(tokenTypeSkype), "stale-skype")
	t.Setenv(tokenEnvName(tokenTypeChatSvcAgg), "stale-chat")

	if err := exportTokensToEnv(""); err != nil {
		t.Fatalf("export: %v", err)
	}

	want, _ := os.ReadFile(tokenFilePath(dir, tokenTypeSkype))
	if got := os.Getenv(tokenEnvName(tokenTypeSkype)); got != string(want) {
		t.Fatalf("expected skype token exported from the default directory, got %q", got)
	}
	// The startup helper keeps ignoring an unset directory so env-only setups still work.
	t.Setenv(tokenEnvName(tokenTypeSkype), "env-only")
	if err := applyTokenDirToEnv(""); err != nil || os.Getenv(tokenEnvName(tokenTypeSkype)) != "env-only" {
		t.Fatalf("expected applyTokenDirToEnv to leave the environment alone, got %q err=%v", os.Getenv(tokenEnvName(tokenTypeSkype)), err)
	}
}

func TestObtainTokenCallerDeadlineIsNotSignInRequired(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	browser := &fakeBrowser{t: t, redirects: []string{""}}

	_, _, err := obtainToken(ctx, browser, tokenTypeTeams, "common", time.Minute)
	if err == nil || errors.Is(err, errSignInRequired) {
		t.Fatalf("expected the caller's deadline to be reported as a plain failure, got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the caller's deadline error to be preserved, got %v", err)
	}
}

func TestSilentRefreshTimeoutCoversHeadlessFlow(t *testing.T) {
	// Teams token, tenant probe, skype and chatsvcagg each get a headless
	// step timeout; the outer deadline must not cut a slow but working run short.
	if silentRefreshTimeout <= 4*headlessTokenTimeout {
		t.Fatalf("silentRefreshTimeout %s cannot cover four %s headless steps", silentRefreshTimeout, headlessTokenTimeout)
	}
}

func TestRunRefreshTokensLogoutRemovesProfileAndCookieJar(t *testing.T) {
	base := t.TempDir()
	profileDir := filepath.Join(base, "profile")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	jarPath := cookieJarPath(profileDir)
	if err := os.WriteFile(jarPath, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := runRefreshTokens(context.Background(), &out, strings.NewReader(""), AppOptions{Logout: true, ProfileDir: profileDir}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := os.Stat(profileDir); !os.IsNotExist(err) {
		t.Fatalf("expected profile removed, got %v", err)
	}
	if _, err := os.Stat(jarPath); !os.IsNotExist(err) {
		t.Fatalf("expected cookie jar removed, got %v", err)
	}
	if !strings.Contains(out.String(), jarPath) {
		t.Fatalf("expected output to name the cookie jar, got %q", out.String())
	}

	// A second logout with nothing left behind is not an error.
	if err := runRefreshTokens(context.Background(), &out, strings.NewReader(""), AppOptions{Logout: true, ProfileDir: profileDir}); err != nil {
		t.Fatalf("repeat logout: %v", err)
	}
}
