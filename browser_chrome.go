package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

const tokenReceivedPage = `<!doctype html><html><body style="font-family:sans-serif;padding:2em">` +
	`<h3>Token received</h3><p>You can return to teams-cli.</p></body></html>`

func defaultProfileDir() (string, error) {
	dir, err := defaultTokenDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "browser-profile"), nil
}

// chromeBrowser drives a Chrome-family browser over the DevTools protocol.
type chromeBrowser struct {
	ctx         context.Context
	cancel      context.CancelFunc
	allocCancel context.CancelFunc
	jarPath     string
	logf        func(string, ...any)
}

// ensureProfilePreferences makes the profile restore its previous session on
// startup. Microsoft's sign-in cookies are session cookies, and Chrome only
// reloads session cookies from disk when the profile is set to continue
// where it left off (or when the last run crashed). Without this, every
// clean exit throws the sign-in away and the next run asks for a login.
func ensureProfilePreferences(profileDir string) error {
	defaultDir := filepath.Join(profileDir, "Default")
	if err := os.MkdirAll(defaultDir, 0o700); err != nil {
		return fmt.Errorf("unable to create browser profile %s: %v", profileDir, err)
	}

	path := filepath.Join(defaultDir, "Preferences")
	prefs := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &prefs); err != nil {
			return fmt.Errorf("unable to parse %s: %v", path, err)
		}
	}

	session, _ := prefs["session"].(map[string]any)
	if session == nil {
		session = map[string]any{}
	}
	if session["restore_on_startup"] == float64(1) {
		return nil
	}
	session["restore_on_startup"] = 1
	prefs["session"] = session

	data, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func newChromeBrowser(parent context.Context, execPath, profileDir, jarPath string, headless bool, logf func(string, ...any)) (*chromeBrowser, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, fmt.Errorf("unable to create browser profile %s: %v", profileDir, err)
	}
	if err := ensureProfilePreferences(profileDir); err != nil {
		return nil, err
	}

	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.ExecPath(execPath),
		chromedp.UserDataDir(profileDir),
		chromedp.WindowSize(900, 760),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		// Encrypt profile cookies with the mock keychain, as Playwright and
		// Puppeteer do. Without this the OS keychain key is used, the profile
		// cannot be shared with those tools, and Chrome silently drops any
		// cookies it cannot decrypt, which destroys the saved sign-in.
		chromedp.Flag("use-mock-keychain", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-features", "DestroyProfileOnBrowserClose,Translate"),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-extensions", true),
	}
	if headless {
		// Headless Chrome announces itself in the user agent; identity
		// providers sometimes refuse that, so present the regular one.
		opts = append(opts, chromedp.Headless, chromedp.Flag("user-agent", desktopUserAgent()))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(parent, opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)
	browser := &chromeBrowser{ctx: ctx, cancel: cancel, allocCancel: allocCancel, jarPath: jarPath, logf: logf}

	// Keep the real Teams web app from loading at the redirect URI: serve a
	// tiny page instead so the fragment stays put and nothing heavy runs.
	paused := chromedp.Events(ctx, fetch.RequestPaused)
	if _, err := chromedp.Call(ctx, fetch.Enable, fetch.EnableParams{
		Patterns: []*fetch.RequestPattern{{URLPattern: "https://teams.microsoft.com/*", RequestStage: fetch.RequestStageRequest}},
	}); err != nil {
		browser.Close()
		return nil, fmt.Errorf("unable to start browser: %v", err)
	}
	go func() {
		for ev, err := range paused {
			if err != nil {
				return
			}
			if ev.ResourceType == network.ResourceTypeDocument {
				_, _ = chromedp.Call(ctx, fetch.FulfillRequest, fetch.FulfillRequestParams{
					RequestID:       ev.RequestID,
					ResponseCode:    200,
					ResponseHeaders: []*fetch.HeaderEntry{{Name: "Content-Type", Value: "text/html; charset=utf-8"}},
					Body:            []byte(tokenReceivedPage),
				})
				continue
			}
			_, _ = chromedp.Call(ctx, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: ev.RequestID})
		}
	}()

	browser.injectCookies()
	return browser, nil
}

// injectCookies seeds the browser with the saved sign-in cookies, promoting
// session cookies to a concrete expiry so the silent flow can reuse them.
func (b *chromeBrowser) injectCookies() {
	if b.jarPath == "" {
		return
	}
	params, err := loadCookieParams(b.jarPath, time.Now(), 12*time.Hour)
	if err != nil {
		b.logf("ignoring saved sign-in cookies: %v", err)
		return
	}
	if len(params) == 0 {
		return
	}
	if _, err := chromedp.Call(b.ctx, storage.SetCookies, storage.SetCookiesParams{Cookies: params}); err != nil {
		b.logf("unable to restore saved sign-in cookies: %v", err)
		return
	}
	b.logf("restored %d saved sign-in cookies", len(params))
}

// saveCookies exports the browser's current cookies so a later run can reuse
// the sign-in. It is called after a successful token flow.
func (b *chromeBrowser) saveCookies() {
	if b.jarPath == "" {
		return
	}
	result, err := chromedp.Call(b.ctx, storage.GetCookies, storage.GetCookiesParams{})
	if err != nil {
		b.logf("unable to read cookies to save: %v", err)
		return
	}
	if !hasAuthCookies(result.Cookies) {
		b.logf("no sign-in cookies to save")
		return
	}
	if err := saveCookies(b.jarPath, result.Cookies); err != nil {
		b.logf("unable to save sign-in cookies: %v", err)
		return
	}
	b.logf("saved sign-in cookies for %s", cookieDomainsSummary(result.Cookies))
}

// ObtainRedirect navigates to target and returns the first main-frame URL
// that carries an OAuth response fragment, or the context's error.
func (b *chromeBrowser) ObtainRedirect(ctx context.Context, target string) (string, error) {
	lctx, cancel := context.WithCancel(b.ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()

	found := make(chan string, 2)
	navigated := chromedp.Events(lctx, page.FrameNavigated)
	within := chromedp.Events(lctx, page.NavigatedWithinDocument)
	go func() {
		for ev, err := range navigated {
			if err != nil {
				return
			}
			if ev.Frame == nil || ev.Frame.ParentID != "" {
				continue
			}
			if candidate := ev.Frame.URL + ev.Frame.URLFragment; isTokenRedirect(candidate) {
				found <- candidate
				return
			}
		}
	}()
	go func() {
		for ev, err := range within {
			if err != nil {
				return
			}
			if isTokenRedirect(ev.URL) {
				found <- ev.URL
				return
			}
		}
	}()

	go func() {
		// Navigate waits for the page load; errors here surface through the
		// context when the browser goes away, so the result is not needed.
		_ = chromedp.Do(lctx, chromedp.Navigate(target))
	}()

	select {
	case redirect := <-found:
		return redirect, nil
	case <-ctx.Done():
		return "", fmt.Errorf("%w (browser was at %s)", ctx.Err(), b.currentLocation())
	case <-b.ctx.Done():
		return "", fmt.Errorf("browser closed: %w", errSignInRequired)
	}
}

// currentLocation reports the main frame's URL without query or fragment, for
// diagnostics when a sign-in does not complete.
func (b *chromeBrowser) currentLocation() string {
	lctx, cancel := context.WithTimeout(b.ctx, 3*time.Second)
	defer cancel()
	raw, err := chromedp.Run(lctx, chromedp.Location())
	if err != nil {
		return "unknown"
	}
	if idx := strings.IndexAny(raw, "?#"); idx >= 0 {
		raw = raw[:idx]
	}
	return raw
}

func (b *chromeBrowser) Close() {
	if b.cancel != nil {
		b.cancel()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
}

// chromeFlowLauncher returns a launcher that runs the whole token flow in a
// Chrome profile, headless or visible.
func chromeFlowLauncher(opts refreshOptions) tokenFlowLauncher {
	return func(ctx context.Context, headless bool) (tokenFlowResult, error) {
		execPath, err := findBrowserExecutable(firstNonEmpty(opts.BrowserPath, os.Getenv("TEAMS_CLI_BROWSER")), defaultBrowserCandidates())
		if err != nil {
			return tokenFlowResult{}, err
		}
		profileDir := opts.ProfileDir
		if profileDir == "" {
			if profileDir, err = defaultProfileDir(); err != nil {
				return tokenFlowResult{}, err
			}
		}

		jarPath := opts.CookieJarPath
		if jarPath == "" {
			jarPath = cookieJarPath(profileDir)
		}
		logf := opts.logf()
		browser, err := newChromeBrowser(ctx, execPath, profileDir, jarPath, headless, logf)
		if err != nil {
			return tokenFlowResult{}, err
		}
		defer browser.Close()

		flow := tokenFlowOptions{
			ChooseTenant:    opts.ChooseTenant,
			FirstTimeout:    interactiveTokenTimeout,
			FollowUpTimeout: followUpTokenTimeout,
		}
		if headless {
			flow.FirstTimeout = headlessTokenTimeout
			flow.FollowUpTimeout = headlessTokenTimeout
		}

		result, err := acquireTokens(ctx, browser, flow)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				return result, errTokenFlowTimeout
			}
			return result, err
		}
		browser.saveCookies()
		return result, nil
	}
}

func desktopUserAgent() string {
	platform := "X11; Linux x86_64"
	switch runtime.GOOS {
	case "darwin":
		platform = "Macintosh; Intel Mac OS X 10_15_7"
	case "windows":
		platform = "Windows NT 10.0; Win64; x64"
	}
	return "Mozilla/5.0 (" + platform + ") AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// describeTokenExpiry renders "expires 2026-10-27 11:02 EDT (in 58 min)".
func describeTokenExpiry(token string, now time.Time) string {
	meta, err := parseJWTMetadata(token)
	if err != nil || !meta.HasExpiry {
		return "no expiry"
	}
	minutes := int(meta.ExpiresAt.Sub(now).Minutes())
	return fmt.Sprintf("expires %s (in %d min)", meta.ExpiresAt.Local().Format("2006-01-02 15:04 MST"), minutes)
}
