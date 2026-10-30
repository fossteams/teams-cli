package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
)

// authCookieNames are the Microsoft sign-in cookies that make a saved session
// reusable. They are normally session cookies, so Chrome drops them on a clean
// exit; the jar persists them across runs instead of relying on the profile.
var authCookieNames = map[string]struct{}{
	"ESTSAUTH":           {},
	"ESTSAUTHPERSISTENT": {},
	"ESTSAUTHLIGHT":      {},
	"SignInStateCookie":  {},
	"buid":               {},
}

func hasAuthCookies(cookies []*network.Cookie) bool {
	for _, cookie := range cookies {
		if _, ok := authCookieNames[cookie.Name]; ok {
			return true
		}
	}
	return false
}

func saveCookies(path string, cookies []*network.Cookie) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(cookies)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loadCookieParams reads saved cookies and converts them to set-cookie params.
// Session cookies and any expiring sooner than promoteTo are given a concrete
// expiry at now+promoteTo so Chrome persists them to disk; already-expired
// cookies are dropped.
func loadCookieParams(path string, now time.Time, promoteTo time.Duration) ([]*network.CookieParam, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var cookies []*network.Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, err
	}

	promoted := cdp.TimeSinceEpoch(float64(now.Add(promoteTo).Unix()))
	params := make([]*network.CookieParam, 0, len(cookies))
	for _, cookie := range cookies {
		if !cookie.Session && cookie.Expires > 0 && float64(now.Unix()) >= cookie.Expires {
			continue
		}

		expires := cdp.TimeSinceEpoch(cookie.Expires)
		if cookie.Session || cookie.Expires <= 0 || int64(cookie.Expires) < now.Add(promoteTo).Unix() {
			expires = promoted
		}

		params = append(params, &network.CookieParam{
			Name:         cookie.Name,
			Value:        cookie.Value,
			Domain:       cookie.Domain,
			Path:         cookie.Path,
			Secure:       cookie.Secure,
			HTTPOnly:     cookie.HTTPOnly,
			SameSite:     cookie.SameSite,
			Expires:      expires,
			SourceScheme: cookie.SourceScheme,
		})
	}

	return params, nil
}

// cookieJarPath places the jar beside the browser profile, so the default
// profile at ~/.config/fossteams/browser-profile keeps its cookies in
// ~/.config/fossteams/sign-in-cookies.json.
func cookieJarPath(profileDir string) string {
	return filepath.Join(filepath.Dir(profileDir), "sign-in-cookies.json")
}

// sanitizeForLog keeps secrets out of logs when naming a cookie domain.
func cookieDomainsSummary(cookies []*network.Cookie) string {
	seen := map[string]struct{}{}
	domains := []string{}
	for _, cookie := range cookies {
		if _, ok := authCookieNames[cookie.Name]; !ok {
			continue
		}
		if _, ok := seen[cookie.Domain]; ok {
			continue
		}
		seen[cookie.Domain] = struct{}{}
		domains = append(domains, cookie.Domain)
	}
	return strings.Join(domains, ", ")
}
