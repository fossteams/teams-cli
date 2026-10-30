package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Teams web client identity used for the implicit-grant sign-in. These match
// the upstream fossteams/teams-token tool.
const (
	teamsClientID             = "5e3ce6c0-2b1f-4285-8d4b-75ee78787346"
	microsoftPersonalTenantID = "f8cdef31-a31e-4b4a-93e4-5f571e91255a"
	skypeResource             = "https://api.spaces.skype.com"
	chatSvcAggResource        = "https://chatsvcagg.teams.microsoft.com"
	tokenRedirectURI          = "https://teams.microsoft.com/go"
	loginHost                 = "https://login.microsoftonline.com"
	tenantsURL                = "https://teams.microsoft.com/api/mt/emea/beta/users/tenants"

	headlessTokenTimeout    = 45 * time.Second
	interactiveTokenTimeout = 10 * time.Minute
	followUpTokenTimeout    = 90 * time.Second
)

var tokenAudiences = map[string]string{
	tokenTypeTeams:      teamsClientID,
	tokenTypeSkype:      skypeResource,
	tokenTypeChatSvcAgg: chatSvcAggResource,
}

var (
	// errSignInRequired means the silent flow cannot finish without a person
	// interacting with the browser (MFA, expired SSO cookies, first login).
	errSignInRequired   = errors.New("sign-in required")
	errTokenFlowTimeout = fmt.Errorf("timed out waiting for sign-in: %w", errSignInRequired)
)

// redirectBrowser drives a browser to an authorize URL and returns the URL of
// the redirect that carries the OAuth response fragment.
type redirectBrowser interface {
	ObtainRedirect(ctx context.Context, target string) (string, error)
}

type tenantInfo struct {
	TenantID   string `json:"tenantId"`
	TenantName string `json:"tenantName"`
}

type tokenFlowOptions struct {
	// ListTenants resolves the tenants of a personal account; nil uses the Teams API.
	ListTenants func(ctx context.Context, bearer string) ([]tenantInfo, error)
	// ChooseTenant picks among several tenants; nil takes the first.
	ChooseTenant func(tenants []tenantInfo) (string, error)
	// FirstTimeout bounds the initial (possibly interactive) sign-in; later
	// tokens use FollowUpTimeout. Zero values mean no extra deadline.
	FirstTimeout    time.Duration
	FollowUpTimeout time.Duration
}

type tokenFlowResult struct {
	Tokens    map[string]string
	TenantID  string
	Principal string
}

func authorizeURL(tokenType, tenantID string) string {
	params := url.Values{}
	params.Set("client_id", teamsClientID)
	params.Set("redirect_uri", tokenRedirectURI)
	params.Set("client-request-id", uuid.NewString())
	params.Set("x-client-SKU", "Js")
	params.Set("x-client-Ver", "1.0.9")
	params.Set("nonce", uuid.NewString())

	state := uuid.NewString()
	if tokenType == tokenTypeTeams {
		params.Set("response_type", "id_token")
		params.Set("state", state)
	} else {
		resource := tokenAudiences[tokenType]
		params.Set("response_type", "token")
		params.Set("resource", resource)
		params.Set("state", state+"|"+resource)
	}

	return loginHost + "/" + url.PathEscape(tenantID) + "/oauth2/authorize?" + params.Encode()
}

// isTokenRedirect reports whether rawURL is a teams.microsoft.com page whose
// fragment carries an OAuth response. Entra redirects to /go but Teams then
// bounces that to /v2/ keeping the fragment, so only the fragment is trusted.
func isTokenRedirect(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "teams.microsoft.com" || parsed.Fragment == "" {
		return false
	}
	values, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		return false
	}
	for _, key := range []string{"id_token", "access_token", "error"} {
		if values.Has(key) {
			return true
		}
	}
	return false
}

func parseRedirect(rawURL string) url.Values {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return url.Values{}
	}
	values, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		return url.Values{}
	}
	return values
}

func obtainToken(ctx context.Context, browser redirectBrowser, tokenType, tenantID string, timeout time.Duration) (string, jwtMetadata, error) {
	stepCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	redirect, err := browser.ObtainRedirect(stepCtx, authorizeURL(tokenType, tenantID))
	if err != nil {
		// Only this step's own timeout says the sign-in stalled on a person.
		// The caller giving up (shutdown, an overall deadline) says nothing
		// about that, and must not switch off silent retries.
		if ctx.Err() != nil {
			return "", jwtMetadata{}, fmt.Errorf("%s token: %w", tokenType, ctx.Err())
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return "", jwtMetadata{}, fmt.Errorf("%w: %v", errTokenFlowTimeout, err)
		}
		return "", jwtMetadata{}, err
	}

	values := parseRedirect(redirect)
	if code := values.Get("error"); code != "" {
		description := strings.TrimSpace(values.Get("error_description"))
		err := fmt.Errorf("%s: %s", code, description)
		if code == "interaction_required" || code == "login_required" || code == "consent_required" {
			return "", jwtMetadata{}, fmt.Errorf("%v: %w", err, errSignInRequired)
		}
		return "", jwtMetadata{}, err
	}

	token := values.Get("id_token")
	if token == "" {
		token = values.Get("access_token")
	}
	if token == "" {
		return "", jwtMetadata{}, fmt.Errorf("redirect for %s carried no token", tokenType)
	}

	meta, err := parseJWTMetadata(token)
	if err != nil {
		return "", jwtMetadata{}, fmt.Errorf("%s token: %v", tokenType, err)
	}
	if meta.Audience != tokenAudiences[tokenType] {
		return "", jwtMetadata{}, fmt.Errorf("unexpected audience %q on %s token", meta.Audience, tokenType)
	}

	return token, meta, nil
}

// acquireTokens performs the three authorize requests the Teams web client
// makes and returns the resulting JWTs keyed by token type.
func acquireTokens(ctx context.Context, browser redirectBrowser, opts tokenFlowOptions) (tokenFlowResult, error) {
	result := tokenFlowResult{Tokens: map[string]string{}}

	teamsToken, teamsMeta, err := obtainToken(ctx, browser, tokenTypeTeams, "common", opts.FirstTimeout)
	if err != nil {
		return result, err
	}
	result.Tokens[tokenTypeTeams] = teamsToken
	result.Principal = teamsMeta.Principal
	result.TenantID = firstStringClaim(teamsMeta.Claims, "tid")
	if result.TenantID == "" {
		result.TenantID = "common"
	}

	if result.TenantID == microsoftPersonalTenantID {
		probe, _, err := obtainToken(ctx, browser, tokenTypeSkype, "common", opts.FollowUpTimeout)
		if err != nil {
			return result, err
		}
		listTenants := opts.ListTenants
		if listTenants == nil {
			listTenants = fetchTenants
		}
		tenants, err := listTenants(ctx, probe)
		if err != nil {
			return result, fmt.Errorf("unable to list tenants: %v", err)
		}
		choose := opts.ChooseTenant
		if choose == nil {
			choose = firstTenant
		}
		result.TenantID, err = choose(tenants)
		if err != nil {
			return result, err
		}
	}

	for _, tokenType := range []string{tokenTypeSkype, tokenTypeChatSvcAgg} {
		token, _, err := obtainToken(ctx, browser, tokenType, result.TenantID, opts.FollowUpTimeout)
		if err != nil {
			return result, err
		}
		result.Tokens[tokenType] = token
	}

	return result, nil
}

func firstTenant(tenants []tenantInfo) (string, error) {
	if len(tenants) == 0 {
		return "", fmt.Errorf("account has no Teams tenants")
	}
	return tenants[0].TenantID, nil
}

func fetchTenants(ctx context.Context, bearer string) ([]tenantInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tenantsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := newMessageHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	var tenants []tenantInfo
	if err := json.NewDecoder(resp.Body).Decode(&tenants); err != nil {
		return nil, err
	}
	return tenants, nil
}

// saveTokens writes each token to token-<type>.jwt with private permissions.
func saveTokens(tokenDir string, tokens map[string]string) error {
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		return fmt.Errorf("unable to create %s: %v", tokenDir, err)
	}
	if err := os.Chmod(tokenDir, 0o700); err != nil {
		return fmt.Errorf("unable to secure %s: %v", tokenDir, err)
	}

	for tokenType, token := range tokens {
		path := tokenFilePath(tokenDir, tokenType)
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, []byte(token), 0o600); err != nil {
			return fmt.Errorf("unable to write %s: %v", tmp, err)
		}
		if err := os.Rename(tmp, path); err != nil {
			return fmt.Errorf("unable to replace %s: %v", path, err)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("unable to secure %s: %v", path, err)
		}
	}

	return nil
}

// browserCandidates lists where to look for a Chrome-family executable.
type browserCandidates struct {
	Known           []string
	PlaywrightCache string
}

func defaultBrowserCandidates() browserCandidates {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return browserCandidates{
			Known: []string{
				"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
				filepath.Join(home, "Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
				"/Applications/Chromium.app/Contents/MacOS/Chromium",
				"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
				"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			},
			PlaywrightCache: filepath.Join(home, "Library", "Caches", "ms-playwright"),
		}
	default:
		known := []string{}
		for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"} {
			for _, dir := range []string{"/usr/bin", "/usr/local/bin", "/snap/bin", "/opt/google/chrome"} {
				known = append(known, filepath.Join(dir, name))
			}
		}
		cache := filepath.Join(home, ".cache", "ms-playwright")
		if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
			cache = filepath.Join(xdg, "ms-playwright")
		}
		return browserCandidates{Known: known, PlaywrightCache: cache}
	}
}

// findBrowserExecutable returns the first usable browser: an explicit path,
// then a well-known install, then the newest Chromium in a Playwright cache.
func findBrowserExecutable(override string, candidates browserCandidates) (string, error) {
	if override != "" {
		if isExecutableFile(override) {
			return override, nil
		}
		return "", fmt.Errorf("browser %q is not an executable file", override)
	}

	for _, path := range candidates.Known {
		if isExecutableFile(path) {
			return path, nil
		}
	}

	if candidates.PlaywrightCache != "" {
		entries, _ := os.ReadDir(candidates.PlaywrightCache)
		dirs := []string{}
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), "chromium-") {
				dirs = append(dirs, entry.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
		for _, dir := range dirs {
			matches, _ := filepath.Glob(filepath.Join(candidates.PlaywrightCache, dir, "chrome-*", "*.app", "Contents", "MacOS", "*"))
			linux, _ := filepath.Glob(filepath.Join(candidates.PlaywrightCache, dir, "chrome-linux*", "chrome"))
			for _, path := range append(matches, linux...) {
				if isExecutableFile(path) {
					return path, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no Chrome or Chromium found; install Google Chrome or set TEAMS_CLI_BROWSER to a browser executable")
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}

// refreshOptions controls a token refresh run.
type refreshOptions struct {
	TokenDir      string
	ProfileDir    string
	BrowserPath   string
	CookieJarPath string
	// Interactive allows a visible browser when the silent attempt needs a person.
	Interactive bool
	// ForceHeaded skips the silent attempt entirely.
	ForceHeaded  bool
	Progress     func(message string)
	ChooseTenant func(tenants []tenantInfo) (string, error)
}

func (o refreshOptions) report(message string) {
	if o.Progress != nil {
		o.Progress(message)
	}
}

func (o refreshOptions) logf() func(string, ...any) {
	if o.Progress == nil {
		return func(string, ...any) {}
	}
	return func(format string, args ...any) { o.Progress(fmt.Sprintf(format, args...)) }
}

type tokenFlowLauncher func(ctx context.Context, headless bool) (tokenFlowResult, error)

// refreshTokens tries a silent (headless) sign-in first and, when allowed,
// falls back to a visible browser. Tokens are saved on success.
func refreshTokens(ctx context.Context, opts refreshOptions, launch tokenFlowLauncher) (tokenFlowResult, error) {
	tokenDir := opts.TokenDir
	if tokenDir == "" {
		var err error
		if tokenDir, err = defaultTokenDir(); err != nil {
			return tokenFlowResult{}, err
		}
	}

	var result tokenFlowResult
	var err error
	if !opts.ForceHeaded {
		opts.report("Refreshing Teams sign-in silently...")
		result, err = launch(ctx, true)
		if err == nil {
			return result, saveTokens(tokenDir, result.Tokens)
		}
		if !errors.Is(err, errSignInRequired) {
			return result, err
		}
		if !opts.Interactive {
			return result, err
		}
		opts.report("Silent sign-in did not complete; opening a browser window to sign in.")
	} else {
		opts.report("Opening a browser window to sign in.")
	}

	result, err = launch(ctx, false)
	if err != nil {
		return result, err
	}
	return result, saveTokens(tokenDir, result.Tokens)
}

// tokensExpireWithin reports whether any required token is missing, unreadable,
// or expires within the window.
func tokensExpireWithin(tokenDir string, now time.Time, window time.Duration) bool {
	for _, tokenType := range runtimeTokenTypes {
		token, err := resolveToken(tokenType, tokenDir)
		if err != nil {
			return true
		}
		meta, err := parseJWTMetadata(token.Value)
		if err != nil {
			return true
		}
		if meta.HasExpiry && !now.Add(window).Before(meta.ExpiresAt) {
			return true
		}
	}
	return false
}

// isAuthFailure recognises rejected credentials from either transport: the
// CLI's own status errors or the library's "401 returned" style messages.
func isAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	var statusErr *messageFetchStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden
	}
	message := err.Error()
	for _, marker := range []string{"401 returned", "403 returned", "status 401", "status 403"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
